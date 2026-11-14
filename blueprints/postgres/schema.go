// Package postgres holds the Go code of the built-in postgres blueprint: the
// database schema a project describes, and the migrations that move a
// database from one version of the project to the next.
package postgres

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// Tables live in public, except for the modules listed in the blueprint's
// schemas option, which own a Postgres schema named after them. Either way,
// foreign keys only point inside a module, so the database keeps modules as
// separate as the code does. In public, enum types are prefixed with their
// module (catalog_status), since two modules may declare the same enum name.

// database is the schema of a whole project.
type database struct {
	schemas []string
	enums   []enumType
	tables  []table
}

type enumType struct {
	// key identifies the enum across versions of the project, whatever its
	// schema and name: module and enum as written.
	key          string
	schema, name string
	values       []string
}

func (e enumType) qualified() string { return e.schema + "." + e.name }

type table struct {
	// key identifies the table across versions of the project, whatever
	// its schema and name: the entity, or the join table's relation.
	key          string
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

// layout says where each module's tables go.
type layout struct {
	own map[string]bool // modules with a schema of their own
	d   project.Description
}

func newLayout(d project.Description) layout {
	l := layout{own: map[string]bool{}, d: d}
	for _, name := range ownSchemas(d) {
		l.own[name] = true
	}
	return l
}

// ownSchemas lists the modules of the schemas option.
func ownSchemas(d project.Description) []string {
	var out []string
	switch v := d.App.Options("postgres")["schemas"].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = v
	}
	return out
}

func (l layout) schema(module string) string {
	if l.own[module] {
		return moduleSnake(l.d, module)
	}
	return "public"
}

// enumName is an enum type's name in its schema.
func (l layout) enumName(module, enum string) string {
	if l.own[module] {
		return enum
	}
	return moduleSnake(l.d, module) + "_" + enum
}

// describe builds the database a project description stands for.
func describe(d project.Description) database {
	var db database
	l := newLayout(d)
	for _, m := range d.Modules {
		if l.own[m.Raw] {
			db.schemas = append(db.schemas, m.Snake)
		}
	}
	for _, e := range d.Enums {
		et := enumType{key: e.Module + "." + e.Raw, schema: l.schema(e.Module), name: l.enumName(e.Module, e.Snake)}
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
		db.tables = append(db.tables, entityTable(l, e, entities))
		for _, r := range e.Relations {
			if r.Kind == "many-to-many" && !r.Inferred {
				db.tables = append(db.tables, joinTable(l, e, r, entities[r.Target]))
			}
		}
	}
	return db
}

func moduleSnake(d project.Description, module string) string {
	for _, m := range d.Modules {
		if m.Raw == module {
			return m.Snake
		}
	}
	return module
}

// Check reports what would make the database invalid: tables of different
// modules with the same name in public, and schemas option entries that are
// not modules. It does not change anything.
func Check(d project.Description) []string {
	var problems []string
	l := newLayout(d)
	for _, name := range ownSchemas(d) {
		known := false
		for _, m := range d.Modules {
			known = known || m.Raw == name
		}
		if !known {
			problems = append(problems, fmt.Sprintf("the schemas option lists %q, which is not a module", name))
		}
	}
	seen := map[string]string{}
	for _, e := range d.Entities {
		if l.own[e.Module] {
			continue
		}
		t := tableName(e)
		if prev, ok := seen[t]; ok {
			problems = append(problems, fmt.Sprintf("%s and %s.%s would both be table public.%s; rename one with @postgres(table ...) or give a module its own schema", prev, e.Module, e.Raw, t))
			continue
		}
		seen[t] = e.Module + "." + e.Raw
	}
	return problems
}

func entityTable(l layout, e project.EntityDescription, entities map[string]project.EntityDescription) table {
	schema := l.schema(e.Module)
	t := table{key: "entity " + e.Module + "." + e.Raw, schema: schema, name: tableName(e), primaryKey: []string{"id"}}
	columns := map[string]string{} // field or relation name as written -> column
	for _, f := range e.Fields {
		c := column{name: columnName(f), notNull: f.Required, unique: f.Unique}
		c.sqlType = sqlTypes[f.Type]
		if f.Type == project.TypeEnum {
			module := enumModule(l.d, f.Enum)
			c.sqlType = l.schema(module) + "." + l.enumName(module, snakeOfEnum(l.d, f.Enum))
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
		if !r.OwnsKey {
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
			t.foreignKeys = append(t.foreignKeys, foreignKey{
				name: t.name + "_" + c.name + "_fkey", column: c.name,
				refTable: schema + "." + tableName(target), onDelete: onDelete(r),
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

// onDelete is the ON DELETE action of a relation's foreign key: the one it
// declares, or RESTRICT when it is required and SET NULL when it is not.
func onDelete(r project.RelationDescription) string {
	switch r.OnDelete {
	case "cascade":
		return "CASCADE"
	case "restrict":
		return "RESTRICT"
	case "set_null":
		return "SET NULL"
	}
	if r.Required {
		return "RESTRICT"
	}
	return "SET NULL"
}

func joinTable(l layout, e project.EntityDescription, r project.RelationDescription, target project.EntityDescription) table {
	schema := l.schema(e.Module)
	left, right := e.Snake+"_id", target.Snake+"_id"
	if left == right {
		right = r.Snake + "_id"
	}
	t := table{
		key:    "join " + e.Module + "." + e.Raw + "." + r.Raw,
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

// reserved are Postgres keywords that cannot name a column or table without
// quotes; esquadro quotes them wherever it writes SQL.
var reserved = map[string]bool{
	"all": true, "analyse": true, "analyze": true, "and": true, "any": true, "array": true,
	"as": true, "asc": true, "asymmetric": true, "both": true, "case": true, "cast": true,
	"check": true, "collate": true, "column": true, "constraint": true, "create": true,
	"current_catalog": true, "current_date": true, "current_role": true, "current_time": true,
	"current_timestamp": true, "current_user": true, "default": true, "deferrable": true,
	"desc": true, "distinct": true, "do": true, "else": true, "end": true, "except": true,
	"false": true, "fetch": true, "for": true, "foreign": true, "from": true, "grant": true,
	"group": true, "having": true, "in": true, "initially": true, "intersect": true, "into": true,
	"lateral": true, "leading": true, "limit": true, "localtime": true, "localtimestamp": true,
	"not": true, "null": true, "offset": true, "on": true, "only": true, "or": true, "order": true,
	"placing": true, "primary": true, "references": true, "returning": true, "select": true,
	"session_user": true, "some": true, "symmetric": true, "system_user": true, "table": true,
	"then": true, "to": true, "trailing": true, "true": true, "union": true, "unique": true,
	"user": true, "using": true, "variadic": true, "when": true, "where": true, "window": true,
	"with": true,
}

// Ident writes a column or table name for SQL, quoted when it is a keyword.
func Ident(name string) string {
	if reserved[name] {
		return `"` + name + `"`
	}
	return name
}
