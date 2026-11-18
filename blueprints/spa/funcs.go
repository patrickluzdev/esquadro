// Package spa holds the Go side of the spa blueprint: what its templates
// know of the model.
package spa

import (
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/rest"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the template helpers of the spa blueprint.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"spaKind":    Kind,
		"humanize":   Humanize,
		"keys":       rest.Keys,
		"keyName":    rest.KeyName,
		"apiFilters": rest.APIFilters,
		"sortsOf":    rest.SortsOf,
		"pagination": rest.PaginationOf,
		"inRequest":  rest.InRequest,
		"realms":     security.RealmsOf,
		"displayOf":  DisplayOf,
		"lucide":     Lucide,
		"add1":       func(n int) int { return n + 1 },
		"enumValues": func(e *project.Enum) []string {
			out := make([]string, len(e.Values))
			for i, v := range e.Values {
				out[i] = v.Raw
			}
			return out
		},
	}
}

// Kind is how the frontend shows and edits a field: text, textarea, int,
// decimal, bool, date, datetime, enum, uuid or json.
func Kind(f *project.Field) string {
	switch f.Type {
	case project.TypeString:
		return "text"
	case project.TypeText:
		return "textarea"
	case project.TypeInt, project.TypeInt64:
		return "int"
	case project.TypeDecimal:
		return "decimal"
	case project.TypeBool:
		return "bool"
	case project.TypeDate:
		return "date"
	case project.TypeTime:
		return "datetime"
	case project.TypeEnum:
		return "enum"
	case project.TypeUUID:
		return "uuid"
	}
	return "json"
}

// Humanize is a name as people read it: due_on is "Due on".
func Humanize(name string) string {
	s := strings.ReplaceAll(name, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Lucide is the component of a Lucide icon's name: shopping-cart is
// ShoppingCart; Circle without one.
func Lucide(name any) string {
	s, _ := name.(string)
	if s == "" {
		return "Circle"
	}
	var b strings.Builder
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == ' ' }) {
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

// DisplayOf is the field a relation's target is shown by: the one it
// declares, or the target's first text field, or its id.
func DisplayOf(r *project.Relation) string {
	if r.Display != nil {
		return r.Display.Snake
	}
	for _, f := range r.Target.Fields {
		if !f.Implicit && (f.Type == project.TypeString || f.Type == project.TypeText) {
			return f.Snake
		}
	}
	return "id"
}
