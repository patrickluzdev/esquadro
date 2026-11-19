package store

import (
	"fmt"
	"slices"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/audit"
	"github.com/patrickluzdev/esquadro/blueprints/notifications"
	"github.com/patrickluzdev/esquadro/blueprints/postgres"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the helpers the postgres blueprint's templates use. They
// know how sqlc names and types what it generates (with the sqlc.yaml the
// blueprint writes: exact table names, pgx/v5 and esquadro's initialisms),
// and how to convert between those types and the domain's.
//
// opts is always the template's .Options, the postgres blueprint's options.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"listFilters":  ListFilters,
		"listSorts":    ListSorts,
		"searchFields": SearchFields,
		// searchField is the Go field of e's first searched field.
		"searchField": func(e *project.Entity) string {
			for _, f := range e.Fields {
				if on, _ := f.Ann["store"]["search"].(bool); on {
					return f.Pascal
				}
			}
			return ""
		},
		"listImports": listImports,
		"listWhere":   ListWhere,
		"rank":        Rank,
		"sqlcName":    func(s string) string { return names(s).Pascal },
		"list":        func(items ...string) []string { return items },
		"dict": func(kv ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				m[kv[i].(string)] = kv[i+1]
			}
			return m
		},
		"pgTable":   pgTable,
		"pgColumn":  pgColumn,
		"sqlcModel": sqlcModel,
		"sqlcField": sqlcField,
		"sqlcIn":    sqlcIn,
		"sqlcOut":   sqlcOut,
		"sqlcEnum":  sqlcEnum,
		"ownSchema": ownSchema,
		"keyColumn": func(r *project.Relation) string { return r.Snake + "_id" },
		// ownerField is the Go field of the owner of an entity's records:
		// its relation's key, or ID in a principal; empty without.
		"ownerField": func(entities []*project.Entity, e *project.Entity) string {
			o, ok := security.OwnerUsed(entities, e)
			switch {
			case !ok:
				return ""
			case o.Relation == nil:
				return "ID"
			}
			return o.Relation.Pascal + "ID"
		},
		// ownerColumn is the column of the owner of an entity's records,
		// when a rule of security is owner: its relation's key, or id in a
		// principal; empty without.
		"ownerColumn": func(entities []*project.Entity, e *project.Entity) string {
			o, ok := security.OwnerUsed(entities, e)
			switch {
			case !ok:
				return ""
			case o.Relation == nil:
				return "id"
			}
			return postgres.Ident(o.Relation.Snake + "_id")
		},
		"keyField":   func(r *project.Relation) string { return names(r.Snake + "_id").Pascal },
		"keyIn":      keyIn,
		"keyOut":     keyOut,
		"needs":      needs,
		"constraint": constraint,
		"pgIdent":    postgres.Ident,
		"writable":   writable,
		"fileFields": fileFieldsOf,
		"refusals":   refusals,
		"contains":   strings.Contains,
		"reportCast": reportCast,
		// The currency the SQL esquadro writes first gives a money column.
		"reportCurrency": func() string { return "'XXX'" },
		"reportImports":  reportImports,
		"reportParam":    reportParam,
		"reportSample":   reportSample,
		"reportRead": func(f *project.Field) string {
			r, _ := reportReader(f)
			return r
		},
		"reportConvert": func(f *project.Field, v string) string {
			_, c := reportReader(f)
			return fmt.Sprintf(c, v)
		},
		"textFields": func(e *project.Entity) []*project.Field {
			var out []*project.Field
			for _, f := range e.Fields {
				if !f.Implicit && (f.Type == project.TypeString || f.Type == project.TypeText) {
					out = append(out, f)
				}
			}
			return out
		},
		"canSample":       canSample,
		"parents":         parents,
		"sample":          sample,
		"sampleImports":   sampleImports,
		"mustRefuseEmpty": mustRefuseEmpty,
		"initialisms":     project.Initialisms,
		"link":            link,
		"softDeleted":     SoftDeleted,
		"single":          Single,
		"importable":      Importable,
		"importBy":        ImportBy,
		"importKey":       importKey,
		"importMax":       func() int { return ImportMax },
		"singleID":        func() string { return SingleID },
		"dependents":      Dependents,
		"trashedParents":  TrashedParents,
		"onDelete":        OnDelete,
		"trashModules":    TrashModules,
		"idLinks":         IDLinks,
		"audited":         audit.Audited,
		"auditSecret":     audit.Secret,
		"auditIgnored":    audit.Ignored,
		"noticeRule":      notifications.RuleOf,
		"noticeOwner":     notifications.OwnerFor,
		"fieldsByName": func(e *project.Entity) map[string]*project.Field {
			out := map[string]*project.Field{}
			for _, f := range e.Fields {
				out[f.Snake], out[f.Raw] = f, f
			}
			return out
		},
		"links":      links,
		"manyToMany": func(r *project.Relation) bool { return r.Kind == project.ManyToMany },
	}
}

