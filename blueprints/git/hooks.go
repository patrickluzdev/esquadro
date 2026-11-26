// Package git holds the Go code of the built-in git blueprint: its hook.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// Init is the post-gen hook "init": it creates the repository when the
// project has none, on the branch option (or branching's main), and, unless
// initial_commit is false, commits everything when the repository has no
// commit yet.
func Init(ctx context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	gitcmd, err := exec.LookPath("git")
	if err != nil {
		return out, errors.New("git is not on PATH; install it, or remove the git blueprint")
	}
	path, _ := in.Options["path"].(string)
	root := filepath.Join(in.ProjectDir, filepath.FromSlash(repoRootOf(path)))
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, gitcmd, args...)
		cmd.Dir = root
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		err := cmd.Run()
		return strings.TrimSpace(buf.String()), err
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); errors.Is(err, os.ErrNotExist) {
		branch, _ := in.Options["branch"].(string)
		if main, ok := in.Blueprints["branching"]["main"].(string); ok && main != "" {
			branch = main
		}
		if branch == "" {
			branch = "main"
		}
		if msg, err := git("init", "-q", "-b", branch); err != nil {
			return out, fmt.Errorf("git init: %w\n%s", err, msg)
		}
		text := "created a git repository"
		if root != in.ProjectDir {
			text += " at " + root
		}
		out.Messages = append(out.Messages, hook.Message{Level: "info", Text: text})
	}
	if commit, _ := in.Options["initial_commit"].(bool); !commit {
		return out, nil
	}
	if _, err := git("rev-parse", "--verify", "-q", "HEAD"); err == nil {
		return out, nil // not the first commit: the user commits from now on
	}
	if msg, err := git("add", "-A"); err != nil {
		return out, fmt.Errorf("git add: %w\n%s", err, msg)
	}
	message, _ := in.Options["message"].(string)
	if msg, err := git("commit", "-q", "-m", message); err != nil {
		// Most often git does not know the user yet; the files are staged.
		out.Messages = append(out.Messages, hook.Message{Level: "warning",
			Text: "the first commit failed, the files are staged: " + msg})
		return out, nil
	}
	out.Messages = append(out.Messages, hook.Message{Level: "info", Text: "committed the project: " + message})
	return out, nil
}
