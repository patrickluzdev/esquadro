package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPostgresGeneratesAWorkingService generates a project with the platform
// and postgres blueprints, checks that it builds and passes go vet, applies
// its migrations with its own migrate command against a real Postgres, and
// runs its use cases. It needs Docker (for Postgres, and for sqlc when it is
// not installed) and the network (Go modules); it is skipped with -short or
// without Docker.
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
	write("app.esq", "app shop {\n  module \"example.com/shop\"\n  blueprint platform\n  blueprint postgres\n}\n")
	write("catalog.esq", `package catalog

entity Category {
  name     string required unique
  products < Product
}

entity Product {
  name     string required min(2)
  stock    int    default(0)
  category > Category required
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

	write("cmd/try/main.go", `package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"example.com/shop/internal/catalog"
	"example.com/shop/internal/platform/postgres"
)

func main() {
	if err := try(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func try() error {
	ctx := context.Background()
	pool, err := postgres.Open(ctx, os.Getenv("DATABASE_URL"), 4)
	if err != nil {
		return err
	}
	c := catalog.New(catalog.Deps{Pool: pool})
	cat, err := c.CreateCategory(ctx, catalog.NewCategory{Name: "Books"})
	if err != nil {
		return err
	}
	var invalid *catalog.InvalidFields
	if _, err := c.CreateCategory(ctx, catalog.NewCategory{Name: "Books"}); !errors.As(err, &invalid) || invalid.Fields[0].Code != "taken" {
		return fmt.Errorf("duplicate category: %v", err)
	}
	p, err := c.CreateProduct(ctx, catalog.NewProduct{Name: "Go", CategoryID: cat.ID})
	if err != nil {
		return err
	}
	page, err := c.ListProducts(ctx, "", 10)
	if err != nil || len(page.Products) != 1 || page.Products[0].ID != p.ID {
		return fmt.Errorf("list: %+v %v", page, err)
	}
	if err := c.DeleteCategory(ctx, cat.ID); !errors.Is(err, catalog.ErrCategoryInUse) {
		return fmt.Errorf("delete in use: %v", err)
	}
	if err := c.DeleteProduct(ctx, p.ID); err != nil {
		return err
	}
	if _, err := c.ReadProduct(ctx, p.ID); !errors.Is(err, catalog.ErrProductNotFound) {
		return fmt.Errorf("read deleted: %v", err)
	}
	return nil
}
`)
	run([]string{url}, "go", "run", "./cmd/try")
}
