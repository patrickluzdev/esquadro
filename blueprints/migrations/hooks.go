// Package migrations holds the Go code of the built-in migrations
// blueprint: the database the model describes, the migration from one model
// to the next, and its hook.
package migrations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// Write is the pre-gen hook "write": it writes the migration from
// the project of the last generation to this one, as a file of the user's
// (esquadro never changes it again), and refuses a project whose tables
// would collide.
func Write(_ context.Context, in hook.Input) (hook.Output, error) {
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

// holds reports whether one of the files has exactly content.
func holds(files []string, content string) bool {
	for _, f := range files {
		if data, err := os.ReadFile(f); err == nil && string(data) == content {
			return true
		}
	}
	return false
}
