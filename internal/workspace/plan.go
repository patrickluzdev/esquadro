package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/patrickluzdev/esquadro/internal/compose"
)

// Change is what a generation does to one file.
type Change int

const (
	Unchanged Change = iota // already as generated
	Create                  // new file
	Update                  // generated file, untouched by the user, with new content
	Delete                  // generated file, untouched, no longer generated
	Keep                    // the user's file (from a .once template): left alone
	Conflict                // would overwrite or delete the user's work
)

func (c Change) String() string {
	return [...]string{"unchanged", "create", "update", "delete", "keep", "conflict"}[c]
}

// Action is the planned change to one file.
type Action struct {
	Path   string // slash-separated, relative to the project
	Change Change
	// Old is the file on disk (nil when there is none) and New the generated
	// content (nil for deletes).
	Old, New []byte
	// Reason explains a Keep or a Conflict.
	Reason string
	// File is the generated file; nil for deletes.
	File *compose.File
	// ByHook names the post-gen hook that wrote the file, for conflicts on
	// files hooks write: forcing lets the hook overwrite them.
	ByHook string
	// locked is the file's previous lock entry, if any.
	locked *LockedFile
}

// Plan is every planned change, sorted by path.
type Plan struct {
	Actions []Action
	// hookFiles are the lock entries of files written by hooks, carried
	// over until the hooks run again.
	hookFiles map[string]LockedFile
}

// Conflicts returns the actions that would lose the user's work.
func (p *Plan) Conflicts() []Action {
	var out []Action
	for _, a := range p.Actions {
		if a.Change == Conflict {
			out = append(out, a)
		}
	}
	return out
}

// Count returns how many actions make each change.
func (p *Plan) Count() map[Change]int {
	n := map[Change]int{}
	for _, a := range p.Actions {
		n[a.Change]++
	}
	return n
}

// Conflict reasons.
const (
	reasonEdited     = "edited since esquadro generated it"
	reasonForeign    = "exists, but esquadro did not generate it"
	reasonEditedGone = "edited since esquadro generated it, and is no longer generated"
	reasonYours      = "yours: generated once, never updated"
	reasonNowYours   = "generated before, yours from now on"
	reasonDeleted    = "yours, and you deleted it: esquadro does not bring it back"
	reasonHookEdited = "edited since hook %s wrote it"
)

// MakePlan compares the generated files with the project folder and the
// lock of the last generation.
func MakePlan(projectDir string, res *compose.Result, lock *Lock) (*Plan, error) {
	p := &Plan{hookFiles: map[string]LockedFile{}}
	generated := map[string]bool{}
	for _, f := range res.Files {
		generated[f.Path] = true
		old, err := readIfExists(projectDir, f.Path)
		if err != nil {
			return nil, err
		}
		a := Action{Path: f.Path, Old: old, New: f.Content, File: f}
		if l, ok := lock.Files[f.Path]; ok {
			a.locked = &l
		}
		// A file esquadro generated that becomes the user's (a .once
		// template at its path): replaced if untouched, so the user gets the
		// new starting point instead of the old generated code.
		wasGenerated := f.Once && old != nil && a.locked != nil && !a.locked.Once
		switch {
		case wasGenerated && Hash(old) == a.locked.Hash:
			a.Change, a.Reason = Update, reasonNowYours
		case wasGenerated:
			a.Change, a.Reason = Conflict, reasonEdited
		case f.Once && old != nil:
			a.Change, a.Reason = Keep, reasonYours
		case f.Once && a.locked != nil:
			a.Change, a.Reason = Keep, reasonDeleted
		case old == nil:
			a.Change = Create
		case bytes.Equal(old, f.Content):
			a.Change = Unchanged
		case a.locked != nil && Hash(old) == a.locked.Hash:
			a.Change = Update
		case a.locked != nil:
			a.Change, a.Reason = Conflict, reasonEdited
		default:
			a.Change, a.Reason = Conflict, reasonForeign
		}
		p.Actions = append(p.Actions, a)
	}

	for path, l := range lock.Files {
		if l.Hook != "" && !generated[path] {
			p.hookFiles[path] = l
			old, err := readIfExists(projectDir, path)
			if err != nil {
				return nil, err
			}
			if old != nil && Hash(old) != l.Hash {
				p.Actions = append(p.Actions, Action{
					Path: path, Change: Conflict, Old: old, locked: &l, ByHook: l.Hook,
					Reason: fmt.Sprintf(reasonHookEdited, l.Hook),
				})
			}
			continue
		}
		if generated[path] || l.Once {
			continue // a .once file no longer generated simply stays the user's
		}
		old, err := readIfExists(projectDir, path)
		if err != nil {
			return nil, err
		}
		if old == nil {
			continue // already gone
		}
		a := Action{Path: path, Old: old, locked: &l}
		if Hash(old) == l.Hash {
			a.Change = Delete
		} else {
			a.Change, a.Reason = Conflict, reasonEditedGone
		}
		p.Actions = append(p.Actions, a)
	}
	sort.Slice(p.Actions, func(i, j int) bool { return p.Actions[i].Path < p.Actions[j].Path })
	return p, nil
}

func readIfExists(projectDir, path string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(path)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
