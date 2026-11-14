// Package postgres holds the Go code of the built-in postgres blueprint: the
// database schema a project describes, and the migrations that move a
// database from one version of the project to the next.
package postgres

import (
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// Each module owns a Postgres schema named after it: its tables and enum
// types live there, and foreign keys only point inside it, so the database
// keeps modules as separate as the code does.

// database is the schema of a whole project.
type database struct {
	schemas []string
	enums   []enumType
	tables  []table
}

type enumType struct {
	schema, name string
	values       []string
}

func (e enumType) qualified() string { return e.schema + "." + e.name }

type table struct {
	schema, name string
	columns      []column
	foreignKeys  []foreignKey
	indexes      []index
	// primaryKey lists the key's columns: id, or both columns of a join
	// table.
	primaryKey []string
}

func (t table) qualified() string { return t.schema + "." + t.name }

func (t table) column(name string) *column {
	for i := range t.columns {
		if t.columns[i].name == name {
			return &t.columns[i]
		}
	}
	return nil
}

type column struct {
	name    string
	sqlType string
	notNull bool
	// def is the default as an SQL expression, or "".
	def    string
	unique bool
}

type foreignKey struct {
	name, column string
	refTable     string // qualified
	onDelete     string
}

type index struct {
	name    string
	columns []string
	unique  bool
}

var sqlTypes = map[project.Type]string{
	project.TypeString:  "text",
	project.TypeText:    "text",
	project.TypeInt:     "integer",
	project.TypeInt64:   "bigint",
	project.TypeDecimal: "numeric",
	project.TypeBool:    "boolean",
	project.TypeTime:    "timestamptz",
	project.TypeDate:    "date",
	project.TypeUUID:    "uuid",
	project.TypeJSON:    "jsonb",
}

// annotation returns a postgres annotation value of a node, or "".
func annotation(a project.Ann, key string) string {
	if v, ok := a["postgres"][key].(string); ok {
		return v
	}
	return ""
}

// tableName is the entity's table: @postgres(table ...) or its plural.
func tableName(e project.EntityDescription) string {
	if t := annotation(e.Ann, "table"); t != "" {
		return t
	}
	return e.Plural.Snake
}

// columnName is the field's column: @postgres(column ...) or its name.
func columnName(f project.FieldDescription) string {
	if c := annotation(f.Ann, "column"); c != "" {
		return c
	}
	return f.Snake
}

// describe builds the database a project description stands for.
func describe(d project.Description) database {
	var db database
	for _, m := range d.Modules {
		db.schemas = append(db.schemas, m.Snake)
	}
	for _, e := range d.Enums {
		et := enumType{schema: moduleSchema(d, e.Module), name: e.Snake}
		for _, v := range e.Values {
			et.values = append(et.values, v.Raw)
		}
		db.enums = append(db.enums, et)
	}
	entities := map[string]project.EntityDescription{}
	for _, e := range d.Entities {
		entities[e.Raw] = e
	}
	for _, e := range d.Entities {
		db.tables = append(db.tables, entityTable(d, e, entities))
		for _, r := range e.Relations {
			if r.Kind == "many-to-many" && !r.Inferred {
				db.tables = append(db.tables, joinTable(d, e, r, entities[r.Target]))
			}
		}
	}
	return db
}

func moduleSchema(d project.Description, module string) string {
	for _, m := range d.Modules {
		if m.Raw == module {
			return m.Snake
		}
	}
	return module
}

func entityTable(d project.Description, e project.EntityDescription, entities map[string]project.EntityDescription) table {
	schema := moduleSchema(d, e.Module)
	t := table{schema: schema, name: tableName(e), primaryKey: []string{"id"}}
	columns := map[string]string{} // field or relation name as written -> column
	for _, f := range e.Fields {
		c := column{name: columnName(f), notNull: f.Required, unique: f.Unique}
		c.sqlType = sqlTypes[f.Type]
		if f.Type == project.TypeEnum {
			c.sqlType = moduleSchema(d, enumModule(d, f.Enum)) + "." + snakeOfEnum(d, f.Enum)
		}
		if f.Implicit && f.Snake == "id" {
			c.def = "gen_random_uuid()"
		}
		if f.Default != nil {
			c.def = defaultSQL(f)
		}
		t.columns = append(t.columns, c)
		columns[f.Raw] = c.name
	}
	for _, r := range e.Relations {
		if r.Inferred || (r.Kind != "many-to-one" && r.Kind != "one-to-one") {
			continue
		}
		c := column{name: r.Snake + "_id", sqlType: "uuid", notNull: r.Required, unique: r.Kind == "one-to-one"}
		// created_at and updated_at stay last.
		at := len(t.columns)
		for i, existing := range t.columns {
			if existing.name == "created_at" {
				at = i
				break
			}
		}
		t.columns = slices.Insert(t.columns, at, c)
		columns[r.Raw] = c.name
		target := entities[r.Target]
		if target.Module == e.Module {
			onDelete := "SET NULL"
			if r.Required {
				onDelete = "RESTRICT"
			}
			t.foreignKeys = append(t.foreignKeys, foreignKey{
				name: t.name + "_" + c.name + "_fkey", column: c.name,
				refTable: schema + "." + tableName(target), onDelete: onDelete,
			})
		}
		if !c.unique {
			t.indexes = append(t.indexes, index{name: t.name + "_" + c.name + "_idx", columns: []string{c.name}})
		}
	}
	for _, key := range []string{"index", "unique_index"} {
		names, _ := e.Ann["postgres"][key].([]any)
		if len(names) == 0 {
			if s, ok := e.Ann["postgres"][key].([]string); ok {
				for _, n := range s {
					names = append(names, n)
				}
			}
		}
		if len(names) == 0 {
			continue
		}
		var cols []string
		for _, n := range names {
			if col, ok := columns[n.(string)]; ok {
				cols = append(cols, col)
			}
		}
		suffix := "_idx"
		if key == "unique_index" {
			suffix = "_unique_idx" // _key is taken by unique constraints
		}
		t.indexes = append(t.indexes, index{
			name: t.name + "_" + strings.Join(cols, "_") + suffix, columns: cols, unique: key == "unique_index",
		})
	}
	return t
}

func joinTable(d project.Description, e project.EntityDescription, r project.RelationDescription, target project.EntityDescription) table {
	schema := moduleSchema(d, e.Module)
	left, right := e.Snake+"_id", target.Snake+"_id"
	if left == right {
		right = r.Snake + "_id"
	}
	t := table{
		schema: schema, name: r.JoinTable, primaryKey: []string{left, right},
		columns: []column{
			{name: left, sqlType: "uuid", notNull: true},
			{name: right, sqlType: "uuid", notNull: true},
		},
		foreignKeys: []foreignKey{{
			name: r.JoinTable + "_" + left + "_fkey", column: left,
			refTable: schema + "." + tableName(e), onDelete: "CASCADE",
		}},
		indexes: []index{{name: r.JoinTable + "_" + right + "_idx", columns: []string{right}}},
	}
	if target.Module == e.Module {
		t.foreignKeys = append(t.foreignKeys, foreignKey{
			name: r.JoinTable + "_" + right + "_fkey", column: right,
			refTable: schema + "." + tableName(target), onDelete: "CASCADE",
		})
	}
	return t
}

func enumModule(d project.Description, name string) string {
	for _, e := range d.Enums {
		if e.Raw == name {
			return e.Module
		}
	}
	return ""
}

func snakeOfEnum(d project.Description, name string) string {
	for _, e := range d.Enums {
		if e.Raw == name {
			return e.Snake
		}
	}
	return name
}

// defaultSQL writes a field's default as an SQL expression.
func defaultSQL(f project.FieldDescription) string {
	def := f.Default
	switch {
	case def.Kind == project.DefaultNow && f.Type == project.TypeDate:
		return "CURRENT_DATE"
	case def.Kind == project.DefaultNow:
		return "now()"
	}
	switch f.Type {
	case project.TypeInt, project.TypeInt64, project.TypeDecimal, project.TypeBool:
		return def.Text
	case project.TypeJSON:
		return quoteLiteral(def.Text) + "::jsonb"
	}
	return quoteLiteral(def.Text)
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// itoa5 writes a migration number with five digits, as goose's sequential
// versions do.
func itoa5(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 5 {
		s = "0" + s
	}
	return s
}
