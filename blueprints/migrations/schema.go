// Package postgres holds the Go code of the built-in postgres blueprint: the
// database schema a project describes, and the migrations that move a
// database from one version of the project to the next.
package migrations

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/postgres"
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
	// generated is the expression of a column PostgreSQL computes and
	// stores (GENERATED ALWAYS AS ... STORED), or "".
	generated string
	// check is the column's CHECK expression, or "".
	check string
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
	// method is the index's access method: "" (btree) or gin.
	method string
	// where makes it partial: the rows it covers, or "" for all.
	where string
}

var sqlTypes = map[project.Type]string{
	project.TypeString:  "text",
	project.TypeText:    "text",
	project.TypeInt:     "integer",
	project.TypeInt64:   "bigint",
	project.TypeDecimal: "numeric",
	// Four places: prices finer than a cent.
	project.TypeMoney: "numeric(19,4)",
	// A file field's column holds its file's id: <field>_id.
	project.TypeFile:  "uuid",
	project.TypeImage: "uuid",
	project.TypeBool:  "boolean",
	project.TypeTime:  "timestamptz",
	project.TypeDate:  "date",
	project.TypeUUID:  "uuid",
	project.TypeJSON:  "jsonb",
}

// annotation returns a postgres annotation value of a node, or "".
func annotation(a project.Ann, key string) string {
	if v, ok := a["postgres"][key].(string); ok {
		return v
	}
	return ""
}

// currencyColumn is the currency of a money column: <column>_currency, an
// ISO 4217 code, the app's by default.
func currencyColumn(d project.Description, amount column) column {
	name := amount.name + "_currency"
	c := column{name: name, sqlType: "char(3)", notNull: amount.notNull, check: postgres.Ident(name) + " ~ '^[A-Z]{3}$'"}
	// Required, it takes the app's currency in the rows already there;
	// optional, it is empty with its amount.
	if amount.notNull && d.App.Currency != "" {
		c.def = "'" + d.App.Currency + "'"
	}
	return c
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
	if f.Type == project.TypeFile || f.Type == project.TypeImage {
		return f.Snake + "_id"
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
// not modules (but security, with the security blueprint). It does not
// change anything.
func Check(d project.Description) []string {
	var problems []string
	l := newLayout(d)
	for _, name := range ownSchemas(d) {
		known := false
		for _, m := range d.Modules {
			known = known || m.Raw == name
		}
		// security is the schema of the security blueprint's tables.
		if name == "security" && d.App.Options("security") != nil {
			known = true
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
		if f.Default != nil {
			c.def = defaultSQL(f)
		}
		t.columns = append(t.columns, c)
		columns[f.Raw] = c.name
		if f.Type == project.TypeMoney {
			t.columns = append(t.columns, currencyColumn(l.d, c))
		}
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
	related := len(t.indexes)
	listIndexes(l, e, &t)
	listed := len(t.indexes)
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
	if SoftDeleted(e) {
		trash(&t, related, listed)
	}
	return t
}

// Live are the rows of a table with soft delete that are not in the trash.
const Live = "deleted_at IS NULL"

// SoftDeleted reports whether deleting a record of the entity puts it in
// the trash (@store(soft_delete)) instead of removing it.
func SoftDeleted(e project.EntityDescription) bool {
	on, _ := e.Ann["store"]["soft_delete"].(bool)
	return on
}

// trash gives a table its trash: deleted_at, and uniqueness and the
// indexes of the lists only among the records not in it (a value of one in
// the trash may be taken again). The unique constraints become partial
// unique indexes of the same name, which the store reads the same way.
func trash(t *table, related, listed int) {
	at := len(t.columns)
	for i, c := range t.columns {
		if c.name == "created_at" {
			at = i
			break
		}
	}
	t.columns = slices.Insert(t.columns, at, column{name: "deleted_at", sqlType: "timestamptz"})
	// The lists' indexes, and the unique ones declared; the relations'
	// stay whole (a delete checks them all), and so do the other ones
	// declared, which serve queries of their own.
	for i := range t.indexes {
		if (i >= related && i < listed) || (i >= listed && t.indexes[i].unique) {
			t.indexes[i].where = Live
		}
	}
	for i := range t.columns {
		c := &t.columns[i]
		if !c.unique {
			continue
		}
		c.unique = false
		t.indexes = append(t.indexes, index{name: uniqueName(*t, *c), columns: []string{c.name}, unique: true, where: Live})
	}
}

// SearchColumn is the column lists search: the text of the fields marked
// @store(search), as PostgreSQL's text search reads it.
const SearchColumn = "search_vector"

// listIndexes are what the lists of the store need: (created_at, id) for
// the default order, (column, id) for each field marked @store(sort), an
// index for each field marked @store(filter) without one, and the search
// column with its GIN index for the fields marked @store(search).
func listIndexes(l layout, e project.EntityDescription, t *table) {
	flag := func(f project.FieldDescription, key string) bool {
		on, _ := f.Ann["store"][key].(bool)
		return on
	}
	t.indexes = append(t.indexes, index{name: t.name + "_created_at_id_idx", columns: []string{"created_at", "id"}})
	var searched []string
	for _, f := range e.Fields {
		c := columnName(f)
		switch {
		case f.Implicit:
			continue
		case flag(f, "sort"):
			t.indexes = append(t.indexes, index{name: t.name + "_" + c + "_id_idx", columns: []string{c, "id"}})
		case flag(f, "filter") && !f.Unique:
			t.indexes = append(t.indexes, index{name: t.name + "_" + c + "_idx", columns: []string{c}})
		}
		if flag(f, "search") {
			searched = append(searched, "coalesce("+postgres.Ident(c)+", '')")
		}
	}
	if len(searched) == 0 {
		return
	}
	lang, _ := l.d.App.Options("store")["search_language"].(string)
	if lang == "" {
		lang = "simple"
	}
	expr := "to_tsvector(" + quoteLiteral(lang) + "::regconfig, " + strings.Join(searched, " || ' ' || ") + ")"
	// It goes before created_at and updated_at, which stay last.
	at := len(t.columns)
	for i, c := range t.columns {
		if c.name == "created_at" {
			at = i
			break
		}
	}
	t.columns = slices.Insert(t.columns, at, column{name: SearchColumn, sqlType: "tsvector", generated: expr})
	t.indexes = append(t.indexes, index{name: t.name + "_search_idx", columns: []string{SearchColumn}, method: "gin"})
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
	case project.TypeInt, project.TypeInt64, project.TypeDecimal, project.TypeMoney, project.TypeBool:
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
