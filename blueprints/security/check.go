// Package security holds the Go code of the built-in security blueprint:
// what it checks in the model, and the helpers of its templates.
package security

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// The values the blueprint's names may take.
var (
	Signups    = []string{"open", "invite", "closed"}
	Providers  = []string{"password", "google", "github"}
	Verify     = []string{"required", "optional", "off"}
	Blocklists = []string{"none", "lists", "pwned"}
	Operations = []string{"list", "read", "create", "update", "delete", "write"}
)

// Realm is an entity that signs in, as the model declares it.
type Realm struct {
	Name          string // the entity's snake name: user, customer
	Entity        *project.Entity
	Signup        string
	Providers     []string
	Roles         []string
	Permissions   []string
	Grants        map[string][]string // role -> its own permissions
	VerifyEmail   string
	PasswordReset bool
	// The realm's own password rules; zero or empty takes the blueprint's.
	PasswordMin, PasswordMax int64
	Blocklist                string
}

// ServiceRealm is the realm of the service keys: rules name their roles
// as service.<role>.
const ServiceRealm = "service"

// Realms are the entities of the project marked @security(principal), with
// their settings and defaults.
func Realms(p *project.Project) []*Realm { return RealmsOf(p.Entities) }

// RealmsOf is Realms of a list of entities, for templates.
func RealmsOf(entities []*project.Entity) []*Realm {
	var out []*Realm
	for _, e := range entities {
		a := e.Ann["security"]
		if principal, _ := a["principal"].(bool); !principal {
			continue
		}
		r := &Realm{Name: e.Snake, Entity: e, Signup: "closed", Providers: []string{"password"}, VerifyEmail: "optional", PasswordReset: true, Grants: map[string][]string{}}
		if s, ok := a["signup"].(string); ok {
			r.Signup = s
		}
		if l := names(a["providers"]); l != nil {
			r.Providers = l
		}
		r.Roles = names(a["roles"])
		r.Permissions = names(a["permissions"])
		for _, g := range names(a["grants"]) {
			role, perm, _ := strings.Cut(g, ".")
			r.Grants[role] = append(r.Grants[role], perm)
		}
		if s, ok := a["verify_email"].(string); ok {
			r.VerifyEmail = s
		}
		if b, ok := a["password_reset"].(bool); ok {
			r.PasswordReset = b
		}
		r.PasswordMin, _ = a["password_min"].(int64)
		r.PasswordMax, _ = a["password_max"].(int64)
		r.Blocklist, _ = a["password_blocklist"].(string)
		out = append(out, r)
	}
	return out
}

func names(v any) []string {
	switch v := v.(type) {
	case []string:
		return v
	case []any:
		var out []string
		for _, x := range v {
			out = append(out, fmt.Sprint(x))
		}
		return out
	}
	return nil
}

