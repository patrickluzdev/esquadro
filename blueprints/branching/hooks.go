// Package branching holds the Go code of the built-in branching blueprint:
// its hook.
package branching

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// Models are the branching models the blueprint knows.
var Models = []string{"gitflow", "github_flow", "gitlab_flow", "trunk", "release_flow"}

// Setup is the post-gen hook "setup": once the repository has its first
// commit, it creates the model's long-lived branches that are missing, from
// main, and, for gitflow, the git-flow extension's configuration. It never
// switches branches, pushes or changes a branch that exists.
func Setup(ctx context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	model, _ := in.Options["model"].(string)
	if !slices.Contains(Models, model) {
		return out, fmt.Errorf("model %q is not one of %s", model, strings.Join(Models, ", "))
	}
	if in.DryRun {
		return out, nil
	}
	gitcmd, err := exec.LookPath("git")
	if err != nil {
		return out, errors.New("git is not on PATH")
	}
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, gitcmd, args...)
		cmd.Dir = in.ProjectDir
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		err := cmd.Run()
		return strings.TrimSpace(buf.String()), err
	}
	main, _ := in.Options["main"].(string)
	if _, err := git("rev-parse", "-q", "--verify", "refs/heads/"+main); err != nil {
		out.Messages = append(out.Messages, hook.Message{Level: "info",
			Text: fmt.Sprintf("no branch %s with a commit yet: the %s branches are created once it has one (esquadro gen)", main, model)})
		return out, nil
	}

	var branches []string
	switch model {
	case "gitflow":
		develop, _ := in.Options["develop"].(string)
		branches = []string{develop}
	case "gitlab_flow":
		branches = stringList(in.Options["environments"])
	}
	var created []string
	for _, b := range branches {
		if _, err := git("rev-parse", "-q", "--verify", "refs/heads/"+b); err == nil {
			continue
		}
		if msg, err := git("branch", b, main); err != nil {
			return out, fmt.Errorf("git branch %s: %w\n%s", b, err, msg)
		}
		created = append(created, b)
	}
	if len(created) > 0 {
		out.Messages = append(out.Messages, hook.Message{Level: "info",
			Text: fmt.Sprintf("created %s from %s; to send: git push origin %s", strings.Join(created, ", "), main, strings.Join(created, " "))})
	}

	if model == "gitflow" {
		develop, _ := in.Options["develop"].(string)
		config := [][2]string{
			{"gitflow.branch.master", main},
			{"gitflow.branch.develop", develop},
			{"gitflow.prefix.feature", "feature/"},
			{"gitflow.prefix.bugfix", "bugfix/"},
			{"gitflow.prefix.release", "release/"},
			{"gitflow.prefix.hotfix", "hotfix/"},
			{"gitflow.prefix.support", "support/"},
			{"gitflow.prefix.versiontag", "v"},
		}
		for _, kv := range config {
			if v, err := git("config", "--local", "--get", kv[0]); err == nil && v != "" {
				continue // the user's own setting stays
			}
			if msg, err := git("config", "--local", kv[0], kv[1]); err != nil {
				return out, fmt.Errorf("git config %s: %w\n%s", kv[0], err, msg)
			}
		}
	}
	return out, nil
}

func stringList(v any) []string {
	var out []string
	switch v := v.(type) {
	case []string:
		out = v
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
