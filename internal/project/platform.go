package project

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
)

// Kind is the kind of a project: an app, made of code, or a platform, the
// servers and services the apps run on.
type Kind string

const (
	KindApp      Kind = "app"
	KindPlatform Kind = "platform"
)

// Server is a machine of a platform, reached by SSH.
type Server struct {
	Names
	Doc []string
	// Host is the address the platform reaches the server at.
	Host string
	Pos  diag.Pos
}

// Hosted is an app a platform runs: an image of an app project, the
// domains it answers on, and what it needs from the platform.
type Hosted struct {
	Names
	Doc []string
	// Image is the image without a tag, e.g. ghcr.io/you/shop: deploys
	// choose the version.
	Image string
	// Domains are the host names the app answers on, the first one its own.
	Domains []string
	// Port is the port the app listens on in its container.
	Port int
	// Worker runs the image's run command beside the app, from the same
	// version.
	Worker bool
	// Database gives the app a database of its own on the platform's
	// PostgreSQL.
	Database bool
	Pos      diag.Pos
}

// Server returns the server with the given name, or nil.
func (p *Project) Server(name string) *Server {
	for _, s := range p.Servers {
		if s.Raw == name {
			return s
		}
	}
	return nil
}

var (
	hostedSettings = []string{"image", "domains", "port", "worker", "database"}
	serverSettings = []string{"host"}
	appSettings    = []string{"module", "currency", "blueprint", "env"}

	domainName = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)
	imageName  = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(\.[a-z0-9]+)*(:[0-9]+)?(/[a-z0-9]+([._-]+[a-z0-9]+)*)+$`)
)

// buildPlatform builds a platform project: its platform block, servers and
// hosted apps. Declarations of app projects are refused.
func (b *builder) buildPlatform(platforms, servers, apps []*dsl.AppDecl) {
	b.p.Kind = KindPlatform
	for _, extra := range platforms[1:] {
		b.errorf(extra.Name.Span, "only one platform block is allowed; the first one is at %s", where(extra.Name.Pos, platforms[0].Pos))
	}
	d := platforms[0]
	for _, it := range d.Items {
		switch it := it.(type) {
		case *dsl.ModuleItem:
			b.errorf(it.Span, "a platform has no Go module").Suggestion = "remove the module line"
		case *dsl.EnvBlock:
			b.errorf(it.Span, "a platform has no env block").Suggestion = "the variables of an app go in its secrets: task secrets:edit APP=<app>"
		case *dsl.Setting:
			b.errorf(it.Key.Span, "unknown platform setting %q", it.Key.Name).
				Suggestion = "a platform block takes blueprints; the apps it runs are app blocks of their own"
		}
	}
	b.p.App = b.buildApp(d)

	for _, decl := range b.allDecls() {
		var what string
		var at dsl.Span
		switch decl := decl.(type) {
		case *dsl.PackageDecl:
			what, at = "package", decl.Name.Span
		case *dsl.EnumDecl:
			what, at = "enum", decl.Name.Span
		case *dsl.EntityDecl:
			what, at = "entity", decl.Name.Span
		case *dsl.JobDecl:
			what, at = "job", decl.Name.Span
		case *dsl.ReportDecl:
			what, at = "report", decl.Name.Span
		default:
			continue
		}
		b.errorf(at, "a platform has no %s: %ss belong to an app project", what, what).
			Suggestion = "a platform declares servers and the apps it runs"
	}

	seen := map[string]diag.Pos{}
	unique := func(id dsl.Ident) bool {
		if prev, ok := seen[id.Name]; ok {
			b.errorf(id.Span, "%s is already declared at %s", id.Name, where(id.Pos, prev))
			return false
		}
		seen[id.Name] = id.Pos
		return true
	}
	for _, s := range servers {
		if unique(s.Name) {
			b.p.Servers = append(b.p.Servers, b.buildServer(s))
		}
	}
	if len(servers) == 0 {
		b.errorf(d.Name.Span, "platform %s has no server", d.Name.Name).
			Suggestion = `add one, e.g. server main { host "203.0.113.10" }`
	}
	domains := map[string]diag.Pos{}
	for _, a := range apps {
		if !unique(a.Name) {
			continue
		}
		h := b.buildHosted(a)
		for i, dom := range h.Domains {
			if prev, ok := domains[dom]; ok {
				b.errorf(a.Name.Span, "the domain %s is already an app's, at %s", dom, where(a.Name.Pos, prev))
				h.Domains = append(h.Domains[:i:i], h.Domains[i+1:]...)
				break
			}
			domains[dom] = a.Name.Pos
		}
		b.p.Hosted = append(b.p.Hosted, h)
	}
}

// settingsOf indexes a block's settings by key, reporting repeated and
// unknown ones and every item a block of settings does not take.
func (b *builder) settingsOf(d *dsl.AppDecl, what string, known []string) map[string]*dsl.Setting {
	out := map[string]*dsl.Setting{}
	for _, it := range d.Items {
		switch it := it.(type) {
		case *dsl.Setting:
			if prev, ok := out[it.Key.Name]; ok {
				b.errorf(it.Key.Span, "%s is already set at line %d", it.Key.Name, prev.Pos.Line)
				continue
			}
			if !slices.Contains(known, it.Key.Name) {
				e := b.errorf(it.Key.Span, "%q is not a setting of %s", it.Key.Name, what)
				if s := didYouMean(it.Key.Name, known); s != "" {
					e.Suggestion = s
				} else {
					e.Suggestion = what + " takes " + strings.Join(known, ", ")
				}
				continue
			}
			out[it.Key.Name] = it
		case *dsl.ModuleItem, *dsl.BlueprintRef, *dsl.EnvBlock, *dsl.Annotation:
			b.errorf(it.SourceSpan(), "%s takes only %s", what, strings.Join(known, ", "))
		}
	}
	return out
}

// stringSetting is the string value of a setting, reporting other kinds.
func (b *builder) stringSetting(s *dsl.Setting) (string, bool) {
	if s.Value == nil || s.Value.Kind != dsl.StringValue {
		b.errorf(s.Key.Span, "%s takes a string", s.Key.Name).Suggestion = fmt.Sprintf(`e.g. %s "..."`, s.Key.Name)
		return "", false
	}
	return s.Value.Text, true
}

func (b *builder) buildServer(d *dsl.AppDecl) *Server {
	s := &Server{Names: b.names(d.Name), Doc: d.Doc, Pos: d.Pos}
	set := b.settingsOf(d, "a server", serverSettings)
	if h, ok := set["host"]; ok {
		if v, ok := b.stringSetting(h); ok {
			if v == "" || strings.ContainsAny(v, " /:@") {
				b.errorf(h.Value.Span, "%q is not a host: write an IP address or a host name", v)
			}
			s.Host = v
		}
	} else {
		b.errorf(d.Name.Span, "server %s has no host", d.Name.Name).Suggestion = `add its address, e.g. host "203.0.113.10"`
	}
	return s
}

func (b *builder) buildHosted(d *dsl.AppDecl) *Hosted {
	h := &Hosted{Names: b.names(d.Name), Doc: d.Doc, Port: 8080, Pos: d.Pos}
	set := b.settingsOf(d, "an app a platform runs", hostedSettings)
	if s, ok := set["image"]; ok {
		if v, ok := b.stringSetting(s); ok {
			switch {
			case strings.ContainsAny(v[strings.LastIndex(v, "/")+1:], ":@"):
				b.errorf(s.Value.Span, "the image %s has a tag: write it without one", v).
					Suggestion = "each deploy chooses the version"
			case !imageName.MatchString(v):
				b.errorf(s.Value.Span, "%q is not an image name", v).Suggestion = `e.g. image "ghcr.io/you/` + h.Kebab + `"`
			}
			h.Image = v
		}
	} else {
		b.errorf(d.Name.Span, "app %s has no image", d.Name.Name).
			Suggestion = `add the image its CI publishes, without a tag, e.g. image "ghcr.io/you/` + h.Kebab + `"`
	}
	if s, ok := set["domains"]; ok {
		if s.Value == nil || s.Value.Kind != dsl.ListValue || len(s.Value.List) == 0 {
			b.errorf(s.Key.Span, "domains takes a list of host names").Suggestion = `e.g. domains ["shop.com", "api.shop.com"]`
		} else {
			for _, v := range s.Value.List {
				if v.Kind != dsl.StringValue || !domainName.MatchString(v.Text) {
					b.errorf(v.Span, "%s is not a domain name", v.Raw).Suggestion = `write it lowercase, in quotes, e.g. "api.shop.com"`
					continue
				}
				if slices.Contains(h.Domains, v.Text) {
					b.errorf(v.Span, "%s is already in the list", v.Raw)
					continue
				}
				h.Domains = append(h.Domains, v.Text)
			}
		}
	}
	if s, ok := set["port"]; ok {
		n, err := strconv.Atoi(valueText(s.Value))
		if s.Value == nil || s.Value.Kind != dsl.NumberValue || err != nil || n < 1 || n > 65535 {
			b.errorf(s.Key.Span, "port takes a number from 1 to 65535")
		} else {
			h.Port = n
		}
	}
	for _, key := range []string{"worker", "database"} {
		s, ok := set[key]
		if !ok {
			continue
		}
		if s.Value != nil {
			b.errorf(s.Value.Span, "%s takes no value: write it alone, or leave it out", key)
			continue
		}
		if key == "worker" {
			h.Worker = true
		} else {
			h.Database = true
		}
	}
	return h
}

func valueText(v *dsl.Value) string {
	if v == nil {
		return ""
	}
	return v.Text
}
