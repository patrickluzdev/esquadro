package storage

import (
	"slices"
	"text/template"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the storage blueprint's template helpers.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"fileFields": FileFields,
		// has reports whether a list of the options holds s.
		"has": func(list any, s string) bool { return slices.Contains(strs(list), s) },
	}
}

// FileField is a file field of the model, with its rules worked out.
type FileField struct {
	// Key is how the store names it: the entity's plural and the field,
	// products.photo.
	Key    string
	Entity *project.Entity
	Field  *project.Field
	Image  bool
	Rules  Rules
}

// FileFields lists the model's file and image fields, entity by entity,
// with the rules the blueprint's options and their @storage give them.
func FileFields(entities []*project.Entity, opts map[string]any) []FileField {
	var out []FileField
	for _, e := range entities {
		for _, f := range e.Fields {
			if f.IsFile() {
				out = append(out, FileField{
					Key: e.Plural.Snake + "." + f.Snake, Entity: e, Field: f,
					Image: f.Type == project.TypeImage, Rules: RulesOf(f, opts),
				})
			}
		}
	}
	return out
}
