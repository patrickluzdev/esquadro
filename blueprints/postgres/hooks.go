package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// Migrations is the pre-gen hook "migrations": it writes the migration from
// the project of the last generation to this one, as a file of the user's
// (esquadro never changes it again), and refuses a project whose tables
// would collide.
func Migrations(_ context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	if problems := Check(in.Project); len(problems) > 0 {
		return out, errors.New(strings.Join(problems, "; "))
	}
	m := Diff(in.Previous, in.Project)
	if in.Previous == nil {
		// Without the last project, the migration creates everything. That is
		// right for a new project, and for one whose first generation failed
		// after writing this very migration; anything else would apply the
		// whole schema twice.
		existing, err := filepath.Glob(filepath.Join(in.ProjectDir, MigrationsDir, "*.sql"))
		if err != nil {
			return out, err
		}
		if len(existing) > 0 && !holds(existing, m.File()) {
			return out, fmt.Errorf("%s/ has migrations, but esquadro/model.json, the project they were written from, is missing; "+
				"restore it from version control (or, if a first generation failed, delete those migrations), or esquadro would write the whole schema again", MigrationsDir)
		}
	}
	name, err := FileName(in.ProjectDir, m)
	if err != nil || name == "" {
		return out, err
	}
	out.Files = append(out.Files, hook.File{Path: name, Content: m.File(), Once: true})
	for _, w := range m.Warnings {
		out.Messages = append(out.Messages, hook.Message{Level: "warning", Text: name + ": " + w})
	}
	return out, nil
}

// SQLCImage is the sqlc image used when sqlc is not installed, as in
// studyloop's "task gen:sqlc".
const SQLCImage = "sqlc/sqlc:1.31.1"

// SQLC is the post-gen hook "sqlc": it runs sqlc generate with the installed
// sqlc, or with Docker when sqlc is not installed.
func SQLC(ctx context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	if _, err := os.Stat(filepath.Join(in.ProjectDir, "sqlc.yaml")); errors.Is(err, fs.ErrNotExist) {
		return out, nil // no entities: nothing to generate
	}
	var cmd *exec.Cmd
	if path, err := exec.LookPath("sqlc"); err == nil {
		cmd = exec.CommandContext(ctx, path, "generate")
		cmd.Dir = in.ProjectDir
	} else if docker, err := exec.LookPath("docker"); err == nil {
		args := []string{"run", "--rm", "-v", in.ProjectDir + ":/src", "-w", "/src"}
		if runtime.GOOS == "linux" {
			// Files written in the container belong to the user, not root.
			args = append(args, "-u", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
		}
		cmd = exec.CommandContext(ctx, docker, append(args, SQLCImage, "generate")...)
		out.Messages = append(out.Messages, hook.Message{Level: "info", Text: "sqlc is not installed; running it with Docker (" + SQLCImage + ")"})
	} else {
		return out, errors.New("needs sqlc or Docker: install sqlc (https://docs.sqlc.dev/en/latest/overview/install.html) or Docker")
	}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(buf.String())
		if strings.Contains(msg, "file does not exist") && strings.Contains(cmd.Path, "docker") {
			msg += "\nDocker could not see the project folder " + in.ProjectDir +
				": share it with Docker (Settings > Resources > File sharing), move the project under your home folder, or install sqlc"
		}
		return out, fmt.Errorf("sqlc generate: %w\n%s", err, msg)
	}
	return out, nil
}

// holds reports whether one of the files has exactly content.
func holds(files []string, content string) bool {
	for _, f := range files {
		if data, err := os.ReadFile(f); err == nil && string(data) == content {
			return true
		}
	}
	return false
}
