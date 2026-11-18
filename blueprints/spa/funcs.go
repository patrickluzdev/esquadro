// Package spa holds the Go side of the spa blueprint: what its templates
// know of the model.
package spa

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/rest"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/blueprints/storage"
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
		"titleOf":    TitleOf,
		"lucide":     Lucide,
		"filterable": Filterable,
		"fileMeta":   FileMeta,
		"enumColor":  EnumColor,
		"rangeFilterable": func(e *project.Entity, name string) bool {
			for _, f := range rest.APIFilters(e) {
				if f.Param == name+"_min" {
					return true
				}
			}
			return false
		},
		"hasMany":    HasMany,
		"defaultOf":  DefaultOf,
		"inverseKey": InverseKey,
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
// decimal, money, file, image, bool, date, datetime, enum, uuid or json.
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
	case project.TypeMoney:
		return "money"
	case project.TypeFile:
		return "file"
	case project.TypeImage:
		return "image"
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

// Filterable reports whether the API filters the entity's list by the
// value of a field or a relation's key (param, e.g. status or category_id):
// the filters a list shows are those.
func Filterable(e *project.Entity, param string) bool {
	for _, f := range rest.APIFilters(e) {
		if f.Param == param && (f.Kind == "eq" || f.Kind == "in") {
			return true
		}
	}
	return false
}

// DefaultOf is a field's default in the model as a TypeScript literal, for
// a new record's form to start with: "ACTIVE", true, 3; empty without one
// (or when it is now, which the API sets).
func DefaultOf(f *project.Field) string {
	d := f.Default
	if d == nil || d.Kind == project.DefaultNow {
		return ""
	}
	switch Kind(f) {
	case "bool", "int":
		return d.Text
	case "money":
		// Its currency is the app's, which the API fills in.
		return "{ amount: " + strconv.Quote(d.Text) + " }"
	}
	return strconv.Quote(d.Text)
}

// HasMany reports whether a relation is the "many" side of another entity's
// key (Category's products), whose records a page can list.
func HasMany(r *project.Relation) bool {
	return r.Kind == project.OneToMany && r.Inverse != nil
}

// InverseKey is the key, on the related entity, that points back at the
// relation's entity: category_id for Category's products.
func InverseKey(r *project.Relation) string {
	return rest.KeyName(r.Inverse)
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

// TitleOf is the field an entity's records are called by: its first text
// field, or id.
func TitleOf(e *project.Entity) string {
	for _, f := range e.Fields {
		if !f.Implicit && (f.Type == project.TypeString || f.Type == project.TypeText) {
			return f.Snake
		}
	}
	return "id"
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

// FileMeta is what a frontend needs of a file field, as the rest of a
// TypeScript literal: what to send it for (the entity's plural and the
// field), the formats it takes and its largest size, from the storage
// blueprint's rules. Empty for other fields.
func FileMeta(e *project.Entity, f *project.Field, opts map[string]any) string {
	if !f.IsFile() {
		return ""
	}
	r := storage.RulesOf(f, opts)
	accept := make([]string, len(r.Types))
	for i, t := range r.Types {
		accept[i] = strconv.Quote(t)
	}
	return fmt.Sprintf(", upload: { entity: %s, field: %s }, accept: [%s], maxSize: %d",
		strconv.Quote(e.Plural.Snake), strconv.Quote(f.Snake), strings.Join(accept, ", "), r.Max)
}

// colorWords guess a value's color from the words of its name.
var colorWords = []struct {
	color string
	words []string
}{
	{"red", []string{"CANCEL", "FAIL", "ERROR", "REJECT", "BLOCK", "BAN", "DENI", "EXPIRE", "OVERDUE", "LOST", "DELETED", "REFUND"}},
	{"amber", []string{"PENDING", "DRAFT", "WAIT", "REVIEW", "HOLD", "PAUS", "PROCESS", "PARTIAL", "TRIAL", "SCHEDUL", "LOW"}},
	{"green", []string{"ACTIVE", "ENABLE", "PAID", "DONE", "COMPLETE", "APPROV", "PUBLISH", "SUCCESS", "OPEN", "ONLINE", "VERIFIED", "AVAILABLE", "DELIVERED", "WON", "IN_STOCK"}},
	{"blue", []string{"NEW", "SENT", "SHIPPED", "PROGRESS", "RUNNING", "CONFIRM"}},
}

// EnumColor is the color of an enum value's dot: its own (@<spa>(color
// ...)), or a guess from its name; gray when none fits.
func EnumColor(v *project.EnumValue, ann map[string]any) string {
	if c, ok := ann["color"].(string); ok && c != "" {
		return c
	}
	name := strings.ToUpper(v.Raw)
	// INACTIVE, DISABLED, UNPAID: a negation is not the word it negates.
	if strings.HasPrefix(name, "IN") && strings.Contains(name, "ACTIVE") || strings.HasPrefix(name, "DIS") || strings.HasPrefix(name, "UN") || strings.HasPrefix(name, "NOT_") {
		return "gray"
	}
	for _, c := range colorWords {
		for _, w := range c.words {
			if strings.Contains(name, w) {
				return c.color
			}
		}
	}
	return "gray"
}
