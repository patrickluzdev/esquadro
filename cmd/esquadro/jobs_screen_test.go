package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jobsScreenProject is a project with jobs and the given jobs block, and,
// with accounts, the security blueprint and that accounts module.
func jobsScreenProject(t *testing.T, jobs, accounts string) string {
	t.Helper()
	blueprints := "app,env,domain,http,postgres,store,migrations,worker,jobs"
	if accounts != "" {
		blueprints += ",security,mail"
	}
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-no-gen", "-no-git", "-module", "example.com/shop", "-blueprints", blueprints, dir); code != 0 {
		t.Fatal(errOut)
	}
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	src := strings.Replace(read(t, appEsq), "  blueprint jobs\n", jobs, 1)
	if err := os.WriteFile(appEsq, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if accounts != "" {
		if err := os.WriteFile(filepath.Join(dir, "esquadro", "accounts.esq"), []byte(accounts), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const jobsScreenAccounts = `package accounts

entity User {
  name string required
  @security(principal, roles [admin, staff], read user, write user.admin)
}
`

// The jobs screen is served behind sign-in, to the roles ui_access names
// (or the first realm's admins), and only with a path of its own.
func TestJobsScreen(t *testing.T) {
	dir := jobsScreenProject(t, "  blueprint jobs {\n    ui \"/jobs\"\n    ui_access [user.admin]\n  }\n", jobsScreenAccounts)
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	routes := read(t, filepath.Join(dir, "cmd", "shop", "routes_gen.go"))
	for _, want := range []string{
		`d.jobs.UI("/jobs", d.logger)`,
		`d.security.EmbeddedUI("/jobs", []string{"user.admin"}, d.realms(), jobsScreen)`,
		`mux.Handle("/jobs/", jobsScreen)`,
	} {
		if !strings.Contains(routes, want) {
			t.Errorf("routes_gen.go lacks %s:\n%s", want, routes)
		}
	}
	if ui := read(t, filepath.Join(dir, "internal", "platform", "jobs", "ui.go")); !strings.Contains(ui, "JobListHideArgsByDefault: true") {
		t.Errorf("the screen shows the jobs' arguments:\n%s", ui)
	}

	dir = jobsScreenProject(t, "  blueprint jobs\n", jobsScreenAccounts)
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen without ui: %s%s", out, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "internal", "platform", "jobs", "ui.go")); !os.IsNotExist(err) {
		t.Errorf("ui.go without ui: %v", err)
	}
	if strings.Contains(read(t, filepath.Join(dir, "cmd", "shop", "routes_gen.go")), "jobsScreen") {
		t.Error("routes_gen.go serves the screen without ui")
	}
}

func TestJobsScreenChecks(t *testing.T) {
	for _, c := range []struct {
		name, jobs, accounts, want string
	}{
		{"without sign-in", "  blueprint jobs {\n    ui \"/jobs\"\n  }\n", "", "needs sign-in to guard it"},
		{"not a path", "  blueprint jobs {\n    ui \"jobs\"\n  }\n", jobsScreenAccounts, `"jobs" is not a path`},
		{"the API's own", "  blueprint jobs {\n    ui \"/healthz\"\n  }\n", jobsScreenAccounts, "is the API's own"},
		{"under the identity routes", "  blueprint jobs {\n    ui \"/identity/jobs\"\n  }\n", jobsScreenAccounts, "under the identity routes"},
		{"no such realm", "  blueprint jobs {\n    ui \"/jobs\"\n    ui_access [team.admin]\n  }\n", jobsScreenAccounts, "team.admin is no realm"},
		{"no such role", "  blueprint jobs {\n    ui \"/jobs\"\n    ui_access [user.boss]\n  }\n", jobsScreenAccounts, "user has no role boss"},
		{"access without a screen", "  blueprint jobs {\n    ui_access [user.admin]\n  }\n", jobsScreenAccounts, "set ui too"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := jobsScreenProject(t, c.jobs, c.accounts)
			code, out, errOut := esq(t, "check", dir)
			if code == 0 || !strings.Contains(out+errOut, c.want) {
				t.Fatalf("exit %d, want %q:\n%s%s", code, c.want, out, errOut)
			}
		})
	}
}
