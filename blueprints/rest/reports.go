package rest

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// ReportSchema is the contract's schema of a report's column or
// parameter; nullable for a column that may be NULL. An enum is text, as
// its values belong to the report's module.
func ReportSchema(f *project.Field, nullable bool) string {
	s := ""
	switch f.Type {
	case project.TypeInt:
		s = "type: integer"
	case project.TypeInt64:
		s = "type: integer, format: int64"
	case project.TypeDecimal:
		s = `type: string, description: 'A decimal number, e.g. "19.90".'`
	case project.TypeBool:
		s = "type: boolean"
	case project.TypeTime:
		s = "type: string, format: date-time"
	case project.TypeDate:
		s = "type: string, format: date"
	case project.TypeUUID:
		s = "type: string, format: uuid"
	case project.TypeEnum:
		values := make([]string, len(f.Enum.Values))
		for i, v := range f.Enum.Values {
			values[i] = v.Raw
		}
		s = fmt.Sprintf("type: string, description: %q", "One of "+strings.Join(values, ", ")+".")
	default:
		s = "type: string"
	}
	if nullable {
		s += ", nullable: true"
	}
	return "{" + s + "}"
}

// ReportIn is the Go expression that turns the value v ogen read of a
// report's parameter into the module's; decimals are parsed apart.
func ReportIn(f *project.Field, v string) string {
	switch f.Type {
	case project.TypeEnum:
		return f.Enum.Pascal + "(" + v + ")"
	case project.TypeUUID:
		return "uuid.UUID(" + v + ")"
	}
	return v
}

// ReportOut is the Go expression of a report's column value x as the
// contract writes it.
func ReportOut(f *project.Field, x string) string {
	switch f.Type {
	case project.TypeEnum:
		return "string(" + x + ")"
	case project.TypeDecimal:
		return x + ".String()"
	case project.TypeUUID:
		return "ogenuuid.UUID(" + x + ")"
	}
	return x
}

// ReportRows is the most rows a report answers through the API: its
// @rest(unpaged_max), else the rest blueprint's unpaged_max.
func ReportRows(r *project.Report, opts map[string]any) int64 {
	if n, ok := r.Ann["rest"]["unpaged_max"].(int64); ok {
		return n
	}
	n, _ := opts["unpaged_max"].(int64)
	return n
}

// ReportSampleQuery is a query string with a valid value for each of the
// report's required parameters, for the generated tests.
func ReportSampleQuery(r *project.Report) string {
	q := url.Values{}
	for _, f := range r.Params {
		if !f.Required {
			continue
		}
		v := "x"
		switch f.Type {
		case project.TypeInt, project.TypeInt64, project.TypeDecimal:
			v = "1"
		case project.TypeBool:
			v = "true"
		case project.TypeTime:
			v = "2026-01-01T00:00:00Z"
		case project.TypeDate:
			v = "2026-01-01"
		case project.TypeUUID:
			v = "0198f000-0000-7000-8000-000000000000"
		case project.TypeEnum:
			v = f.Enum.Values[0].Raw
		}
		q.Set(f.Snake, v)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// ReportPublic reports whether anyone may read the report, signed in or
// not.
func ReportPublic(r *project.Report) bool {
	who, _ := r.Ann["security"]["read"].([]any)
	for _, w := range who {
		if w == "public" {
			return true
		}
	}
	return false
}