// Check checks what the types of the annotation cannot: the names of
// signups, providers, roles and permissions, who the access rules name, and
// warns of the entities nobody restricts once something signs in.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	at := func(e *project.Entity, key string) dsl.Span {
		for _, a := range e.Annotations {
			if a.Name.Name != "security" {
				continue
			}
			for _, arg := range a.Args {
				if arg.Key.Name == key {
					return arg.Span
				}
			}
			return a.Span
		}
		return dsl.Span{Pos: e.Pos, End: e.Pos}
	}
	errorf := func(e *project.Entity, key, format string, args ...any) *diag.Diagnostic {
		s := at(e, key)
		return diags.Errorf(s.Pos, s.End, format, args...)
	}
	one := func(e *project.Entity, key, value string, allowed []string) {
		if value != "" && !slices.Contains(allowed, value) {
			errorf(e, key, "%s %q is not one of %s", key, value, strings.Join(allowed, ", "))
		}
	}

	realms := Realms(p)
	byName := map[string]*Realm{}
	if services := names(set.Options["security"]["services"]); len(services) > 0 {
		// The service keys' roles, which rules name as service.<role>.
		byName[ServiceRealm] = &Realm{Name: ServiceRealm, Roles: services}
		if ref := securityRef(p); len(realms) == 0 && ref != nil {
			diags.Errorf(ref.Pos, ref.Pos, "services needs a realm: service keys are created by the app's admins, who sign in to one").
				Suggestion = "mark the entity of the people who sign in @security(principal)"
		}
	}
	for _, r := range realms {
		if r.Name == ServiceRealm && byName[ServiceRealm] != nil {
			errorf(r.Entity, "principal", "%s is the realm of the service keys: rename the entity, or leave services out", ServiceRealm)
		}
		byName[r.Name] = r
		a := r.Entity.Ann["security"]
		one(r.Entity, "signup", r.Signup, Signups)
		one(r.Entity, "verify_email", r.VerifyEmail, Verify)
		if s, ok := a["password_blocklist"].(string); ok {
			one(r.Entity, "password_blocklist", s, Blocklists)
		}
		for _, pr := range r.Providers {
			one(r.Entity, "providers", pr, Providers)
		}
		for _, perm := range r.Permissions {
			if !strings.Contains(perm, ".") {
				errorf(r.Entity, "permissions", "permission %q should name what it is about, then the action: e.g. orders.refund", perm)
			}
		}
		for role, perms := range r.Grants {
			if !slices.Contains(r.Roles, role) {
				errorf(r.Entity, "grants", "grants gives permissions to %s, which is not a role of %s: %s", role, r.Name, strings.Join(r.Roles, ", "))
			}
			for _, perm := range perms {
				if !slices.Contains(r.Permissions, perm) {
					errorf(r.Entity, "grants", "grants gives %s.%s, but %s is not one of the realm's permissions", role, perm, perm)
				}
			}
		}
		passwordRules(a, func(key, format string, args ...any) { errorf(r.Entity, key, format, args...) })
	}

	for _, e := range p.Entities {
		a := e.Ann["security"]
		ruled, ownerChecked := false, false
		for _, op := range Operations {
			who := names(a[op])
			if who == nil {
				continue
			}
			ruled = true
			for _, w := range who {
				if w == "public" {
					continue
				}
				if w == "owner" {
					if principal, _ := a["principal"].(bool); principal && op == "create" {
						errorf(e, op, "create owner: a principal is created by sign-up, not by its owner")
					} else if _, why := OwnerOf(p.Entities, e); why != "" && !ownerChecked {
						errorf(e, op, "%s", why)
					}
					ownerChecked = true
					continue
				}
				realm, role, hasRole := strings.Cut(w, ".")
				r := byName[realm]
				switch {
				case r == nil:
					var known []string
					for _, k := range realms {
						known = append(known, k.Name)
					}
					errorf(e, op, "%s: %s is not public, owner, nor a realm (an entity marked @security(principal)): %s", op, w, strings.Join(append([]string{"public", "owner"}, known...), ", "))
				case hasRole && !slices.Contains(r.Roles, role):
					errorf(e, op, "%s: %s is not a role of %s, whose roles are %s", op, role, realm, strings.Join(r.Roles, ", "))
				}
			}
		}
		if !ruled && len(realms) > 0 {
			diags.Warnf(e.Pos, nameEnd(e), "%s has no @security(read, write...): whoever signed in, in any realm, may change it", e.Raw).
				Suggestion = fmt.Sprintf("say who may, e.g. @security(read public, write %s)", realms[0].Name)
		}
	}

	for _, rep := range p.Reports {
		end := diag.Pos{File: rep.Pos.File, Line: rep.Pos.Line, Col: rep.Pos.Col + len("report"), Offset: rep.Pos.Offset + len("report")}
		who := names(rep.Ann["security"]["read"])
		if who == nil {
			if len(realms) > 0 {
				diags.Errorf(rep.Pos, end, "report %s has no @security(read ...): say who may read its numbers", rep.Raw).
					Suggestion = fmt.Sprintf("e.g. @security(read %s)", realms[0].Name)
			}
			continue
		}
		for _, w := range who {
			if w == "public" {
				continue
			}
			realm, role, hasRole := strings.Cut(w, ".")
			r := byName[realm]
			switch {
			case w == "owner":
				diags.Errorf(rep.Pos, end, "report %s: owner is for records; a report's SQL filters what it shows", rep.Raw)
			case r == nil:
				diags.Errorf(rep.Pos, end, "report %s: %s is not public nor a realm (an entity marked @security(principal))", rep.Raw, w)
			case hasRole && !slices.Contains(r.Roles, role):
				diags.Errorf(rep.Pos, end, "report %s: %s is not a role of %s, whose roles are %s", rep.Raw, role, realm, strings.Join(r.Roles, ", "))
			}
		}
	}

	// A field only some may read: who they are, and nothing that would
	// tell its value to the others.
	for _, e := range p.Entities {
		for _, f := range e.Fields {
			who := names(f.Ann["security"]["read"])
			if who == nil {
				continue
			}
			end := diag.Pos{File: f.Pos.File, Line: f.Pos.Line, Col: f.Pos.Col + len(f.Raw), Offset: f.Pos.Offset + len(f.Raw)}
			if f.Ann["store"]["search"] == true {
				diags.Errorf(f.Pos, end, "%s.%s: a field only some may read cannot be in the search, which everyone who lists %s uses", e.Raw, f.Raw, e.Raw).
					Suggestion = "remove @store(search) from it, or its @security(read ...)"
			}
			for _, w := range who {
				if w == "public" {
					continue
				}
				realm, role, hasRole := strings.Cut(w, ".")
				r := byName[realm]
				switch {
				case w == "owner":
					diags.Errorf(f.Pos, end, "%s.%s: owner is for records; say a realm or realm.role", e.Raw, f.Raw)
				case r == nil:
					diags.Errorf(f.Pos, end, "%s.%s: %s is not public nor a realm (an entity marked @security(principal))", e.Raw, f.Raw, w)
				case hasRole && !slices.Contains(r.Roles, role):
					diags.Errorf(f.Pos, end, "%s.%s: %s is not a role of %s, whose roles are %s", e.Raw, f.Raw, role, realm, strings.Join(r.Roles, ", "))
				}
			}
		}
	}

	options(p, set, &diags)
	maintenance(p, set, realms, &diags)
	needs(p, set, realms, &diags)
	return diags
}

