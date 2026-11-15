// Package blueprints holds the blueprints built into the esquadro binary, one
// folder per blueprint, each with a blueprint.esq manifest.
package blueprints

import "embed"

// FS contains the built-in blueprints: each one's manifest, templates and
// slots, and not its Go code or test data.
//
//go:embed platform/blueprint.esq all:platform/templates
//go:embed postgres/blueprint.esq all:postgres/templates postgres/slots
//go:embed api/blueprint.esq all:api/templates api/slots
//go:embed git/blueprint.esq
var FS embed.FS
