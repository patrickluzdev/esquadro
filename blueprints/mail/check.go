package mail

import (
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/ui"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Check checks mail's options: its locale, and its brand, which an ssr
// site naming the same folder checks already.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	bp := set.Get("mail")
	if bp == nil {
		return nil
	}
	opts := set.Options["mail"]
	at := func(key string) diag.Pos {
		for _, o := range bp.Ref.Options {
			if o.Key.Name == key {
				return o.Pos
			}
		}
		return bp.Ref.Pos
	}
	if locale, _ := opts["locale"].(string); locale != "frontend" && !slices.Contains(ui.Locales, locale) {
		diags.Errorf(at("locale"), at("locale"), "locale %s is not %s, or frontend", locale, strings.Join(ui.Locales, ", "))
	}
	if strings.ContainsAny(stringOpt(opts, "address"), "\r\n") {
		diags.Errorf(at("address"), at("address"), "address is a single line")
	}
	dir := strings.Trim(stringOpt(opts, "brand"), "/")
	if dir == "" {
		return diags
	}
	b, err := ui.BrandOf(p.App.Pos.File, opts)
	shared := siteNames(set, dir) // the site tells what is wrong with it
	switch {
	case err != nil && !shared:
		diags.Errorf(at("brand"), at("brand"), "%v", err)
	case err == nil && !shared:
		for _, problem := range b.Problems() {
			diags.Errorf(at("brand"), at("brand"), "brand %s: %s", dir, problem)
		}
	}
	if err != nil {
		return diags
	}
	if file, _ := b.EmailLogo(); file == "" && b.Theme.Identity.Logo != "" {
		diags.Warnf(at("brand"), at("brand"), "brand %s: the emails write the app's name in place of the logo, %s, which email clients do not show",
			dir, b.Theme.Identity.Logo).Suggestion = `a PNG of it in theme.json: "identity": {"logo_email": "logo.png"}`
	}
	return diags
}

// siteNames reports whether an ssr site names the brand folder dir, and
// checks it.
func siteNames(set *blueprint.Set, dir string) bool {
	for _, f := range ui.Frontends(set) {
		if site, _ := f.Options["brand"].(string); f.Kind == "ssr" && strings.Trim(site, "/") == dir {
			return true
		}
	}
	return false
}

func stringOpt(opts map[string]any, key string) string {
	s, _ := opts[key].(string)
	return s
}