// needs checks what realms need: identities are kept in PostgreSQL, with
// their own migrations, principals are entities, and the emails of sign-up
// and password reset go by mail.
func needs(p *project.Project, set *blueprint.Set, realms []*Realm, diags *diag.List) {
	ref := securityRef(p)
	if len(realms) == 0 || ref == nil {
		return
	}
	var missing []string
	for _, name := range []string{"domain", "postgres", "migrations", "mail"} {
		if set.Get(name) == nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		diags.Errorf(ref.Pos, ref.Pos, "%s marked @security(principal), but security keeps identities with blueprints %s, and sends their emails with mail, which the app does not use", realms[0].Entity.Raw, strings.Join(missing, ", ")).
			Suggestion = "blueprint " + missing[0]
	}
	if origins := names(set.Options["http"]["cors_origins"]); slices.Contains(origins, "*") && len(realms) > 0 {
		diags.Warnf(ref.Pos, ref.Pos, `cors_origins ["*"] lets any site call the app, but browsers then send no cookies: pages of other origins sign in with tokens ("cookie": false), or list their origins`)
	}
}

func securityRef(p *project.Project) *project.BlueprintRef {
	for _, r := range p.App.Blueprints {
		if r.Name == "security" {
			return r
		}
	}
	return nil
}

// maintenance warns when nothing cleans up the expired sessions and links:
// jobs or the worker do it every hour.
func maintenance(p *project.Project, set *blueprint.Set, realms []*Realm, diags *diag.List) {
	if len(realms) == 0 || set.Get("jobs") != nil || set.Get("worker") != nil {
		return
	}
	if r := securityRef(p); r != nil {
		diags.Warnf(r.Pos, r.Pos, "nothing cleans up expired sessions and email links, nor rotates the signing keys: add blueprint jobs or worker, or run %s security cleanup on a schedule", p.App.Kebab).
			Suggestion = "blueprint worker"
	}
}

