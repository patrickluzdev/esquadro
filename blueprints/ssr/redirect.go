package ssr

import (
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Redirect is an entity whose records redirect on a site
// (@site(redirect, by code, to target_url)): /<path>/<by> sends to <to>.
type Redirect struct {
	Entity *project.Entity
	// Path is where its addresses are, "" for the site's root.
	Path string
	// By is the unique field a record is found by; To, the field of where
	// it sends.
	By, To *project.Field
	// Wait is how many seconds a person sees where they are going before
	// going on (@site(redirect, wait 3)), 0 to go on at once.
	Wait int
}

// Route is the redirect's pattern on the site's mux: /{key} or
// /l/{key}.
func (r *Redirect) Route() string {
	if r.Path == "" {
		return "/{key}"
	}
	return "/" + r.Path + "/{key}"
}

// Redirects are the entities that redirect on a site (instance), in the
// model's order.
func Redirects(entities []*project.Entity, instance string) []*Redirect {
	var out []*Redirect
	for _, e := range entities {
		a := e.Ann[instance]
		if on, _ := a["redirect"].(bool); !on {
			continue
		}
		r := &Redirect{Entity: e, By: fieldNamed(e, a["by"]), To: fieldNamed(e, a["to"])}
		if s, _ := a["path"].(string); s != "" {
			r.Path = strings.Trim(s, "/")
		}
		if n, ok := a["wait"].(int64); ok {
			r.Wait = int(n)
		}
		out = append(out, r)
	}
	return out
}

// RedirectOf is the redirect of e on a site (instance), nil when its
// records do not redirect there.
func RedirectOf(entities []*project.Entity, instance string, e *project.Entity) *Redirect {
	for _, r := range Redirects(entities, instance) {
		if r.Entity == e {
			return r
		}
	}
	return nil
}

// checkRedirects checks a site's redirects: by a unique text field (it
// may be left empty for BeforeCreate to fill: a code made up), to a text field, not beside the entity's pages, and one at most at
// the site's root; a wait of 1 to 10 seconds, with redirect only.
func checkRedirects(p *project.Project, instance string) diag.List {
	var diags diag.List
	root := ""
	for _, e := range p.Entities {
		a := e.Ann[instance]
		on, _ := a["redirect"].(bool)
		if wait, ok := a["wait"].(int64); ok {
			switch {
			case !on:
				diags.Errorf(e.Pos, e.Pos, "@%s(wait) of %s is for a redirect: @%s(redirect, by code, to target_url, wait 3)", instance, e.Raw, instance)
			case wait < 1 || wait > 10:
				diags.Errorf(e.Pos, e.Pos, "@%s(redirect, wait %d) of %s: a person waits 1 to 10 seconds", instance, wait, e.Raw)
			}
		}
		if !on {
			continue
		}
		text := []project.Type{project.TypeString, project.TypeText}
		by, to := fieldNamed(e, a["by"]), fieldNamed(e, a["to"])
		switch {
		case a["by"] == nil:
			diags.Errorf(e.Pos, e.Pos, "@%s(redirect) of %s needs by, the unique field a record is found by: by code", instance, e.Raw)
		case by == nil:
			diags.Errorf(e.Pos, e.Pos, "@%s(redirect): by %v is not a field of %s", instance, a["by"], e.Raw)
		case !by.Unique || !slices.Contains(text, by.Type):
			diags.Errorf(by.Pos, by.Pos, "@%s(redirect): %s.%s must be a unique string, for a record to be found by it", instance, e.Raw, by.Raw)
		}
		switch {
		case a["to"] == nil:
			diags.Errorf(e.Pos, e.Pos, "@%s(redirect) of %s needs to, the field of where it sends: to target_url", instance, e.Raw)
		case to == nil:
			diags.Errorf(e.Pos, e.Pos, "@%s(redirect): to %v is not a field of %s", instance, a["to"], e.Raw)
		case !slices.Contains(text, to.Type):
			diags.Errorf(to.Pos, to.Pos, "@%s(redirect): %s.%s is %s; where a record sends is a string", instance, e.Raw, to.Raw, to.Type)
		}
		if page, _ := a["page"].(bool); page {
			diags.Errorf(e.Pos, e.Pos, "@%s(redirect, page) of %s: an entity's records either redirect or have pages on a site", instance, e.Raw)
		}
		if path, _ := a["path"].(string); strings.Trim(path, "/") == "" {
			if root != "" {
				diags.Errorf(e.Pos, e.Pos, "%s and %s both redirect at the root of the site %s: give one a path", root, e.Raw, instance)
			}
			root = e.Raw
		}
	}
	return diags
}

// canRedirectSample reports whether the generated test can write a record
// of a redirect by itself: one that needs no other record.
func canRedirectSample(r *Redirect) bool {
	if !store.CanSample(r.Entity) {
		return false
	}
	for _, rel := range r.Entity.Relations {
		if rel.OwnsForeignKey() && rel.Required {
			return false
		}
	}
	return true
}

// redirectSampleImports are what the samples of the redirects' records
// import (decimal, time...).
func redirectSampleImports(redirects []*Redirect) []string {
	var fields []*project.Field
	for _, r := range redirects {
		if canRedirectSample(r) {
			fields = append(fields, r.Entity.Fields...)
		}
	}
	return store.SampleImports(fields)
}

// redirectSamplePackages are the modules of the redirects the generated
// test writes records of, as Go imports them.
func redirectSamplePackages(appModule string, redirects []*Redirect) []string {
	var out []string
	for _, r := range redirects {
		p := appModule + "/internal/" + r.Entity.Module.Snake
		if canRedirectSample(r) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
