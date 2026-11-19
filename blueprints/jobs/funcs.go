// Package jobs holds the Go helpers of the jobs blueprint's templates.
package jobs

import (
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the helpers the jobs blueprint's templates use.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"sample":        store.Sample,
		"canSample":     canSample,
		"sampleImports": sampleImports,
		"refusesEmpty":  refusesEmpty,
		"uniqueOf":      UniqueOf,
		"uniqueBy":      UniqueBy,
	}
}

// canSample reports whether a test can make valid arguments for a job: not
// when a required string has a pattern.
func canSample(j *project.Job) bool {
	for _, f := range j.Fields {
		if f.Required && f.Pattern != "" && (f.Type == project.TypeString || f.Type == project.TypeText) {
			return false
		}
	}
	return true
}

// refusesEmpty reports whether a job's empty arguments are invalid: a
// required argument whose zero value is refused.
func refusesEmpty(j *project.Job) bool {
	for _, f := range j.Fields {
		if !f.Required || f.Default != nil {
			continue
		}
		switch f.Type {
		case project.TypeInt, project.TypeInt64, project.TypeDecimal, project.TypeBool:
		default:
			return true
		}
	}
	return false
}

// sampleImports are the imports the tests of a module's jobs need for their
// sample arguments.
func sampleImports(m *project.Module) []string {
	var fields []*project.Field
	for _, j := range m.Jobs {
		if canSample(j) {
			for _, f := range j.Fields {
				if f.Required {
					fields = append(fields, f)
				}
			}
		}
	}
	out := store.SampleImports(fields)
	for _, f := range fields {
		if f.Type == project.TypeUUID {
			return append(out, "uuid")
		}
	}
	return out
}