// names builds the Go names of a snake_case word as esquadro and sqlc (with
// the initialisms the generated sqlc.yaml lists) both do.
func names(snake string) project.Names {
	n, _ := project.NamesOf(snake)
	return n
}

func ownSchema(module *project.Module, opts map[string]any) bool {
	list, _ := opts["schemas"].([]any)
	for _, x := range list {
		if x == module.Raw {
			return true
		}
	}
	return false
}

func schemaOf(module *project.Module, opts map[string]any) string {
	if ownSchema(module, opts) {
		return module.Snake
	}
	return "public"
}

func tableOf(e *project.Entity) string {
	if t, ok := e.Ann["postgres"]["table"].(string); ok {
		return t
	}
	return e.Plural.Snake
}

// pgTable is the entity's table, schema-qualified, for SQL.
func pgTable(e *project.Entity, opts map[string]any) string {
	return schemaOf(e.Module, opts) + "." + tableOf(e)
}

// pgColumn is the field's column.
func pgColumn(f *project.Field) string {
	if c, ok := f.Ann["postgres"]["column"].(string); ok {
		return c
	}
	if f.IsFile() {
		return f.Snake + "_id"
	}
	return f.Snake
}

// fileFieldsOf lists an entity's file and image fields.
func fileFieldsOf(e *project.Entity) []*project.Field {
	var out []*project.Field
	for _, f := range e.Fields {
		if f.IsFile() {
			out = append(out, f)
		}
	}
	return out
}

// sqlcModel is the struct sqlc generates for the entity's table: its exact
// name, prefixed with the schema outside public.
func sqlcModel(e *project.Entity, opts map[string]any) string {
	if ownSchema(e.Module, opts) {
		return names(e.Module.Snake + "_" + tableOf(e)).Pascal
	}
	return names(tableOf(e)).Pascal
}

// sqlcField is the field's name in sqlc's structs: its column, in Go case.
func sqlcField(f *project.Field) string { return names(pgColumn(f)).Pascal }

// sqlcEnum is the Go type sqlc generates for the enum of a field.
func sqlcEnum(f *project.Field) string {
	return "sqlgen." + names(f.Enum.Module.Snake+"_"+f.Enum.Snake).Pascal
}

// nullable reports whether the field's column accepts NULL.
func nullable(f *project.Field) bool { return !f.Required && !f.Implicit }

// sqlcIn converts the domain value expr of a field to what sqlc's parameter
// expects.
func sqlcIn(f *project.Field, expr string) string {
	null := nullable(f)
	switch f.Type {
	case project.TypeString, project.TypeText:
		if null {
			return "postgres.Text(" + expr + ")"
		}
		return expr
	case project.TypeInt:
		if null {
			return "postgres.Int4(" + expr + ")"
		}
		return "int32(" + expr + ")"
	case project.TypeInt64:
		if null {
			return "postgres.Int8(" + expr + ")"
		}
		return expr
	case project.TypeDecimal:
		return "postgres.Numeric(" + expr + ")"
	case project.TypeMoney:
		return "postgres.MoneyAmount(" + expr + ")"
	case project.TypeBool:
		if null {
			return "postgres.Bool(" + expr + ")"
		}
		return expr
	case project.TypeTime:
		return "postgres.Timestamptz(" + expr + ")"
	case project.TypeDate:
		return "postgres.Date(" + expr + ")"
	case project.TypeUUID, project.TypeFile, project.TypeImage:
		return "postgres.UUID(" + expr + ")"
	case project.TypeJSON:
		return "[]byte(" + expr + ")"
	case project.TypeEnum:
		if null {
			return fmt.Sprintf("%s{%s: %s(%s), Valid: %s != \"\"}", nullEnum(f), strings.TrimPrefix(sqlcEnum(f), "sqlgen."), sqlcEnum(f), expr, expr)
		}
		return sqlcEnum(f) + "(" + expr + ")"
	}
	return expr
}

