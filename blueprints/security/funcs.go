package security

import (
	"slices"
	"text/template"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the helpers of the security blueprint's templates.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"securityTable": Table,
		"realms":        RealmsOf,
		"usesProvider":  UsesProvider,
	}
}

// UsesProvider reports whether a realm of entities signs in with provider.
func UsesProvider(entities []*project.Entity, provider string) bool {
	for _, r := range RealmsOf(entities) {
		if slices.Contains(r.Providers, provider) {
			return true
		}
	}
	return false
}

// Table is the name of one of the security tables: security_<name> in the
// public schema, or security.<name> when the postgres blueprint's schemas
// option has security.
func Table(name string, postgresOptions map[string]any) string {
	if slices.Contains(names(postgresOptions["schemas"]), "security") {
		return "security." + name
	}
	return "security_" + name
}
