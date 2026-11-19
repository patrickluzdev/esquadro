// Package domain holds the Go code of the built-in domain blueprint: the
// helpers its templates use.
package domain

import (
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/store"
)

// TemplateFuncs are the helpers the domain blueprint's templates use.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		// written reports whether a record is written with a field:
		// declared, and not a total the store keeps.
		"written": store.Written,
	}
}
