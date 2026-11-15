package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// ErrConflicts means the plan would lose the user's work and was not forced.
var ErrConflicts = errors.New("workspace: generating would overwrite files you edited")

// Blueprint describes a blueprint for the lockfile.
type Blueprint = LockedBlueprint

// Write applies a plan to the project folder and records it in the lockfile.
//
// Without force, a plan with conflicts writes nothing and returns
// ErrConflicts. Otherwise every new content is first written to a staging
// folder inside esquadro/, then moved into place; files to delete are
// removed, along with folders they leave empty; the lockfile is written
// last, so an interrupted run never records files it did not write.
func Write(projectDir string, p *Plan, force bool, esquadroVersion string, blueprints []Blueprint) error {
	if len(p.Conflicts()) > 0 && !force {
		return ErrConflicts
	}
	stateDir := filepath.Join(projectDir, Dir)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(stateDir, ".staging-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	type move struct{ from, to string }
	var moves []move
	var deletes []string
	for i, a := range p.Actions {
		target := filepath.Join(projectDir, filepath.FromSlash(a.Path))
		switch {
		case a.Change == Create, a.Change == Update, a.Change == Conflict && a.New != nil:
			staged := filepath.Join(staging, fmt.Sprintf("%d", i))
			mode := os.FileMode(0o644)
			if info, err := os.Stat(target); err == nil {
				mode = info.Mode().Perm()
			}
			if err := os.WriteFile(staged, a.New, mode); err != nil {
				return err
			}
			moves = append(moves, move{staged, target})
		case a.Change == Delete, a.Change == Conflict && a.New == nil && a.ByHook == "":
			deletes = append(deletes, target)
		}
	}

	for _, m := range moves {
		if err := os.MkdirAll(filepath.Dir(m.to), 0o755); err != nil {
			return err
		}
		if err := os.Rename(m.from, m.to); err != nil {
			return err
		}
	}
	for _, d := range deletes {
		if err := os.Remove(d); err != nil && !os.IsNotExist(err) {
			return err
		}
		removeEmptyParents(projectDir, filepath.Dir(d))
	}

	lock := &Lock{Esquadro: esquadroVersion, Blueprints: blueprints, Files: map[string]LockedFile{}}
	maps.Copy(lock.Files, p.hookFiles)
	for _, a := range p.Actions {
		if a.File == nil {
			continue // deleted
		}
		switch {
		case a.Change == Keep && a.locked != nil:
			lock.Files[a.Path] = *a.locked
		case a.Change == Keep:
			// A .once file the user already had before esquadro generated
			// it: record it as theirs without claiming its content.
			lock.Files[a.Path] = entry(a, "")
		default:
			lock.Files[a.Path] = entry(a, Hash(a.New))
		}
	}
	return writeAtomic(filepath.Join(projectDir, filepath.FromSlash(LockPath)), lock.encode())
}

func entry(a Action, hash string) LockedFile {
	f := a.File
	e := LockedFile{Hash: hash, Blueprint: f.Blueprint, Template: f.Template, Once: f.Once, Replaced: f.Replaced}
	for _, s := range f.Slots {
		e.Slots = append(e.Slots, Slot{Slot: s.Slot, Contributors: s.Contributors})
	}
	return e
}

// removeEmptyParents removes dir and its parents while they are empty,
// stopping at the project folder.
func removeEmptyParents(projectDir, dir string) {
	root := filepath.Clean(projectDir)
	for {
		dir = filepath.Clean(dir)
		if dir == root || !strings.HasPrefix(dir, root+string(filepath.Separator)) {
			return
		}
		if err := os.Remove(dir); err != nil { // fails when not empty
			return
		}
		dir = filepath.Dir(dir)
	}
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// TrackHookFiles records in the lockfile the files a post-gen hook wrote: every
// file of the project matching its writes globs. Entries of that hook for
// files that no longer exist are dropped. match is the glob matcher.
func TrackHookFiles(projectDir, hook, blueprint string, writes []string, match func(patterns []string, name string) bool) error {
	lock, err := ReadLock(projectDir)
	if err != nil {
		return err
	}
	for path, l := range lock.Files {
		if l.Hook == hook {
			delete(lock.Files, path)
		}
	}
	err = filepath.WalkDir(projectDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(projectDir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == Dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !match(writes, rel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		lock.Files[rel] = LockedFile{Hash: Hash(data), Blueprint: blueprint, Hook: hook}
		return nil
	})
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(projectDir, filepath.FromSlash(LockPath)), lock.encode())
}
