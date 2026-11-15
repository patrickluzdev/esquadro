package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPostgresGeneratesAWorkingService generates a project with every
// built-in blueprint of a JSON API on Postgres, checks that it builds and passes go vet, applies
// its migrations with its own migrate command against a real Postgres, and
// runs the tests esquadro generated for it. It needs Docker (for Postgres, and for sqlc when it is
// not installed) and the network (Go modules); it is skipped with -short or
// without Docker.
// every lists the blueprints of a JSON API on Postgres, as esquadro new does.
var every = func() string {
	var b strings.Builder
	for bp := range strings.SplitSeq(defaultBlueprints, ",") {
		b.WriteString("  blueprint ")
		b.WriteString(bp)
		b.WriteString("\n")
	}
	return b.String()
}()

func TestPostgresGeneratesAWorkingService(t *testing.T) {
	if testing.Short() {
		t.Skip("generates and runs a service against Postgres")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker is not available")
	}
	// Docker Desktop only shares some folders with containers (sqlc runs in
	// one); the user's cache folder is among them, the system's temp folder
	// may not be.
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	dir, err := os.MkdirTemp(filepath.Join(cache), "esquadro-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	write := func(name, content string) {
		t.Helper()
		full := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("esquadro/app.esq", "app shop {\n  module \"example.com/shop\"\n"+every+"}\n")
	write("esquadro/catalog.esq", `package catalog

entity Category {
  name     string required unique
  products < Product
}

entity Product {
  name     string required min(2)
  stock    int    default(0)
  category > Category required
  tags     <> Tag
}

entity Tag {
  label string required
}
`)
	write("esquadro/orders.esq", `package orders

entity Order {
  number   string required
  products <> Product
}

entity OrderItem {
  quantity   int  required
  product_id uuid required
  order      > Order required
}
`)
	if code, out, errOut := gen(t, dir); code != 0 {
		t.Fatalf("gen: exit %d\n%s%s", code, out, errOut)
	}
	run := func(env []string, name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	run(nil, "go", "build", "./...")
	run(nil, "go", "vet", "./...")
	// compose.yaml is YAML gofmt never sees: Docker Compose checks it.
	run(nil, "docker", "compose", "config", "-q")

	out, err := exec.Command("docker", "run", "-d", "--rm", "-e", "POSTGRES_PASSWORD=postgres", "-e", "POSTGRES_DB=shop",
		"-p", "127.0.0.1::5432", "postgres:17-alpine").Output()
	if err != nil {
		t.Fatalf("docker run: %v", err)
	}
	container := strings.TrimSpace(string(out))
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", container).Run() })
	port, err := exec.Command("docker", "port", container, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	_, hostPort, _ := strings.Cut(strings.TrimSpace(strings.Split(string(port), "\n")[0]), ":")
	url := "DATABASE_URL=postgres://postgres:postgres@127.0.0.1:" + hostPort + "/shop?sslmode=disable"
	deadline := time.Now().Add(60 * time.Second)
	for exec.Command("docker", "exec", container, "psql", "-U", "postgres", "-d", "shop", "-c", "select 1").Run() != nil {
		if time.Now().After(deadline) {
			t.Fatal("Postgres did not start")
		}
		time.Sleep(300 * time.Millisecond)
	}
	run([]string{url}, "go", "run", "./cmd/shop", "migrate")

	// The tests esquadro generates run every use case against their own
	// Postgres (testcontainers).
	tests := run(nil, "go", "test", "-v", "./...")
	for _, want := range []string{"--- PASS: TestCreateAndReadProduct", "--- PASS: TestAPIProducts", "--- PASS: TestAPIEdge", "--- PASS: TestCompressAnswersGzipToWhoAcceptsIt",
		"--- PASS: TestProductTags", "--- PASS: TestOrderProducts", "--- PASS: TestAPIProductTags", "--- PASS: TestAPIOrderProducts"} {
		if !strings.Contains(tests, want) {
			t.Fatalf("the generated tests did not run %s:\n%s", want, tests)
		}
	}
}
