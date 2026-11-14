package postgres

import (
	"fmt"
	"strings"
	"text/template"

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
		"pgTable":     pgTable,
		"pgColumn":    pgColumn,
		"sqlcModel":   sqlcModel,
		"sqlcField":   sqlcField,
		"sqlcIn":      sqlcIn,
		"sqlcOut":     sqlcOut,
		"sqlcEnum":    sqlcEnum,
		"ownSchema":   ownSchema,
		"keyColumn":   func(r *project.Relation) string { return r.Snake + "_id" },
		"keyField":    func(r *project.Relation) string { return names(r.Snake + "_id").Pascal },
		"keyIn":       keyIn,
		"keyOut":      keyOut,
		"needs":       needs,
		"constraint":  constraint,
		"pgIdent":     Ident,
		"writable":    writable,
		"refusals":    refusals,
		"contains":    strings.Contains,
		"initialisms": project.Initialisms,
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
	return f.Snake
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
	case project.TypeBool:
		if null {
			return "postgres.Bool(" + expr + ")"
		}
		return expr
	case project.TypeTime:
		return "postgres.Timestamptz(" + expr + ")"
	case project.TypeDate:
		return "postgres.Date(" + expr + ")"
	case project.TypeUUID:
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
	case project.TypeBool:
		if null {
			return expr + ".Bool"
		}
		return expr
	case project.TypeTime, project.TypeDate:
		return expr + ".Time"
	case project.TypeUUID:
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
}

// In converts the domain value of the column, read from the struct named
// v (a NewX), to sqlc's parameter.
func (w Writable) In(v string) string {
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
		out = append(out, Writable{Column: Ident(c), Param: c, GoParam: names(c).Pascal, Field: f})
	}
	for _, r := range e.Relations {
		if r.OwnsForeignKey() {
			c := r.Snake + "_id"
			out = append(out, Writable{Column: Ident(c), Param: c, GoParam: names(c).Pascal, Relation: r})
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
