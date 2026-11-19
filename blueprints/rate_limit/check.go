// Package ratelimit checks the limits of creates (@rate_limit(create
// "10/h")); the store blueprint writes them.
package ratelimit

import (
	"slices"

	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Check checks @rate_limit(create): N/min, N/h or N/d, on an entity the
// store keeps and creates, whose records have an owner (a relation to a
// principal, with an owner rule) to count them by, and which only a
// signed-in person may create.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	for _, e := range p.Entities {
		text := store.CreateLimitText(e)
		if text == "" {
			continue
		}
		s := span(e)
		errorf := func(format string, args ...any) { diags.Errorf(s.Pos, s.End, format, args...) }
		if _, _, err := store.ParseLimit(text); err != nil {
			errorf("@rate_limit(create) of %s: %v", e.Raw, err)
			continue
		}
		switch {
		case set.Get("store") == nil:
			errorf("@rate_limit(create) of %s counts the records in the database: add blueprint store", e.Raw)
			continue
		case store.Single(e):
			errorf("%s is single and has @rate_limit(create): its one record is never created", e.Raw)
			continue
		case set.Get("security") == nil:
			errorf("@rate_limit(create) of %s counts what each person created by the records they own: add blueprint security, and give %s an owner, e.g. customer > Customer required and @security(write owner)", e.Raw, e.Raw)
			continue
		}
		if principal, _ := e.Ann["security"]["principal"].(bool); principal {
			errorf("%s is a principal and has @rate_limit(create): its records are created by sign-up, which is limited by the rate of requests", e.Raw)
			continue
		}
		if _, ok := security.OwnerUsed(p.Entities, e); !ok {
			why := "give it an owner rule, e.g. @security(write owner)"
			if _, reason := security.OwnerOf(p.Entities, e); reason != "" {
				why = reason + ", and an owner rule, e.g. @security(write owner)"
			}
			errorf("@rate_limit(create) of %s counts what each person created by the records they own: %s", e.Raw, why)
			continue
		}
		if slices.Contains(security.Access(p.Entities, e, "create"), "public") {
			errorf("@rate_limit(create) of %s limits each person who signed in, and anyone may create %s (public): let only a realm create them, e.g. @security(create owner)", e.Raw, e.Plural.Snake)
		}
	}
	return diags
}

// span is where an entity's @rate_limit is, or the entity.
func span(e *project.Entity) dsl.Span {
	for _, a := range e.Annotations {
		if a.Name.Name != "rate_limit" {
			continue
		}
		for _, arg := range a.Args {
			if arg.Key.Name == "create" {
				return arg.Span
			}
		}
		return a.Span
	}
	return dsl.Span{Pos: e.Pos, End: e.Pos}
}
