// Package blueprints holds the blueprints built into the esquadro binary, one
// folder per blueprint, each with a blueprint.esq manifest.
package blueprints

import "embed"

// FS contains the built-in blueprints: each one's manifest, templates and
// slots, and not its Go code or test data.
//
// The blueprints only for platforms are in platform/.
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
//go:embed rate_limit/blueprint.esq all:rate_limit/templates rate_limit/slots
//go:embed compression/blueprint.esq all:compression/templates compression/slots
//go:embed i18n/blueprint.esq all:i18n/templates i18n/slots
//go:embed worker/blueprint.esq all:worker/templates worker/slots
//go:embed jobs/blueprint.esq all:jobs/templates jobs/slots
//go:embed observability/blueprint.esq all:observability/templates observability/slots
//go:embed docker/blueprint.esq all:docker/templates docker/slots
//go:embed github_actions/blueprint.esq all:github_actions/templates
//go:embed branching/blueprint.esq all:branching/templates branching/slots
//go:embed env/blueprint.esq all:env/templates env/slots
//go:embed security/blueprint.esq all:security/templates security/slots security/partials
//go:embed mail/blueprint.esq all:mail/templates mail/slots all:mail/files
//go:embed storage/blueprint.esq all:storage/templates storage/slots
//go:embed images/blueprint.esq all:images/templates
//go:embed integration/blueprint.esq all:integration/templates integration/slots
//go:embed seed/blueprint.esq all:seed/templates seed/slots
//go:embed audit/blueprint.esq all:audit/templates audit/slots
//go:embed notifications/blueprint.esq all:notifications/templates notifications/slots
//go:embed client_ts/blueprint.esq all:client_ts/templates client_ts/slots
//go:embed git/blueprint.esq
//go:embed deploy/blueprint.esq deploy/slots
//go:embed workspace/blueprint.esq all:workspace/templates workspace/slots
//go:embed ui/blueprint.esq all:ui/templates
//go:embed spa/blueprint.esq all:spa/templates
//go:embed ssr/blueprint.esq all:ssr/templates ssr/slots
//go:embed platform/server/blueprint.esq all:platform/server/templates
//go:embed platform/caddy/blueprint.esq all:platform/caddy/templates platform/caddy/slots
//go:embed platform/postgres/blueprint.esq all:platform/postgres/templates platform/postgres/slots
//go:embed platform/apps/blueprint.esq all:platform/apps/templates platform/apps/slots
//go:embed platform/observability/blueprint.esq all:platform/observability/templates platform/observability/slots
//go:embed platform/cloudflare/blueprint.esq all:platform/cloudflare/templates platform/cloudflare/slots
//go:embed platform/backup/blueprint.esq all:platform/backup/templates platform/backup/slots
//go:embed platform/github_actions/blueprint.esq all:platform/github_actions/templates
//go:embed platform/mail/blueprint.esq all:platform/mail/templates platform/mail/slots
var FS embed.FS
