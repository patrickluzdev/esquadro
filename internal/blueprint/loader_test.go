package blueprint

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// catalogs has a postgres for apps, one for platforms, a blueprint for
// both, and one for platforms only.
var catalogs = fstest.MapFS{
	"postgres/blueprint.esq":          {Data: []byte("blueprint postgres {\n  version \"1.0.0\"\n}\n")},
	"platform/postgres/blueprint.esq": {Data: []byte("blueprint postgres {\n  version \"2.0.0\"\n  for platform\n}\n")},
	"git/blueprint.esq":               {Data: []byte("blueprint git {\n  version \"1.0.0\"\n  for app, platform\n}\n")},
	"platform/caddy/blueprint.esq":    {Data: []byte("blueprint caddy {\n  version \"1.0.0\"\n  for platform\n}\n")},
}

func TestEachKindOfProjectHasItsCatalog(t *testing.T) {
	load := func(kind project.Kind, name string) (*Blueprint, string) {
		l := &Loader{Builtin: catalogs, Kind: kind}
		b, diags := l.Load(&project.BlueprintRef{Ref: name, Source: project.SourceBuiltin})
		return b, fmt.Sprint(diags)
	}
	if b, _ := load(project.KindApp, "postgres"); b == nil || b.Manifest.Version != "1.0.0" || b.Key != "postgres" {
		t.Fatalf("an app's postgres: %+v", b)
	}
	if b, _ := load(project.KindPlatform, "postgres"); b == nil || b.Manifest.Version != "2.0.0" || b.Key != "platform/postgres" {
		t.Fatalf("a platform's postgres: %+v", b)
	}
	if b, _ := load(project.KindPlatform, "git"); b == nil || b.Key != "git" {
		t.Fatalf("git on a platform: %+v", b)
	}
	if b, err := load(project.KindApp, "caddy"); b != nil || !strings.Contains(err, `unknown built-in blueprint "caddy"`) {
		t.Fatalf("caddy on an app: %v", err)
	}
	if b, err := load(project.KindApp, "platform"); b != nil || !strings.Contains(err, "unknown built-in blueprint") {
		t.Fatalf("the platform folder: %v", err)
	}
	if got := strings.Join((&Loader{Builtin: catalogs, Kind: project.KindPlatform}).BuiltinNames(), ","); got != "caddy,git,postgres" {
		t.Fatalf("a platform's names: %s", got)
	}
	if got := strings.Join((&Loader{Builtin: catalogs}).BuiltinNames(), ","); got != "git,postgres" {
		t.Fatalf("an app's names: %s", got)
	}
}

func TestABlueprintForTheOtherKindIsRefused(t *testing.T) {
	appOnly := fstest.MapFS{"x/blueprint.esq": {Data: []byte("blueprint x {\n  version \"1.0.0\"\n}\n")}}
	l := &Loader{Builtin: appOnly, Kind: project.KindPlatform}
	_, diags := l.Load(&project.BlueprintRef{Ref: "x", Source: project.SourceBuiltin})
	if !strings.Contains(fmt.Sprint(diags), "blueprint x is made for app projects, not for a platform") {
		t.Fatalf("%v", diags)
	}
}

func TestForTakesKindsOfProject(t *testing.T) {
	for src, want := range map[string]string{
		"for server":              "server is not a kind of project",
		"for app, app":            "app is already listed",
		"for app\n  for platform": "for is already",
	} {
		_, diags := ParseManifest("m.esq", []byte("blueprint x {\n  version \"1.0.0\"\n  "+src+"\n}\n"))
		if !strings.Contains(fmt.Sprint(diags), want) {
			t.Errorf("%q: %v, want %q", src, diags, want)
		}
	}
}
