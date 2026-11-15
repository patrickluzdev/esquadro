package store

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
