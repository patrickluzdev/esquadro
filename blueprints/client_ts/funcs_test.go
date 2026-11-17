package clientts

import (
	"testing"

	"github.com/patrickluzdev/esquadro/internal/project"
)

func TestPackageName(t *testing.T) {
	app := &project.App{Module: "github.com/Acme/shop"}
	app.Kebab = "shop"
	for _, tc := range []struct {
		module string
		opts   map[string]any
		want   string
	}{
		{"github.com/Acme/shop", nil, "@acme/shop-client"},
		{"example.com/shop", nil, ""},
		{"example.com/shop", map[string]any{"scope": "you"}, "@you/shop-client"},
		{"example.com/shop", map[string]any{"package": "shop-api"}, "shop-api"},
	} {
		app.Module = tc.module
		if got := PackageName(app, tc.opts); got != tc.want {
			t.Errorf("%s %v: %q, want %q", tc.module, tc.opts, got, tc.want)
		}
	}
}
