// Package app holds the Go code of the built-in app blueprint: its
// hooks.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// GoMod is the post-gen hook "go-mod". It creates go.mod when the project has
// none, brings each module a blueprint requires up to its minimum version
// when the code uses it, and tidies go.mod and go.sum. Both files stay the
// user's: they may add, upgrade or remove modules freely.
func GoMod(ctx context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	gocmd, err := exec.LookPath("go")
	if err != nil {
		return out, errors.New("the go command is not on PATH; install Go from https://go.dev/dl")
	}
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, gocmd, args...)
		cmd.Dir = in.ProjectDir
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		err := cmd.Run()
		if err != nil {
			return buf.String(), fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(buf.String()))
		}
		return buf.String(), nil
	}

	if _, err := os.Stat(filepath.Join(in.ProjectDir, "go.mod")); errors.Is(err, fs.ErrNotExist) {
		if _, err := run("mod", "init", in.Project.App.Module); err != nil {
			return out, err
		}
		out.Messages = append(out.Messages, hook.Message{Level: "info", Text: "created go.mod for " + in.Project.App.Module})
	}
	if _, err := run("mod", "tidy"); err != nil {
		return out, err
	}

	upgraded := false
	for _, req := range in.GoRequires {
		current, err := run("list", "-m", "-f", "{{.Version}}", req.Module)
		if err != nil {
			continue // not used by the code: nothing to require
		}
		if compareVersions(strings.TrimSpace(current), req.Version) >= 0 {
			continue
		}
		if _, err := run("get", req.Module+"@"+req.Version); err != nil {
			return out, err
		}
		upgraded = true
		out.Messages = append(out.Messages, hook.Message{Level: "info", Text: "upgraded " + req.Module + " to " + req.Version})
	}
	if upgraded {
		if _, err := run("mod", "tidy"); err != nil {
			return out, err
		}
	}
	return out, nil
}

// compareVersions compares semantic versions such as v1.2.3 and
// v1.2.3-rc.1, returning -1, 0 or 1. A pre-release sorts before its release;
// pre-releases compare as text.
func compareVersions(a, b string) int {
	ac, apre, _ := strings.Cut(strings.TrimPrefix(a, "v"), "-")
	bc, bpre, _ := strings.Cut(strings.TrimPrefix(b, "v"), "-")
	as, bs := strings.Split(ac, "."), strings.Split(bc, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case apre == bpre:
		return 0
	case apre == "":
		return 1
	case bpre == "":
		return -1
	case apre < bpre:
		return -1
	}
	return 1
}
