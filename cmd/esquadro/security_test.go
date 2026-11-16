package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// securityProject is a project with the security blueprint and the given
// accounts module and postgres block.
func securityProject(t *testing.T, accounts, postgres string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-no-gen", "-no-git", "-module", "example.com/shop", "-blueprints", "app,env,domain,http,postgres,migrations,security", dir); code != 0 {
		t.Fatal(errOut)
	}
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	src := read(t, appEsq)
	if postgres != "" {
		src = strings.Replace(src, "  blueprint postgres\n", postgres, 1)
	}
	os.WriteFile(appEsq, []byte(src), 0o644)
	os.WriteFile(filepath.Join(dir, "esquadro", "accounts.esq"), []byte(accounts), 0o644)
	return dir
}

func TestSecurityChecksTheModel(t *testing.T) {
	dir := securityProject(t, `package accounts

entity User {
  name string required
  @security(principal, roles [admin, staff], permissions [orders.refund, refund], grants [admin.orders.refund, boss.orders.refund, staff.reports.view], providers [password, facebook], verify_email maybe, signup sometimes, password_min 6)
}

entity Note {
  text string
  @security(read public, write [user.admin, user.boss, team])
}

entity Tag {
  label string
}
`, "")
	code, _, errOut := esq(t, "check", dir)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	for _, want := range []string{
		`signup "sometimes" is not one of open, invite, closed`,
		`providers "facebook" is not one of password, google, github`,
		`verify_email "maybe" is not one of required, optional, off`,
		`permission "refund" should name what it is about`,
		`grants gives permissions to boss, which is not a role of user`,
		`grants gives staff.reports.view, but reports.view is not one of the realm's permissions`,
		`password_min 6 is below 8`,
		`write: boss is not a role of user, whose roles are admin, staff`,
		`write: team is not public, nor a realm`,
		`Tag has no @security(read, write...)`,
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("missing %q in:\n%s", want, errOut)
		}
	}
}

func TestSecurityTables(t *testing.T) {
	accounts := `package accounts

entity User {
  name string required
  @security(principal, roles [admin], read user, write user.admin)
}
`
	dir := securityProject(t, accounts, "")
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	sql := read(t, filepath.Join(dir, "internal", "platform", "security", "migrations", "00001_security.sql"))
	if !strings.Contains(sql, "CREATE TABLE security_identities (") || strings.Contains(sql, "CREATE SCHEMA") {
		t.Fatalf("public, prefixed:\n%s", sql)
	}
	if !strings.Contains(read(t, filepath.Join(dir, "migrations", "migrations.go")), "securitymigrations.Apply(ctx, db)") {
		t.Fatal("the model's migrations do not apply the security ones")
	}

	dir = securityProject(t, accounts, "  blueprint postgres {\n    schemas [\"security\"]\n  }\n")
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	sql = read(t, filepath.Join(dir, "internal", "platform", "security", "migrations", "00001_security.sql"))
	if !strings.Contains(sql, "CREATE SCHEMA IF NOT EXISTS security;") || !strings.Contains(sql, "CREATE TABLE security.identities (") {
		t.Fatalf("own schema:\n%s", sql)
	}
}

// A provider's variables exist only when a realm signs in with it: env
// declarations with when.
func TestSecurityProviderVariables(t *testing.T) {
	model := func(providers string) string {
		return "package accounts\n\nentity User {\n  name string required\n  @security(principal, providers " + providers + ", read user, write user)\n}\n"
	}
	dir := securityProject(t, model("password"), "")
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	if example := read(t, filepath.Join(dir, ".env.example")); strings.Contains(example, "SECURITY_GOOGLE") || strings.Contains(example, "SECURITY_PEPPER") {
		t.Fatalf("variables nobody uses:\n%s", example)
	}

	os.WriteFile(filepath.Join(dir, "esquadro", "accounts.esq"), []byte(model("[password, google]")), 0o644)
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	example := read(t, filepath.Join(dir, ".env.example"))
	if !strings.Contains(example, "SECURITY_GOOGLE_CLIENT_ID") || !strings.Contains(example, "SECURITY_GOOGLE_CLIENT_SECRET") || strings.Contains(example, "SECURITY_GITHUB") {
		t.Fatalf("google's variables:\n%s", example)
	}
	if deps := read(t, filepath.Join(dir, "cmd", "shop", "dependencies.go")); !strings.Contains(deps, `"google": security.Google(d.config.SecurityGoogleClientID, d.config.SecurityGoogleClientSecret)`) {
		t.Fatalf("dependencies:\n%s", deps)
	}
}

