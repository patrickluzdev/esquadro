// Package apps holds the Go side of the platform's apps blueprint.
package apps

import (
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Check refuses an app with database on a platform without PostgreSQL.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	if set.Get("postgres") != nil {
		return nil
	}
	for _, h := range p.Hosted {
		if h.Database {
			diags.Errorf(h.Pos, diag.Pos{}, "app %s has database, and the platform has no PostgreSQL", h.Raw).
				Suggestion = "add it: esquadro add postgres"
		}
	}
	return diags
}
