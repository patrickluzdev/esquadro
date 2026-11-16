package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/hook"
	"github.com/patrickluzdev/esquadro/internal/project"
)

func vars() []project.EnvDescription {
	return []project.EnvDescription{
		{Name: "APP_NAME", Type: "string", Doc: []string{"The app's name."}, Values: map[string]string{"development": "shop", "staging": "shop", "production": "shop"}},
		{Name: "DATABASE_URL", Type: "string", Secret: true, Required: []string{"staging", "production"}, Values: map[string]string{"development": "postgres://localhost"}},
		{Name: "SESSION_KEY", Type: "string", Secret: true, Generate: 32},
	}
}

func run(t *testing.T, dir string, v []project.EnvDescription) hook.Output {
	t.Helper()
	out, err := Files(t.Context(), hook.Input{ProjectDir: dir, Env: v, Environments: []string{"development", "staging", "production"}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFilesCreatesOnePerEnvironment(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/bin/"), 0o644)
	run(t, dir, vars())

	dev := read(t, filepath.Join(dir, ".env.development"))
	if !strings.Contains(dev, "# The app's name.\n# APP_NAME=shop\n") || !strings.Contains(dev, "# DATABASE_URL=\n") {
		t.Fatalf(".env.development:\n%s", dev)
	}
	_, after, _ := strings.Cut(dev, "SESSION_KEY=")
	if key, _, _ := strings.Cut(after, "\n"); len(key) < 40 {
		t.Fatalf("the generated secret: %q", key)
	}
	prod := read(t, filepath.Join(dir, ".env.production"))
	if !strings.Contains(prod, "\nDATABASE_URL=\n") || !strings.Contains(prod, "# SESSION_KEY=\n") {
		t.Fatalf(".env.production:\n%s", prod)
	}
	if info, _ := os.Stat(filepath.Join(dir, ".env.production")); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v: secrets are for the user only", info.Mode().Perm())
	}
	ignore := read(t, filepath.Join(dir, ".gitignore"))
	if !strings.HasPrefix(ignore, "/bin/\n\n#") || !strings.Contains(ignore, ".env.development\n.env.staging\n.env.production\n") {
		t.Fatalf(".gitignore:\n%s", ignore)
	}

	// Again: nothing changes.
	run(t, dir, vars())
	if read(t, filepath.Join(dir, ".env.development")) != dev || read(t, filepath.Join(dir, ".gitignore")) != ignore {
		t.Fatal("a second run changed the files")
	}
}

func TestFilesKeepsTheUsersValues(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, vars())
	path := filepath.Join(dir, ".env.production")
	mine := strings.Replace(read(t, path), "\nDATABASE_URL=\n", "\nDATABASE_URL=postgres://prod\n", 1) + "OLD=1\n"
	os.WriteFile(path, []byte(mine), 0o600)

	more := append(vars()[1:], project.EnvDescription{Name: "STRIPE_KEY", Type: "string", Secret: true, Required: []string{"production"}})
	out := run(t, dir, more)
	got := read(t, path)
	for _, want := range []string{"DATABASE_URL=postgres://prod\n", notDeclared + "\nOLD=1\n", "\nSTRIPE_KEY=\n", "# APP_NAME=shop\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if len(out.Messages) == 0 || !strings.Contains(out.Messages[len(out.Messages)-1].Text, "added STRIPE_KEY to .env.production") {
		t.Fatalf("messages: %+v", out.Messages)
	}
	run(t, dir, more)
	if read(t, path) != got {
		t.Fatal("a second run changed the file")
	}
}