// options checks the blueprint's own options, in the app block.
func options(p *project.Project, set *blueprint.Set, diags *diag.List) {
	opts := set.Options["security"]
	ref := securityRef(p)
	if opts == nil || ref == nil {
		return
	}
	errorf := func(key, format string, args ...any) {
		s := dsl.Span{Pos: ref.Pos, End: ref.Pos}
		for _, o := range ref.Options {
			if o.Key.Name == key {
				s = o.Span
			}
		}
		diags.Errorf(s.Pos, s.End, format, args...)
	}
	if v, _ := opts["password_blocklist"].(string); !slices.Contains(Blocklists, v) {
		errorf("password_blocklist", "password_blocklist %q is not one of %s", v, strings.Join(Blocklists, ", "))
	}
	if v, _ := opts["pwned_fail"].(string); v != "open" && v != "closed" {
		errorf("pwned_fail", "pwned_fail %q is not open or closed", v)
	}
	passwordRules(opts, errorf)
	for _, path := range names(opts["password_lists"]) {
		// A path of the project: it goes in the Dockerfile as COPY <path>.
		clean := filepath.ToSlash(filepath.Clean(path))
		if path == "" || filepath.IsAbs(path) || clean != path || strings.HasPrefix(clean, "../") || clean == ".." ||
			strings.ContainsFunc(path, func(r rune) bool { return r <= ' ' || r == 0x7f || r == '"' || r == '\\' || r == '$' }) {
			errorf("password_lists", "password_lists %q: write a path of the project, relative and without spaces, e.g. config/passwords-br.txt", path)
		}
	}
	if m, _ := opts["argon2_memory_kib"].(int64); m < 7168 {
		errorf("argon2_memory_kib", "argon2_memory_kib %d is below OWASP's least (7168 KiB, with 5 iterations; 19456 with 2)", m)
	}
	if n, _ := opts["argon2_iterations"].(int64); n < 1 {
		errorf("argon2_iterations", "argon2_iterations must be at least 1")
	}
	idle, _ := opts["session_idle_hours"].(int64)
	maxDays, _ := opts["session_max_days"].(int64)
	days, _ := opts["session_days"].(int64)
	if days < 1 || idle < 1 || maxDays < days {
		errorf("session_max_days", "sessions need session_days and session_idle_hours of at least 1, and session_max_days (%d) not below session_days (%d)", maxDays, days)
	}
}

// passwordRules checks a realm's password settings against NIST's limits.
func passwordRules(a map[string]any, errorf func(key, format string, args ...any)) {
	minimum, hasMin := a["password_min"].(int64)
	maximum, hasMax := a["password_max"].(int64)
	if hasMin && minimum < 8 {
		errorf("password_min", "password_min %d is below 8, the least NIST allows for any password", minimum)
	}
	if hasMax && (maximum < 64 || maximum > 1024) {
		errorf("password_max", "password_max %d: allow at least 64 (NIST), and at most 1024", maximum)
	}
	if hasMin && hasMax && minimum > maximum {
		errorf("password_min", "password_min %d is above password_max %d", minimum, maximum)
	}
}

func nameEnd(e *project.Entity) diag.Pos {
	end := e.Pos
	end.Col += len("entity ") + len(e.Raw)
	end.Offset += len("entity ") + len(e.Raw)
	return end
}
