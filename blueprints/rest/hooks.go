package rest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// OgenVersion is the ogen that generates the server.
const OgenVersion = "v1.24.0"

// Ogen is the post-gen hook "ogen": it generates internal/apigen from
// api/openapi.yaml.
func Ogen(ctx context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	gocmd, err := exec.LookPath("go")
	if err != nil {
		return out, errors.New("the go command is not on PATH; install Go from https://go.dev/dl")
	}
	cmd := exec.CommandContext(ctx, gocmd, "run", "github.com/ogen-go/ogen/cmd/ogen@"+OgenVersion,
		"--target", "internal/apigen", "--package", "apigen", "--clean", "api/openapi.yaml")
	cmd.Dir = in.ProjectDir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return out, fmt.Errorf("ogen: %w\n%s", err, strings.TrimSpace(buf.String()))
	}
	return out, nil
}
