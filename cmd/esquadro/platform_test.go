package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// platformProject generates a platform from the starter, with the given
// blueprints added and an app it runs.
func platformProject(t *testing.T, blueprints ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "acme")
	if code, _, errOut := esq(t, "new", "-starter", "platform", "-no-gen", "-no-git", dir); code != 0 {
		t.Fatal(errOut)
	}
	model := filepath.Join(dir, "esquadro", "platform.esq")
	src, err := os.ReadFile(model)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, b := range blueprints {
		if strings.Contains(s, "  blueprint "+strings.Fields(b)[0]+"\n") {
			continue
		}
		s = strings.Replace(s, "  blueprint server\n", "  blueprint server\n  blueprint "+b+"\n", 1)
	}
	s += "\n/// The shop.\napp shop {\n  image   \"ghcr.io/acme/shop\"\n  domains [\"shop.com\", \"api.shop.com\"]\n  worker\n  database\n}\n"
	if err := os.WriteFile(model, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	return dir
}

func TestAPlatformDescribesItsServersAndApps(t *testing.T) {
	dir := platformProject(t, "backup", "cloudflare {\n    account_id \"0123456789abcdef\"\n  }")
	vars, err := os.ReadFile(filepath.Join(dir, "ansible", "group_vars", "all", "platform.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"platform_admin_user: admin", "caddy_certificates: acme", "backup_endpoint: \"https://0123456789abcdef.r2.cloudflarestorage.com\"", "platform_ports:\n  - 80\n  - 443", "  - name: shop\n    snake: shop\n    image: ghcr.io/acme/shop\n    domains: [shop.com, api.shop.com]\n    port: 8080\n    worker: true\n    database: true"} {
		if !strings.Contains(string(vars), want) {
			t.Errorf("platform.yml lacks %q:\n%s", want, vars)
		}
	}
	inv, _ := os.ReadFile(filepath.Join(dir, "ansible", "inventory.yml"))
	if !strings.Contains(string(inv), "        main:\n          ansible_host: 203.0.113.10") {
		t.Errorf("inventory.yml:\n%s", inv)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		t.Error("a platform has a go.mod")
	}
	lintPlatform(t, dir)
}

// lintPlatform checks a generated platform's playbooks as its task lint
// does, when Ansible and ansible-lint are installed.
func lintPlatform(t *testing.T, dir string) {
	t.Helper()
	for _, tool := range []string{"ansible-playbook", "ansible-lint", "ansible-galaxy"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Logf("%s is not installed: the playbooks are not linted", tool)
			return
		}
	}
	env := append(os.Environ(), "ANSIBLE_CONFIG=ansible/ansible.cfg", "ANSIBLE_COLLECTIONS_PATH="+collectionsPath(t))
	if _, err := exec.LookPath("task"); err == nil {
		cmd := exec.Command("task", "--list")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("task --list: %v\n%s", err, out)
		}
	}
	if _, err := exec.LookPath("tofu"); err == nil {
		if _, err := os.Stat(filepath.Join(dir, "tofu")); err == nil {
			cmd := exec.Command("tofu", "fmt", "-check", "-recursive", "tofu")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("tofu fmt: %v\n%s", err, out)
			}
		}
	}
	for _, args := range [][]string{
		{"ansible-playbook", "ansible/site.yml", "--syntax-check"},
		{"ansible-lint", "--offline", "-q", "ansible/"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// collectionsPath is where the installed Ansible keeps community.docker, so
// ansible-lint finds the collections whatever Python it runs on.
func collectionsPath(t *testing.T) string {
	out, err := exec.Command("ansible-galaxy", "collection", "list", "community.docker").Output()
	if err != nil {
		t.Skip("the community.docker collection is not installed: run task deps in a platform")
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		if p, ok := strings.CutPrefix(sc.Text(), "# "); ok && strings.HasSuffix(p, "ansible_collections") {
			return filepath.Dir(p)
		}
	}
	t.Skip("cannot tell where the Ansible collections are")
	return ""
}

func TestAnAppWithADatabaseNeedsThePlatformsPostgres(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "acme")
	if code, _, errOut := esq(t, "new", "-starter", "platform", "-no-gen", "-no-git", dir); code != 0 {
		t.Fatal(errOut)
	}
	model := filepath.Join(dir, "esquadro", "platform.esq")
	src, _ := os.ReadFile(model)
	s := strings.Replace(string(src), "  blueprint postgres\n", "", 1) + "\napp shop {\n  image \"ghcr.io/acme/shop\"\n  database\n}\n"
	if err := os.WriteFile(model, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := esq(t, "check", dir); code == 0 || !strings.Contains(errOut, "app shop has database, and the platform has no PostgreSQL") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

func TestDeployAddsTheDeployToTheCI(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-blueprints", "app,env,http,docker,github_actions,deploy", "-module", "example.com/shop", "-no-git", dir); code != 0 {
		t.Fatal(errOut)
	}
	ci, _ := os.ReadFile(filepath.Join(dir, ".github", "workflows", "ci.yml"))
	if !strings.Contains(string(ci), `printf '%s' "$REGISTRY_TOKEN" | ssh -i ~/.ssh/deploy "deploy@$DEPLOY_HOST" "$GITHUB_REF_NAME"`) {
		t.Fatalf("ci.yml:\n%s", ci)
	}
	dockerfile, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if !strings.Contains(string(dockerfile), "ENV SHOP_TRUST_PROXY=true") {
		t.Fatalf("Dockerfile:\n%s", dockerfile)
	}
}
