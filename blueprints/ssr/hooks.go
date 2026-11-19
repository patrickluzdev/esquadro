package ssr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// TemplVersion is the templ that writes the Go of a site's templ files;
// the generated code requires the same version of its runtime.
const TemplVersion = "v0.3.1118"

// Templ is the post-gen hook "templ": it writes the Go of every templ file
// under web/ (the sites'), the generated ones and yours.
func Templ(ctx context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	gocmd, err := exec.LookPath("go")
	if err != nil {
		return out, errors.New("the go command is not on PATH; install Go from https://go.dev/dl")
	}
	cmd := exec.CommandContext(ctx, gocmd, "run", "github.com/a-h/templ/cmd/templ@"+TemplVersion, "generate", "-path", "web")
	cmd.Dir = in.ProjectDir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return out, fmt.Errorf("templ: %w\n%s", err, strings.TrimSpace(buf.String()))
	}
	return out, nil
}