func nullEnum(f *project.Field) string {
	return "sqlgen.Null" + strings.TrimPrefix(sqlcEnum(f), "sqlgen.")
}

// sqlcOut converts sqlc's value expr of a field to the domain's.
func sqlcOut(f *project.Field, expr string) string {
	null := nullable(f)
	switch f.Type {
	case project.TypeString, project.TypeText:
		if null {
			return expr + ".String"
		}
		return expr
	case project.TypeInt:
		if null {
			return "int(" + expr + ".Int32)"
		}
		return "int(" + expr + ")"
	case project.TypeInt64:
		if null {
			return expr + ".Int64"
		}
		return expr
	case project.TypeDecimal:
		return "postgres.Decimal(" + expr + ")"
	case project.TypeMoney:
		// expr is the amount's column (row.Price); its currency is beside it.
		code := expr + "Currency"
		if null {
			code += ".String"
		}
		return "postgres.Money(" + expr + ", " + code + ")"
	case project.TypeBool:
		if null {
			return expr + ".Bool"
		}
		return expr
	case project.TypeTime, project.TypeDate:
		return expr + ".Time"
	case project.TypeUUID, project.TypeFile, project.TypeImage:
		return "postgres.ID(" + expr + ")"
	case project.TypeJSON:
		return "json.RawMessage(" + expr + ")"
	case project.TypeEnum:
		if null {
			return fmt.Sprintf("%s(%s.%s)", f.Enum.Pascal, expr, strings.TrimPrefix(sqlcEnum(f), "sqlgen."))
		}
		return f.Enum.Pascal + "(" + expr + ")"
	}
	return expr
}

// keyIn and keyOut convert a relation's key (a string ID in the domain).
func keyIn(r *project.Relation, expr string) string { return "postgres.UUID(" + expr + ")" }

func keyOut(r *project.Relation, expr string) string { return "postgres.ID(" + expr + ")" }

// needs reports whether an entity's stored form needs an import: "json" or
// "decimal" (its fields), for the files that convert rows.
func needs(e *project.Entity, what string) bool {
	for _, f := range e.Fields {
		switch {
		case what == "json" && f.Type == project.TypeJSON:
			return true
		case what == "decimal" && f.Type == project.TypeDecimal:
			return true
		case what == "money" && f.Type == project.TypeMoney:
			return true
		}
	}
	return false
}

// constraint is the name of the constraint the migrations give a unique
// field ("key") or a relation's foreign key ("fkey").
func constraint(e *project.Entity, column, kind string) string {
	return tableOf(e) + "_" + column + "_" + kind
}

// Writable is a column a use case writes: a field, or the key of a relation
// this entity holds.
type Writable struct {
	// Column is the column for SQL (quoted when a keyword), Param the
	// sqlc parameter name and GoParam its field in sqlc's Params struct.
	Column, Param, GoParam string
	Field                  *project.Field
	Relation               *project.Relation
	// Currency marks the currency column of a money field, written after
	// its amount's.
	Currency bool
}

// In converts the domain value of the column, read from the struct named
// v (a NewX), to sqlc's parameter.
func (w Writable) In(v string) string {
	if w.Currency {
		code := v + "." + w.Field.Pascal + ".CurrencyCode()"
		if nullable(w.Field) {
			return "postgres.Text(" + code + ")"
		}
		return code
	}
	if w.Field != nil {
		return sqlcIn(w.Field, v+"."+w.Field.Pascal)
	}
	return keyIn(w.Relation, v+"."+w.Relation.Pascal+"ID")
}

// writable lists the columns an entity's create and update write: its
// declared fields, then the keys of the relations it holds.
func writable(e *project.Entity) []Writable {
	var out []Writable
	for _, f := range e.Fields {
		if f.Implicit {
			continue
		}
		c := pgColumn(f)
		out = append(out, Writable{Column: postgres.Ident(c), Param: c, GoParam: names(c).Pascal, Field: f})
		if f.Type == project.TypeMoney {
			cc := c + "_currency"
			out = append(out, Writable{Column: postgres.Ident(cc), Param: cc, GoParam: names(cc).Pascal, Field: f, Currency: true})
		}
	}
	for _, r := range e.Relations {
		if r.OwnsForeignKey() {
			c := r.Snake + "_id"
			out = append(out, Writable{Column: postgres.Ident(c), Param: c, GoParam: names(c).Pascal, Relation: r})
		}
	}
	return out
}

