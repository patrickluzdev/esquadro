package postgres

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// TestMigrationsApplyToPostgres runs the migrations of testdata/shop against a
// real Postgres in Docker: every Up in order, then every Down in reverse, so
// both directions are valid SQL in the order they are written. It needs
// Docker and is skipped without it or with -short.
func TestMigrationsApplyToPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a Postgres container")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker is not available")
	}
	out, err := exec.Command("docker", "run", "-d", "--rm", "-e", "POSTGRES_PASSWORD=esquadro", "postgres:17-alpine").Output()
	if err != nil {
		t.Fatalf("docker run: %v", err)
	}
	container := strings.TrimSpace(string(out))
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", container).Run() })

	deadline := time.Now().Add(60 * time.Second)
	for exec.Command("docker", "exec", container, "pg_isready", "-U", "postgres").Run() != nil {
		if time.Now().After(deadline) {
			t.Fatal("Postgres did not start")
		}
		time.Sleep(300 * time.Millisecond)
	}
	// pg_isready can answer before the server accepts SQL during initdb's
	// restart; wait for a real query.
	for psql(container, "SELECT 1;") != nil {
		if time.Now().After(deadline) {
			t.Fatal("Postgres does not accept queries")
		}
		time.Sleep(300 * time.Millisecond)
	}

	versions, _ := filepath.Glob("testdata/shop/v*")
	var prev *project.Description
	var migrations []Migration
	for _, v := range versions {
		cur := load(t, v)
		migrations = append(migrations, Diff(prev, cur))
		prev = snapshot(t, cur)
	}
	for i, m := range migrations {
		if err := psql(container, m.Up); err != nil {
			t.Fatalf("up of %s: %v", filepath.Base(versions[i]), err)
		}
	}
	for i, m := range slices.Backward(migrations) {
		if err := psql(container, m.Down); err != nil {
			t.Fatalf("down of %s: %v", filepath.Base(versions[i]), err)
		}
	}
	// Every Down applied: nothing of the project may be left.
	if err := psql(container, `DO $$ BEGIN
	IF EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name IN ('catalog', 'orders')) THEN
		RAISE EXCEPTION 'schemas left behind after every down';
	END IF;
	IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public') THEN
		RAISE EXCEPTION 'tables left behind in public after every down';
	END IF;
	IF EXISTS (SELECT 1 FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = 'public' AND t.typtype = 'e') THEN
		RAISE EXCEPTION 'enum types left behind in public after every down';
	END IF;
END $$;`); err != nil {
		t.Fatal(err)
	}
}

// psql runs SQL in the container, stopping at the first error.
func psql(container, sql string) error {
	cmd := exec.Command("docker", "exec", "-i", container, "psql", "-U", "postgres", "-v", "ON_ERROR_STOP=1", "-q")
	cmd.Stdin = strings.NewReader(sql)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return &psqlError{err: err, output: out.String()}
	}
	return nil
}

type psqlError struct {
	err    error
	output string
}

func (e *psqlError) Error() string { return e.err.Error() + "\n" + e.output }
