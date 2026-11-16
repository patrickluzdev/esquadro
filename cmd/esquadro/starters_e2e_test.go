//go:build e2e

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestEveryKindOfProjectRuns generates the smallest project and one from
// each starter but api (TestLifecycle follows that one), and runs them: each
// builds, passes go vet and its tests, and answers version; the worker runs
// its job queue against Postgres and stops cleanly when asked to (its
// generated tests work a job through the queue).
func TestEveryKindOfProjectRuns(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatal("Docker is not available")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"", "cli", "worker"} {
		name := "smallest"
		if kind != "" {
			name = kind
		}
		t.Run(name, func(t *testing.T) {
			root, err := os.MkdirTemp(cache, "esquadro-e2e-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(root) })
			dir := filepath.Join(root, "shop")
			e := &lifecycle{t: t, dir: dir}
			args := []string{"new", "-module", "example.com/shop", dir}
			if kind != "" {
				args = append([]string{"new", "-starter", kind}, args[1:]...)
			}
			e.esquadro(0, args...)
			e.run(nil, "go", "vet", "./...")
			e.run(nil, "go", "test", "./...")
			// The version as the Dockerfile and the CI set it: not from git,
			// whose first commit needs a user.name the machine may not have.
			e.run(nil, "go", "build", "-ldflags", "-X main.version=v1.2.3", "-o", "bin/shop", "./cmd/shop")
			if out := e.run(nil, "bin/shop", "version"); out != "shop v1.2.3\n" {
				t.Fatalf("version: %q", out)
			}
			if kind != "worker" {
				return
			}
			db := e.postgres()
			e.run(db, "bin/shop", "migrate")
			cmd := exec.Command(filepath.Join(dir, "bin", "shop"))
			cmd.Env = append(os.Environ(), db...)
			var logs bytes.Buffer
			cmd.Stdout, cmd.Stderr = &logs, &logs
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// The job queue runs as a process of the worker.
			deadline := time.Now().Add(30 * time.Second)
			for !strings.Contains(logs.String(), "process started") || !strings.Contains(logs.String(), "process=jobs") {
				if time.Now().After(deadline) {
					cmd.Process.Kill()
					t.Fatalf("the worker did not start its job queue:\n%s", logs.String())
				}
				time.Sleep(100 * time.Millisecond)
			}
			cmd.Process.Signal(syscall.SIGTERM)
			if err := cmd.Wait(); err != nil || !strings.Contains(logs.String(), "process stopped") {
				t.Fatalf("stopping: %v\n%s", err, logs.String())
			}
		})
	}
}
