// Package spa holds the Go side of the spa blueprint: what its templates
// know of the model.
package spa

import (
	"fmt"
	"github.com/patrickluzdev/esquadro/blueprints/ui"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/audit"
	"github.com/patrickluzdev/esquadro/blueprints/rest"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/blueprints/storage"
	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the template helpers of the spa blueprint.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"spaKind":     Kind,
		"humanize":    ui.Humanize,
		"keys":        rest.Keys,
		"keyName":     rest.KeyName,
		"apiFilters":  rest.APIFilters,
		"sortsOf":     rest.SortsOf,
		"pagination":  rest.PaginationOf,
		"inRequest":   rest.InRequest,
		"inAnswer":    rest.InAnswer,
		"isTotal":     store.IsTotal,
		"orders":      store.Orders,
		"written":     store.Written,
		"realms":      security.RealmsOf,
		"softDeleted": store.SoftDeleted,
		"single":      store.Single,
		"importable":  store.Importable,
		"importBy":    store.ImportBy,
		"idLinks":     store.IDLinks,
		"richText":    rest.HTML,
		"audited":     audit.Audited,
		"auditedOf":   audit.AuditedOf,
		"auditAdmins": audit.Admins,
		"realmAdmins": security.Admins,
		"displayOf":   DisplayOf,
		"titleOf":     TitleOf,
		"lucide":      ui.Lucide,
		"uiWords":     ui.Words,
		"filterable":  Filterable,
		"fileMeta":    FileMeta,
		"enumColor":   ui.EnumColor,
		"names":       strs,
		"list":        func(items ...string) []string { return items },
		"slug":        ui.Slug,
		"dashboards":  Dashboards,
		"uiTheme":     ui.ThemeOf,
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
// declares, or the target's first text field (a comment's body), or its id.
func DisplayOf(r *project.Relation) string {
	if r.Display != nil && !r.Display.Implicit {
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

// Dashboards are the dashboards the reports of a frontend name, in the
// order the model names them first: the first page's is not among them.
func Dashboards(reports []*project.Report, instance string) []string {
	var out []string
	for _, r := range reports {
		a := r.Ann[instance]
		if a["widget"] == nil {
			continue
		}
		if d, _ := a["dashboard"].(string); d != "" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}
