package rest

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

// OgenVersion is the ogen that generates the server.
const OgenVersion = "v1.24.0"

// Ogen is the post-gen hook "ogen": it generates internal/apigen from the
// server's part of api/openapi.yaml (see ServerDocument).
func Ogen(ctx context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	gocmd, err := exec.LookPath("go")
	if err != nil {
		return out, errors.New("the go command is not on PATH; install Go from https://go.dev/dl")
	}
	published, err := os.ReadFile(filepath.Join(in.ProjectDir, "api", "openapi.yaml"))
	if err != nil {
		return out, err
	}
	tmp, err := os.MkdirTemp("", "esquadro-ogen-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(tmp)
	doc := filepath.Join(tmp, "openapi.yaml")
	if err := os.WriteFile(doc, ServerDocument(published), 0o644); err != nil {
		return out, err
	}
	cmd := exec.CommandContext(ctx, gocmd, "run", "github.com/ogen-go/ogen/cmd/ogen@"+OgenVersion,
		"--target", "internal/apigen", "--package", "apigen", "--clean", doc)
	cmd.Dir = in.ProjectDir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return out, fmt.Errorf("ogen: %w\n%s", err, strings.TrimSpace(buf.String()))
	}
	return out, nil
}

// apiBase is where the API's operations are mounted.
const apiBase = "/api/v1"

// dateTime is how the contract writes the schema of a time.
const dateTime = "type: string, format: date-time"

// timeLayout is RFC 3339 with the microseconds the database keeps, where
// ogen would write whole seconds: a time that a client reads and sends
// back, in a filter or in a PUT, is the one that is stored.
const timeLayout = "2006-01-02T15:04:05.999999Z07:00"

// ServerDocument is the part of the published contract ogen generates the
// API's server from: its paths under /api/v1, relative to it again; no
// identity routes or components, which are written by hand; and no model
// rules, so that the domain checks them and answers invalid_fields with
// every field at once, where ogen would refuse the first with a 400. Nor
// does it say which texts are decimals, for the same reason. Its times
// are written as timeLayout.
//
// The published contract is esquadro's own YAML, so its parts are found
// by their lines: top-level keys at column 0, paths at two spaces,
// components at four, and rules ending with ruleMark.
func ServerDocument(published []byte) []byte {
	var out []string
	section, sub := "", ""
	dropping := false
	for line := range strings.Lines(string(published)) {
		line = strings.TrimRight(line, "\n")
		if strings.HasSuffix(line, ruleMark) {
			continue
		}
		line = strings.ReplaceAll(line, ", format: decimal", "")
		line = strings.ReplaceAll(line, dateTime, dateTime+`, x-ogen-time-format: "`+timeLayout+`"`)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
		case indent == 0:
			section, dropping = strings.TrimSuffix(trimmed, ":"), false
		case section == "components" && indent == 2:
			sub, dropping = strings.TrimSuffix(trimmed, ":"), false
		case section == "paths" && indent == 2:
			path := strings.TrimSuffix(trimmed, ":")
			dropping = !strings.HasPrefix(path, apiBase+"/")
			line = "  " + strings.TrimPrefix(path, apiBase) + ":"
		case section == "components" && sub != "" && indent == 4:
			dropping = strings.HasPrefix(trimmed, "Identity")
		case section == "servers" && trimmed == "- url: /":
			line = "  - url: " + apiBase
		}
		if !dropping {
			out = append(out, line)
		}
	}
	return []byte(strings.Join(out, "\n") + "\n")
}
