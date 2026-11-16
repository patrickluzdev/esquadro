// Package env holds the Go code of the built-in env blueprint: its hook.
package env

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Files is the post-gen hook "files": it keeps a .env.<environment> file
// for each environment, which is the user's, and the .gitignore lines that
// keep them out of git. A new file lists every variable: in development,
// commented with its value, and the secrets to generate set; elsewhere, the
// required variables set empty, to fill in, and the others commented. An
// existing file gets the variables it does not mention yet, commented, and a
// note over those no declaration has any more; its values never change.
func Files(_ context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	if in.DryRun || len(in.Environments) == 0 {
		return out, nil
	}
	for i, env := range in.Environments {
		name := ".env." + env
		path := filepath.Join(in.ProjectDir, name)
		existing, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			if err := os.WriteFile(path, []byte(newFile(env, i == 0, in.Env)), 0o600); err != nil {
				return out, err
			}
			out.Messages = append(out.Messages, hook.Message{Level: "info", Text: "created " + name + ": your values for " + env + ", out of git"})
		case err != nil:
			return out, err
		default:
			updated, added := update(string(existing), env, i == 0, in.Env)
			if updated != string(existing) {
				if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
					return out, err
				}
			}
			if len(added) > 0 {
				out.Messages = append(out.Messages, hook.Message{Level: "info", Text: fmt.Sprintf("added %s to %s", strings.Join(added, ", "), name)})
			}
		}
	}
	return out, ignore(in.ProjectDir, in.Environments)
}

// newFile is a new .env file for an environment.
func newFile(env string, development bool, vars []project.EnvDescription) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# The environment variables of %s: yours, out of git. esquadro gen adds the\n", env)
	b.WriteString("# variables the model declares later, and never changes a value. A variable\n")
	b.WriteString("# set in the process's environment wins over this file. See .env.example.\n")
	for _, v := range vars {
		b.WriteString("\n")
		b.WriteString(entry(v, env, development))
	}
	return b.String()
}

// entry is a variable's lines in an environment's file.
func entry(v project.EnvDescription, env string, development bool) string {
	var b strings.Builder
	for _, d := range v.Doc {
		fmt.Fprintf(&b, "# %s\n", d)
	}
	switch {
	case development && v.Generate > 0:
		fmt.Fprintf(&b, "%s=%s\n", v.Name, generate(v.Generate))
	case slices.Contains(v.Required, env):
		fmt.Fprintf(&b, "%s=\n", v.Name)
	case v.Secret:
		fmt.Fprintf(&b, "# %s=\n", v.Name)
	default:
		fmt.Fprintf(&b, "# %s=%s\n", v.Name, v.Values[env])
	}
	return b.String()
}

var assignment = regexp.MustCompile(`^#?\s*(?:export\s+)?([A-Z][A-Z0-9_]*)=`)

// notDeclared is the note over a variable no declaration has any more.
const notDeclared = "# Not declared any more: esquadro does not read it."

// update adds the variables an existing file does not mention, and notes
// those it should not: its lines and values stay as they are.
func update(content, env string, development bool, vars []project.EnvDescription) (string, []string) {
	declared := map[string]bool{}
	for _, v := range vars {
		declared[v.Name] = true
	}
	mentioned := map[string]bool{}
	var lines []string
	s := bufio.NewScanner(strings.NewReader(content))
	for s.Scan() {
		line := s.Text()
		if m := assignment.FindStringSubmatch(line); m != nil {
			mentioned[m[1]] = true
			isSet := !strings.HasPrefix(strings.TrimSpace(line), "#")
			if isSet && !declared[m[1]] && (len(lines) == 0 || lines[len(lines)-1] != notDeclared) {
				lines = append(lines, notDeclared)
			}
		}
		lines = append(lines, line)
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	var added []string
	for _, v := range vars {
		if mentioned[v.Name] {
			continue
		}
		b.WriteString("\n")
		b.WriteString(entry(v, env, development))
		added = append(added, v.Name)
	}
	return b.String(), added
}

// generate is a random secret of n bytes, as URL-safe base64.
func generate(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ignore makes .gitignore keep every environment's file out of git.
func ignore(dir string, envs []string) error {
	path := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	have := map[string]bool{}
	for line := range strings.SplitSeq(string(content), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, env := range envs {
		if name := ".env." + env; !have[name] && !have["/"+name] && !have[".env.*"] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	text := string(content)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	text += "# Each environment's variables, secrets among them: never in git.\n" + strings.Join(missing, "\n") + "\n"
	return os.WriteFile(path, []byte(text), 0o644)
}
