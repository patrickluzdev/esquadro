package store

import (
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/postgres"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// ListFilter is a way a list filters an entity's records: by a range, a
// value, or a list of values.
type ListFilter struct {
	// Name is the filter's name, in snake case: price, category_id.
	Name string
	// Column is the column it filters, quoted when a keyword.
	Column string
	// Kind is range (numbers and dates: Min and Max), eq (a value) or in
	// (enums: a list).
	Kind string
	// SQLType is the type of its parameters.
	SQLType string
	// Go is the Go type of its value, Field the domain field it filters.
	Go, Field string
	// Conv turns a Go value into its parameter (postgres.Numeric...).
	Conv string
}

// ListSort is an order a list takes: by creation, or by a field.
type ListSort struct {
	// Key is how the API names it: created_at, price.
	Key string
	// Pascal names its queries: ListProductsByPriceAsc.
	Pascal string
	// Column is the column it orders by; Cast turns a cursor's text into
	// its type.
	Column, Cast string
	// Value is the Go expression of x's value as a cursor holds it.
	Value string
}

// rangeTypes are the types lists filter by a range.
var rangeTypes = map[project.Type][3]string{
	// SQL type, Go type, conversion
	project.TypeInt:     {"integer", "int", "postgres.Int4"},
	project.TypeInt64:   {"bigint", "int64", "postgres.Int8"},
	project.TypeDecimal: {"numeric", "decimal.Decimal", "postgres.Numeric"},
	project.TypeDate:    {"date", "time.Time", "postgres.Date"},
	project.TypeTime:    {"timestamptz", "time.Time", "postgres.Timestamptz"},
}

// ListFilters are the filters of e's lists: by creation, always, then the
// fields and relations marked @store(filter).
func ListFilters(e *project.Entity) []ListFilter {
	out := []ListFilter{{Name: "created_at", Column: "created_at", Kind: "range", SQLType: "timestamptz", Go: "time.Time", Field: "CreatedAt", Conv: "postgres.Timestamptz"}}
	for _, f := range e.Fields {
		if on, _ := f.Ann["store"]["filter"].(bool); !on || f.Implicit {
			continue
		}
		col := pgColumn(f)
		lf := ListFilter{Name: f.Snake, Column: postgres.Ident(col), Field: f.Pascal}
		if r, ok := rangeTypes[f.Type]; ok {
			lf.Kind, lf.SQLType, lf.Go, lf.Conv = "range", r[0], r[1], r[2]
		} else {
			switch f.Type {
			case project.TypeEnum:
				lf.Kind, lf.SQLType, lf.Go = "in", "text[]", f.Enum.Pascal
			case project.TypeBool:
				lf.Kind, lf.SQLType, lf.Go, lf.Conv = "eq", "boolean", "bool", "postgres.Bool"
			case project.TypeUUID:
				lf.Kind, lf.SQLType, lf.Go, lf.Conv = "eq", "uuid", "uuid.UUID", "postgres.UUID"
			default:
				lf.Kind, lf.SQLType, lf.Go, lf.Conv = "eq", "text", "string", "postgres.Text"
			}
		}
		out = append(out, lf)
	}
	for _, r := range e.Relations {
		if on, _ := r.Ann["store"]["filter"].(bool); !on || !r.OwnsForeignKey() {
			continue
		}
		out = append(out, ListFilter{Name: r.Snake + "_id", Column: postgres.Ident(r.Snake + "_id"), Kind: "eq", SQLType: "uuid",
			Go: "uuid.UUID", Field: r.Pascal + "ID", Conv: "postgres.UUID"})
	}
	return out
}

// ListSorts are the orders of e's lists: by creation, then by each field
// marked @store(sort). opts are the postgres blueprint's options, for the
// type of an enum.
func ListSorts(e *project.Entity, opts map[string]any) []ListSort {
	out := []ListSort{{Key: "created_at", Pascal: "CreatedAt", Column: "created_at", Cast: "timestamptz", Value: "x.CreatedAt.UTC().Format(time.RFC3339Nano)"}}
	for _, f := range e.Fields {
		if on, _ := f.Ann["store"]["sort"].(bool); !on || f.Implicit {
			continue
		}
		s := ListSort{Key: f.Snake, Pascal: f.Pascal, Column: postgres.Ident(pgColumn(f))}
		x := "x." + f.Pascal
		switch f.Type {
		case project.TypeInt:
			s.Cast, s.Value = "integer", "strconv.Itoa("+x+")"
		case project.TypeInt64:
			s.Cast, s.Value = "bigint", "strconv.FormatInt("+x+", 10)"
		case project.TypeDecimal:
			s.Cast, s.Value = "numeric", x+".String()"
		case project.TypeDate:
			s.Cast, s.Value = "date", x+".Format(time.DateOnly)"
		case project.TypeTime:
			s.Cast, s.Value = "timestamptz", x+".UTC().Format(time.RFC3339Nano)"
		case project.TypeBool:
			s.Cast, s.Value = "boolean", "strconv.FormatBool("+x+")"
		case project.TypeUUID:
			s.Cast, s.Value = "uuid", x+".String()"
		case project.TypeEnum:
			s.Cast, s.Value = enumType(f.Enum, opts), "string("+x+")"
		default:
			s.Cast, s.Value = "text", x
		}
		out = append(out, s)
	}
	return out
}

// enumType is an enum's type in PostgreSQL, as the migrations name it.
func enumType(en *project.Enum, opts map[string]any) string {
	if ownSchema(en.Module, opts) {
		return en.Module.Snake + "." + en.Snake
	}
	return "public." + en.Module.Snake + "_" + en.Snake
}

// SearchFields are the columns of e's fields marked @store(search).
func SearchFields(e *project.Entity) []string {
	var out []string
	for _, f := range e.Fields {
		if on, _ := f.Ann["store"]["search"].(bool); on {
			out = append(out, pgColumn(f))
		}
	}
	return out
}

// listImports are the imports a list's Go code needs for its filters and
// sorts.
func listImports(entities []*project.Entity, e *project.Entity, opts map[string]any) []string {
	var out []string
	add := func(p string) {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	add("time") // created_at
	if ownerColumnOf(entities, e) != "" {
		add("uuid")
	}
	for _, f := range ListFilters(e) {
		switch {
		case strings.HasPrefix(f.Go, "decimal."):
			add("github.com/shopspring/decimal")
		case f.Go == "uuid.UUID":
			add("uuid")
		}
	}
	for _, s := range ListSorts(e, opts) {
		if strings.HasPrefix(s.Value, "strconv.") {
			add("strconv")
		}
	}
	if len(SearchFields(e)) > 0 {
		add("strconv") // the relevance in a cursor
	}
	return out
}

// ListWhere is the WHERE of e's list queries: each filter, the owner and
// the search, NULL when not asked.
func ListWhere(entities []*project.Entity, e *project.Entity, opts map[string]any, lang string) string {
	var conds []string
	if col := ownerColumnOf(entities, e); col != "" {
		conds = append(conds, "(sqlc.narg(owner_id)::uuid IS NULL OR "+col+" = sqlc.narg(owner_id)::uuid)")
	}
	if len(SearchFields(e)) > 0 {
		conds = append(conds, "(sqlc.narg(q)::text IS NULL OR "+searchColumn+" @@ websearch_to_tsquery("+langOf(lang)+", sqlc.narg(q)::text) OR "+searchColumn+" @@ "+prefixQuery(lang)+")")
	}
	for _, f := range ListFilters(e) {
		switch f.Kind {
		case "range":
			conds = append(conds,
				"(sqlc.narg("+f.Name+"_min)::"+f.SQLType+" IS NULL OR "+f.Column+" >= sqlc.narg("+f.Name+"_min)::"+f.SQLType+")",
				"(sqlc.narg("+f.Name+"_max)::"+f.SQLType+" IS NULL OR "+f.Column+" <= sqlc.narg("+f.Name+"_max)::"+f.SQLType+")")
		case "in":
			conds = append(conds, "(sqlc.narg("+f.Name+")::text[] IS NULL OR "+f.Column+"::text = ANY(sqlc.narg("+f.Name+")::text[]))")
		default:
			conds = append(conds, "(sqlc.narg("+f.Name+")::"+f.SQLType+" IS NULL OR "+f.Column+" = sqlc.narg("+f.Name+")::"+f.SQLType+")")
		}
	}
	return "WHERE " + strings.Join(conds, "\n  AND ")
}

// Rank is the relevance of a record to the search, for its order.
func Rank(lang string) string {
	return "ts_rank(" + searchColumn + ", websearch_to_tsquery(" + langOf(lang) + ", sqlc.narg(q)::text) || " + prefixQuery(lang) + ")::real"
}

// prefixQuery is the search as words being typed: each word the start of
// one in the record ("tecl" finds "teclado"), all of them in it, and none
// of those after a minus ("-fio"). Each word is quoted, so that what is
// typed is never tsquery syntax.
func prefixQuery(lang string) string {
	return "coalesce(to_tsquery(" + langOf(lang) + ", (SELECT string_agg(CASE WHEN neg THEN '!' ELSE '' END || quote_literal(word) || ':*', ' & ') " +
		"FROM (SELECT left(w, 1) = '-' AS neg, regexp_replace(w, '^-|[\"()&|!:*<>]', '', 'g') AS word " +
		"FROM regexp_split_to_table(trim(sqlc.narg(q)::text), '\\s+') AS w) AS words WHERE word <> '')), ''::tsquery)"
}

const searchColumn = "search_vector"

func langOf(lang string) string {
	if lang == "" {
		lang = "simple"
	}
	return "'" + lang + "'::regconfig"
}

// ownerColumnOf is the column of the owner of e's records, when a rule of
// security is owner; empty without.
func ownerColumnOf(entities []*project.Entity, e *project.Entity) string {
	o, ok := security.OwnerUsed(entities, e)
	switch {
	case !ok:
		return ""
	case o.Relation == nil:
		return "id"
	}
	return postgres.Ident(o.Relation.Snake + "_id")
}
