package compose

import (
	"slices"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// goTypes maps field types to Go types. Enums use their own type.
var goTypes = map[project.Type]string{
	project.TypeString:  "string",
	project.TypeText:    "string",
	project.TypeInt:     "int",
	project.TypeInt64:   "int64",
	project.TypeDecimal: "decimal.Decimal",
	project.TypeBool:    "bool",
	project.TypeTime:    "time.Time",
	project.TypeDate:    "time.Time",
	project.TypeUUID:    "string",
	project.TypeJSON:    "json.RawMessage",
}

// goImportOf is the import each Go type needs.
var goImportOf = map[project.Type]string{
	project.TypeDecimal: "github.com/shopspring/decimal",
	project.TypeTime:    "time",
	project.TypeDate:    "time",
	project.TypeJSON:    "encoding/json",
}

// goType returns the Go type of a field.
func goType(f *project.Field) string {
	if f.Type == project.TypeEnum && f.Enum != nil {
		return f.Enum.Pascal
	}
	return goTypes[f.Type]
}

// goImports returns, sorted, the imports the Go types of an entity's fields
// need.
func goImports(e *project.Entity) []string {
	var out []string
	for _, f := range e.Fields {
		if imp := goImportOf[f.Type]; imp != "" && !slices.Contains(out, imp) {
			out = append(out, imp)
		}
	}
	slices.Sort(out)
	return out
}