// Expired sessions and links are cleaned up by jobs, else by the worker,
// else the check says nothing does.
func TestSecurityMaintenance(t *testing.T) {
	accounts := "package accounts\n\nentity User {\n  name string required\n  @security(principal, read user, write user)\n}\n"
	for _, tc := range []struct{ blueprints, file, want string }{
		{"  blueprint worker\n  blueprint jobs\n", "cmd/shop/dependencies.go", "d.security.RegisterJobs(registry)"},
		{"  blueprint worker\n", "cmd/shop/worker.go", `{Name: "security", Run: d.security.Maintain},`},
	} {
		dir := securityProject(t, accounts, "  blueprint postgres\n"+tc.blueprints)
		if code, out, errOut := esq(t, "gen", dir); code != 0 || strings.Contains(errOut, "nothing cleans up") {
			t.Fatalf("gen: %s%s", out, errOut)
		}
		if got := read(t, filepath.Join(dir, tc.file)); !strings.Contains(got, tc.want) {
			t.Errorf("%s has no %s:\n%s", tc.file, tc.want, got)
		}
	}
	dir := securityProject(t, accounts, "")
	if _, _, errOut := esq(t, "check", dir); !strings.Contains(errOut, "nothing cleans up expired sessions") {
		t.Fatalf("no warning:\n%s", errOut)
	}
}

// cors and security_headers became part of security: a project that still
// names them is told what to write instead.
func TestRetiredBlueprints(t *testing.T) {
	for name, option := range map[string]string{"cors": "origins", "security_headers": "hsts"} {
		dir, appEsq := smallProject(t, "app,env,http")
		src := strings.Replace(read(t, appEsq), "  blueprint http\n", "  blueprint http\n  blueprint "+name+"\n", 1)
		os.WriteFile(appEsq, []byte(src), 0o644)
		code, _, errOut := esq(t, "check", dir)
		if code != 1 || !strings.Contains(errOut, "blueprint "+name+" is no more") || !strings.Contains(errOut, "with its "+option+" option") {
			t.Errorf("%s: exit %d\n%s", name, code, errOut)
		}
	}
}

// Without a principal, security only answers CORS and sends the headers:
// no database needed.
func TestSecurityWithoutPrincipals(t *testing.T) {
	dir, _ := smallProject(t, "app,env,http,security")
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	routes := read(t, filepath.Join(dir, "cmd", "shop", "routes.go"))
	if !strings.Contains(routes, "edge.CORS(d.config.CORSOrigins)") || !strings.Contains(routes, "edge.Secure(d.config.HSTS)") || strings.Contains(routes, "Authenticate") {
		t.Fatalf("routes.go:\n%s", routes)
	}
	if _, err := os.Stat(filepath.Join(dir, "internal", "platform", "security")); err == nil {
		t.Fatal("a security package without principals")
	}
	if strings.Contains(read(t, filepath.Join(dir, ".env.example")), "SECURITY_SECRET") {
		t.Fatal("SECURITY_SECRET without principals")
	}
}

// password_lists goes in the Dockerfile: only plain paths of the project.
func TestSecurityPasswordListPaths(t *testing.T) {
	accounts := "package accounts\n\nentity User {\n  name string required\n  @security(principal, read user, write user)\n}\n"
	for path, ok := range map[string]bool{
		"config/passwords-br.txt": true, "../secret.txt": false, "/etc/passwd": false,
		"a b.txt": false, "x.txt\nRUN curl evil": false, "./config/x.txt": false,
	} {
		dir := securityProject(t, accounts, "")
		appEsq := filepath.Join(dir, "esquadro", "app.esq")
		quoted, _ := json.Marshal(path)
		src := strings.Replace(read(t, appEsq), "  blueprint security\n", "  blueprint security {\n    password_lists ["+string(quoted)+"]\n  }\n", 1)
		os.WriteFile(appEsq, []byte(src), 0o644)
		_, _, errOut := esq(t, "check", dir)
		if refused := strings.Contains(errOut, "write a path of the project"); refused == ok {
			t.Errorf("%q: refused %v\n%s", path, refused, errOut)
		}
	}
}
