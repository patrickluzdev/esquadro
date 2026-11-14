// Package blueprints holds the blueprints built into the esquadro binary, one
// folder per blueprint, each with a blueprint.esq manifest.
package blueprints

import "embed"

// FS contains the built-in blueprints.
//
//go:embed all:platform all:postgres
var FS embed.FS