// Refusal is a constraint the database enforces on an entity, with the
// field and the domain code it stands for.
type Refusal struct {
	Constraint, Field, Code string
}

// refusals lists the constraints the migrations give an entity that a
// write can break: unique fields and one-to-one keys ("taken"), and the
// foreign keys inside the module ("not_found").
func refusals(e *project.Entity) []Refusal {
	var out []Refusal
	for _, f := range e.Fields {
		if f.Unique {
			out = append(out, Refusal{constraint(e, pgColumn(f), "key"), f.Snake, "taken"})
		}
	}
	for _, r := range e.Relations {
		if !r.OwnsForeignKey() {
			continue
		}
		col := r.Snake + "_id"
		if r.Kind == project.OneToOne {
			out = append(out, Refusal{constraint(e, col, "key"), r.Snake, "taken"})
		}
		if r.Target.Module == e.Module {
			out = append(out, Refusal{constraint(e, col, "fkey"), r.Snake, "not_found"})
		}
	}
	return out
}

// ---- test data ----

// canSample reports whether esquadro can write a valid NewX for an entity:
// not when a required field has a pattern (no generic value matches any
// pattern), nor when a required relation needs a record it cannot write
// either, or loops back.
func canSample(e *project.Entity) bool { return sampleable(e, map[*project.Entity]bool{}) }

func sampleable(e *project.Entity, visiting map[*project.Entity]bool) bool {
	if visiting[e] {
		return false
	}
	visiting[e] = true
	defer delete(visiting, e)
	for _, f := range e.Fields {
		if f.Required && f.Pattern != "" && (f.Type == project.TypeString || f.Type == project.TypeText) {
			return false
		}
		// A required file is one sent first: no value stands for it.
		if f.Required && f.IsFile() {
			return false
		}
	}
	for _, r := range parents(e) {
		if !sampleable(r.Target, visiting) {
			return false
		}
	}
	return true
}

// parents are the required relations of an entity to records of its own
// module, which a test must create first.
func parents(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range e.Relations {
		if r.OwnsForeignKey() && r.Required && r.Target.Module == e.Module {
			out = append(out, r)
		}
	}
	return out
}

// sample is a Go expression for a valid value of a field in tests, with i
// (a Go expression) telling records apart; empty when the field is best
// left unset (an optional field with a pattern).
// Sample is a Go expression of a valid value for a field, for tests: pkg
// qualifies its enum, i is an int expression that makes strings differ. It
// is empty when no value can be made (a string with a pattern).
func Sample(f *project.Field, pkg, i string) string { return sample(f, pkg, i) }

func sample(f *project.Field, pkg, i string) string {
	switch f.Type {
	case project.TypeString, project.TypeText:
		if f.Pattern != "" {
			return ""
		}
		return sampleString(f, i)
	case project.TypeUUID:
		return "uuid.NewV7()"
	case project.TypeInt, project.TypeInt64:
		n := "1"
		if f.Min != nil {
			n = f.Min.Text
		} else if f.Max != nil && f.Max.Value < 1 {
			n = f.Max.Text
		}
		return n
	case project.TypeDecimal:
		n := "1"
		if f.Min != nil {
			n = f.Min.Text
		} else if f.Max != nil && f.Max.Value < 1 {
			n = f.Max.Text
		}
		return `decimal.RequireFromString("` + n + `")`
	case project.TypeMoney:
		n := "1"
		if f.Min != nil {
			n = f.Min.Text
		} else if f.Max != nil && f.Max.Value < 1 {
			n = f.Max.Text
		}
		return `func() currency.Amount { a, _ := currency.NewAmount("` + n + `", "USD"); return a }()`
	case project.TypeBool:
		return "true"
	case project.TypeTime, project.TypeDate:
		return "time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)"
	case project.TypeJSON:
		return `json.RawMessage("{}")`
	case project.TypeEnum:
		return pkg + "." + f.Enum.Pascal + f.Enum.Values[0].Pascal
	}
	return ""
}

