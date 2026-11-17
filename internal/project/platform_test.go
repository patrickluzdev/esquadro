package project

import (
	"strings"
	"testing"
)

const platformSrc = `/// The servers of the projects.
platform acme {
  blueprint server
}

/// The VPS.
server main {
  host "203.0.113.10"
}

/// The shop.
app shop {
  image   "ghcr.io/acme/shop"
  domains ["shop.com", "api.shop.com"]
  worker
  database
}

app blog {
  image "ghcr.io/acme/blog"
  port  3000
}
`

func TestPlatform(t *testing.T) {
	p, diags := Load(Source{Name: "platform.esq", Src: []byte(platformSrc)})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if p.Kind != KindPlatform || p.App.Raw != "acme" || p.App.Module != "" || len(p.App.Blueprints) != 1 {
		t.Fatalf("platform: %+v", p.App)
	}
	if len(p.Servers) != 1 || p.Server("main").Host != "203.0.113.10" || p.Servers[0].Doc[0] != "The VPS." {
		t.Fatalf("servers: %+v", p.Servers)
	}
	if len(p.Hosted) != 2 {
		t.Fatalf("hosted: %+v", p.Hosted)
	}
	shop, blog := p.Hosted[0], p.Hosted[1]
	if shop.Image != "ghcr.io/acme/shop" || strings.Join(shop.Domains, ",") != "shop.com,api.shop.com" || !shop.Worker || !shop.Database || shop.Port != 8080 {
		t.Fatalf("shop: %+v", shop)
	}
	if blog.Port != 3000 || blog.Worker || blog.Database || len(blog.Domains) != 0 {
		t.Fatalf("blog: %+v", blog)
	}
}

func TestAnAppIsAnAppProject(t *testing.T) {
	p, diags := Load(Source{Name: "app.esq", Src: []byte("app shop {\n  module \"example.com/shop\"\n}\n")})
	if diags.HasErrors() || p.Kind != KindApp {
		t.Fatalf("%v, kind %q", diags, p.Kind)
	}
}

func TestPlatformErrors(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"no server":       {"platform acme {\n  blueprint server\n}\n", "has no server"},
		"two platforms":   {platformSrc + "platform other {\n}\n", "only one platform block"},
		"module":          {strings.Replace(platformSrc, "  blueprint server\n", "  blueprint server\n  module \"x.com/y\"\n", 1), "a platform has no Go module"},
		"entity":          {"package shop\n\n" + platformSrc + "\nentity Product {\n  name string\n}\n", "a platform has no entity"},
		"no host":         {strings.Replace(platformSrc, `  host "203.0.113.10"`, "", 1), "server main has no host"},
		"bad host":        {strings.Replace(platformSrc, `"203.0.113.10"`, `"root@203.0.113.10"`, 1), "is not a host"},
		"no image":        {strings.Replace(platformSrc, `  image "ghcr.io/acme/blog"`, "", 1), "app blog has no image"},
		"tagged image":    {strings.Replace(platformSrc, `"ghcr.io/acme/blog"`, `"ghcr.io/acme/blog:v1"`, 1), "has a tag"},
		"bad domain":      {strings.Replace(platformSrc, `"shop.com"`, `"Shop.com"`, 1), "is not a domain name"},
		"shared domain":   {strings.Replace(platformSrc, `  port  3000`, "  domains [\"shop.com\"]", 1), "already an app's"},
		"repeated domain": {strings.Replace(platformSrc, `"api.shop.com"`, `"shop.com"`, 1), "already in the list"},
		"flag value":      {strings.Replace(platformSrc, "  worker\n", "  worker true\n", 1), "worker takes no value"},
		"bad port":        {strings.Replace(platformSrc, "3000", "70000", 1), "port takes a number"},
		"unknown setting": {strings.Replace(platformSrc, "  worker\n", "  workers\n", 1), `"workers" is not a setting of an app a platform runs`},
		"hosted module":   {strings.Replace(platformSrc, "  worker\n", "  module \"x.com/y\"\n", 1), "takes only image, domains"},
		"same name":       {platformSrc + "server shop {\n  host \"10.0.0.1\"\n}\n", "already declared"},
		"server in app":   {"app shop {\n  module \"example.com/shop\"\n}\n\nserver main {\n  host \"1.2.3.4\"\n}\n", "belongs to a platform project"},
		"app setting":     {"app shop {\n  modul \"example.com/shop\"\n}\n", `unknown app setting "modul"`},
	} {
		_, diags := Load(Source{Name: "p.esq", Src: []byte(tc.src)})
		if !diags.HasErrors() || !strings.Contains(diags.Err().Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, diags, tc.want)
		}
	}
}
