package security

import (
	"slices"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/compose"
)

func frontendsOnDomains() []*compose.Instance {
	return []*compose.Instance{
		{Name: "admin", Kind: "spa", Options: map[string]any{"domain": "painel.shop.com"}},
		{Name: "staff", Kind: "spa", Options: map[string]any{"path": "/equipe/"}},
		{Name: "site", Kind: "ssr", Options: map[string]any{"domain": "shop.com", "locale": "pt_br"}},
		{Name: "blog", Kind: "ssr", Options: map[string]any{}},
	}
}

func TestResetPageOnADomain(t *testing.T) {
	for _, f := range frontendsOnDomains() {
		want := map[string]string{
			"admin": "https://painel.shop.com/reset-password",
			"staff": "/equipe/reset-password",
			"site":  "https://shop.com/nova-senha",
			"blog":  "/blog/new-password",
		}[f.Name]
		if got := ResetPage(f.Kind, f.Name, f.Options); got != want {
			t.Errorf("%s: %s, want %s", f.Name, got, want)
		}
	}
}

func TestDevServersOfDomains(t *testing.T) {
	want := []DevServerOf{
		{At: "/equipe", URL: DevServer},
		{At: "painel.shop.com", URL: DevServer},
		{At: "shop.com", Host: "site.localhost"},
	}
	if got := DevServers(frontendsOnDomains()); !slices.Equal(got, want) {
		t.Errorf("%+v, want %+v", got, want)
	}
}

func TestRedirectURLsHoldTheDomains(t *testing.T) {
	got := RedirectURLs([]string{"https://shop.com", "https://partner.example/back"}, frontendsOnDomains())
	want := []string{"https://shop.com", "https://partner.example/back", "https://painel.shop.com"}
	if !slices.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
	if got := RedirectURLs(nil, frontendsOnDomains()[1:2]); len(got) != 0 {
		t.Errorf("without domains: %v", got)
	}
}