// sampleString is "<field> <i>", padded with zeros to the field's minimum
// length, or a run of x when its maximum is too short for that.
func sampleString(f *project.Field, i string) string {
	word := f.Snake
	digits := 1
	if f.Min != nil {
		digits = max(digits, int(f.Min.Value)-len(word)-1)
	}
	length := len(word) + 1 + digits
	if f.Max != nil && float64(length) > f.Max.Value {
		n := int(f.Max.Value)
		if f.Min != nil {
			n = int(f.Min.Value)
		}
		return fmt.Sprintf("strings.Repeat(\"x\", %d)", max(n, 1))
	}
	return fmt.Sprintf("fmt.Sprintf(\"%s %%0%dd\", %s)", word, digits, i)
}

// sampleImports lists the imports the samples of an entity need.
func sampleImports(e *project.Entity) []string { return SampleImports(e.Fields) }

// SampleImports are the imports the samples of fields need.
func SampleImports(fields []*project.Field) []string {
	var out []string
	add := func(p string) {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	for _, f := range fields {
		if f.Implicit {
			continue
		}
		s := sample(f, "", "i")
		switch {
		case strings.HasPrefix(s, "fmt."):
			add("fmt")
		case strings.HasPrefix(s, "strings."):
			add("strings")
		case strings.HasPrefix(s, "decimal."):
			add("github.com/shopspring/decimal")
		case strings.HasPrefix(s, "func() currency."):
			add("github.com/bojanz/currency")
		case strings.HasPrefix(s, "time."):
			add("time")
		case strings.HasPrefix(s, "json."):
			add("encoding/json")
		}
	}
	return out
}

// mustRefuseEmpty reports whether an empty NewX breaks a rule, so a test can
// check that creating one is refused.
func mustRefuseEmpty(e *project.Entity) bool {
	for _, f := range e.Fields {
		if !f.Required || f.Implicit {
			continue
		}
		switch f.Type {
		case project.TypeString, project.TypeText, project.TypeUUID, project.TypeEnum,
			project.TypeTime, project.TypeDate, project.TypeJSON:
			return true
		}
	}
	for _, r := range e.Relations {
		if r.OwnsForeignKey() && r.Required {
			return true
		}
	}
	return false
}

// ---- many-to-many ----

// Link is a many-to-many relation as its join table sees it from one side.
type Link struct {
	Table string // the join table, with its schema
	// Own and Other are the columns of this side's ID and the other side's.
	Own, Other string
	// OwnKey and OtherKey are their foreign keys; OtherKey is empty across
	// modules, which share no foreign keys.
	OwnKey, OtherKey string
	// Declared is the side written in the .esq file, which links and
	// unlinks; the other side only lists.
	Declared bool
	// Other is the other side's ID parameter in Go.
	OtherParam string
}

// link is r's join table seen from r's side.
func link(r *project.Relation, opts map[string]any) Link {
	owner := r
	if r.Inferred && r.Inverse != nil {
		owner = r.Inverse
	}
	left, right := owner.Entity.Snake+"_id", owner.Target.Snake+"_id"
	if left == right {
		right = owner.Snake + "_id"
	}
	l := Link{Table: schemaOf(owner.Entity.Module, opts) + "." + owner.JoinTable, Declared: r == owner}
	l.Own, l.Other = left, right
	if !l.Declared {
		l.Own, l.Other = right, left
	}
	l.OwnKey = owner.JoinTable + "_" + l.Own + "_fkey"
	if !owner.CrossesModules() {
		l.OtherKey = owner.JoinTable + "_" + l.Other + "_fkey"
	}
	l.OtherParam = r.Target.Camel + "ID"
	if r.Target == r.Entity {
		l.OtherParam = "otherID"
	}
	return l
}

// links are the many-to-many relations of an entity, both sides.
func links(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range e.Relations {
		if r.Kind == project.ManyToMany {
			out = append(out, r)
		}
	}
	return out
}

// reportCast is the PostgreSQL cast of a report's column or parameter of
// f's type, which makes the Go type sqlc gives it known.
func reportCast(f *project.Field) string {
	switch f.Type {
	case project.TypeInt, project.TypeInt64:
		return "bigint"
	case project.TypeDecimal, project.TypeMoney:
		return "numeric"
	case project.TypeBool:
		return "boolean"
	case project.TypeTime:
		return "timestamptz"
	case project.TypeDate:
		return "date"
	case project.TypeUUID:
		return "uuid"
	}
	return "text"
}

// reportParam is the Go expression of a report parameter, q.<Field>, as
// sqlc's parameters take it: sqlc.arg when required, sqlc.narg when not.
func reportParam(f *project.Field) string {
	x := "q." + f.Pascal
	if !f.Required {
		switch f.Type {
		case project.TypeInt:
			return "postgres.Opt(" + x + ", func(n int) pgtype.Int8 { return postgres.Int8(int64(n)) })"
		case project.TypeInt64:
			return "postgres.Opt(" + x + ", postgres.Int8)"
		case project.TypeDecimal:
			return "postgres.Opt(" + x + ", postgres.Numeric)"
		case project.TypeBool:
			return "postgres.Opt(" + x + ", postgres.Bool)"
		case project.TypeTime:
			return "postgres.Opt(" + x + ", postgres.Timestamptz)"
		case project.TypeDate:
			return "postgres.Opt(" + x + ", postgres.Date)"
		case project.TypeUUID:
			return "postgres.Opt(" + x + ", postgres.UUID)"
		case project.TypeEnum:
			return "postgres.Opt(" + x + ", func(e " + f.Enum.Pascal + ") pgtype.Text { return postgres.Text(string(e)) })"
		}
		return "postgres.Opt(" + x + ", postgres.Text)"
	}
	switch f.Type {
	case project.TypeInt:
		return "int64(" + x + ")"
	case project.TypeDecimal:
		return "postgres.Numeric(" + x + ")"
	case project.TypeTime:
		return "postgres.Timestamptz(" + x + ")"
	case project.TypeDate:
		return "postgres.Date(" + x + ")"
	case project.TypeUUID:
		return "postgres.UUID(" + x + ")"
	case project.TypeEnum:
		return "string(" + x + ")"
	}
	return x
}

// reportReader is the postgres function that reads a report's column of
// f's type from whatever sqlc made of it, and how its value becomes the
// row's: "%s" stands for the value read.
func reportReader(f *project.Field) (reader, convert string) {
	switch f.Type {
	case project.TypeInt:
		return "postgres.IntOf", "int(%s)"
	case project.TypeInt64:
		return "postgres.IntOf", "%s"
	case project.TypeDecimal:
		return "postgres.DecimalOf", "%s"
	case project.TypeMoney:
		return "postgres.MoneyOf", "%s"
	case project.TypeBool:
		return "postgres.BoolOf", "%s"
	case project.TypeTime, project.TypeDate:
		return "postgres.TimeOf", "%s"
	case project.TypeUUID:
		return "postgres.UUIDOf", "%s"
	case project.TypeEnum:
		return "postgres.TextOf", f.Enum.Pascal + "(%s)"
	}
	return "postgres.TextOf", "%s"
}

// reportSample is a SQL value of a column of f's type, for the SQL esquadro
// writes first.
func reportSample(f *project.Field) string {
	switch f.Type {
	case project.TypeInt, project.TypeInt64, project.TypeDecimal, project.TypeMoney:
		return "0"
	case project.TypeBool:
		return "false"
	case project.TypeTime, project.TypeDate:
		return "now()"
	case project.TypeUUID:
		return "gen_random_uuid()"
	case project.TypeEnum:
		return "'" + f.Enum.Values[0].Raw + "'"
	}
	return "''"
}

// reportImports are the imports a report's query and row types need.
func reportImports(r *project.Report) []string {
	var out []string
	add := func(p string) {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	for _, f := range append(append([]*project.Field{}, r.Params...), r.Columns...) {
		switch f.Type {
		case project.TypeTime, project.TypeDate:
			add("time")
		case project.TypeUUID:
			add("uuid")
		case project.TypeDecimal:
			add("github.com/shopspring/decimal")
		case project.TypeMoney:
			add("github.com/bojanz/currency")
		}
	}
	for _, f := range r.Params {
		if !f.Required && (f.Type == project.TypeInt || f.Type == project.TypeEnum) {
			add("github.com/jackc/pgx/v5/pgtype")
		}
	}
	return out
}

// IDLinks are the many-to-many relations an entity writes with its record,
// as lists of ids (tag_ids): those it declares.
func IDLinks(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range e.Relations {
		if r.Kind == project.ManyToMany && !r.Inferred {
			out = append(out, r)
		}
	}
	return out
}
