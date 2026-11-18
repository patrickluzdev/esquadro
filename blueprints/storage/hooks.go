package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// Ignore keeps the folder of the disk store in development (local_dir) out
// of git: the files people send are no code.
func Ignore(_ context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	if in.DryRun {
		return out, nil
	}
	dir, _ := in.Options["local_dir"].(string)
	if dir == "" {
		dir = "storage"
	}
	entry := "/" + strings.Trim(filepath.ToSlash(dir), "/") + "/"
	path := filepath.Join(in.ProjectDir, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return out, err
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		if strings.TrimSpace(line) == entry {
			return out, nil
		}
	}
	text := string(content)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	text += "# The files sent in development (storage's disk store).\n" + entry + "\n"
	return out, os.WriteFile(path, []byte(text), 0o644)
}
