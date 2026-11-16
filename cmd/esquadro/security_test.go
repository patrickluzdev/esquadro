package main

import (
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
