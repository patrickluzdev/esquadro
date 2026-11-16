package rest

import (
	"fmt"
	"slices"

	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// PaginationOf are the ways e's list pages: its @rest(pagination ...), or
// the blueprint's pagination option.
func PaginationOf(e *project.Entity, opts map[string]any) []string {
	if v, ok := e.Ann["rest"]["pagination"]; ok {
		return strs(v)
	}
	if v, ok := opts["pagination"]; ok {
		return strs(v)
	}
	return Paginations
}

// UnpagedMax is the most records e's list answers without pagination.
func UnpagedMax(e *project.Entity, opts map[string]any) int64 {
	if n, ok := e.Ann["rest"]["unpaged_max"].(int64); ok {
		return n
	}
	if n, ok := opts["unpaged_max"].(int64); ok {
		return n
	}
	return 1000
}

// APIFilter is a query parameter of a list that filters it.
type APIFilter struct {
	// Param is its name in the API: price_min, status.
	Param string
	// Schema is its schema in the contract, as a YAML flow mapping.
	Schema string
	// Kind is min, max, eq or in; Field is the domain filter's field it
	// sets (Min and Max are added for min and max); Go is the value's Go
	// type in the domain.
	Kind, Field, Go string
	// Array is whether the parameter is a list, written a,b,c.
	Array bool
}

// APIFilters are the filter parameters of e's list: _min and _max for a
// range (and the value itself, but for created_at), the value, or a list
// of values.
func APIFilters(e *project.Entity) []APIFilter {
	var out []APIFilter
	for _, f := range store.ListFilters(e) {
		schema := schemaOfGo(f.Go, f.SQLType)
		switch f.Kind {
		case "range":
			out = append(out,
				APIFilter{Param: f.Name + "_min", Schema: schema, Kind: "min", Field: f.Field, Go: f.Go},
				APIFilter{Param: f.Name + "_max", Schema: schema, Kind: "max", Field: f.Field, Go: f.Go})
			if f.Name != "created_at" {
				out = append(out, APIFilter{Param: f.Name, Schema: schema, Kind: "eq", Field: f.Field, Go: f.Go})
			}
		case "in":
			out = append(out, APIFilter{Param: f.Name, Kind: "in", Field: f.Field, Go: f.Go, Array: true,
				Schema: fmt.Sprintf(`{type: array, items: {$ref: "#/components/schemas/%s"}}`, enumOf(e, f.Field))})
		default:
			out = append(out, APIFilter{Param: f.Name, Schema: schema, Kind: "eq", Field: f.Field, Go: f.Go})
		}
	}
	return out
}

// enumOf is the contract's name of the enum of e's field.
func enumOf(e *project.Entity, field string) string {
	for _, f := range e.Fields {
		if f.Pascal == field && f.Enum != nil {
			return enumName(f.Enum)
		}
	}
	return ""
}

// schemaOfGo is the contract's schema of a filter's value.
func schemaOfGo(goType, sqlType string) string {
	switch {
	case goType == "decimal.Decimal":
		return `{type: string, description: 'A decimal number, e.g. "19.90".'}`
	case goType == "int":
		return "{type: integer}"
	case goType == "int64":
		return "{type: integer, format: int64}"
	case sqlType == "date":
		return "{type: string, format: date}"
	case goType == "time.Time":
		return "{type: string, format: date-time}"
	case goType == "bool":
		return "{type: boolean}"
	case goType == "uuid.UUID":
		return "{type: string, format: uuid}"
	}
	return "{type: string}"
}

// SortsOf are the orders e's list takes, as the store names them.
func SortsOf(e *project.Entity, opts map[string]any) []string {
	var out []string
	for _, s := range store.ListSorts(e, opts) {
		out = append(out, s.Key, "-"+s.Key)
	}
	if len(store.SearchFields(e)) > 0 {
		out = append(out, "relevance")
	}
	return out
}

// has reports whether list holds s.
func has(list []string, s string) bool { return slices.Contains(list, s) }
