// Package blueprints holds the blueprints built into the esquadro binary, one
// folder per blueprint, each with a blueprint.esq manifest.
package blueprints

import "embed"

// FS contains the built-in blueprints: each one's manifest, templates and
// slots, and not its Go code or test data.
//
//go:embed app/blueprint.esq all:app/templates
//go:embed domain/blueprint.esq all:domain/templates domain/slots
//go:embed http/blueprint.esq all:http/templates http/slots
//go:embed postgres/blueprint.esq all:postgres/templates postgres/slots
//go:embed compose/blueprint.esq all:compose/templates compose/slots
//go:embed migrations/blueprint.esq all:migrations/templates migrations/slots
//go:embed store/blueprint.esq all:store/templates store/slots
//go:embed testcontainers/blueprint.esq all:testcontainers/templates
//go:embed rest/blueprint.esq all:rest/templates rest/slots
//go:embed api_docs/blueprint.esq all:api_docs/templates api_docs/slots
//go:embed cors/blueprint.esq all:cors/templates cors/slots
//go:embed security_headers/blueprint.esq all:security_headers/templates security_headers/slots
//go:embed rate_limit/blueprint.esq all:rate_limit/templates rate_limit/slots
//go:embed compression/blueprint.esq all:compression/templates compression/slots
//go:embed i18n/blueprint.esq all:i18n/templates i18n/slots
//go:embed git/blueprint.esq
var FS embed.FS
