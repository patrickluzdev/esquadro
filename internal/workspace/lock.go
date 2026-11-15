// Package workspace turns composed files into changes on disk: it compares
// them with the project folder and the lockfile of the last generation,
// plans what to create, update and delete, refuses to overwrite files the
// user edited, and writes everything or nothing.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// Dir is the folder, inside the project, of everything esquadro reads and
// keeps: the .esq files, local blueprints, the lockfile and the snapshot.
// The rest of the project is the generated code.
const Dir = "esquadro"

// LockPath is the lockfile's path relative to the project.
const LockPath = Dir + "/lock.json"

// Lock records what the last generation wrote, so the next one can tell
// generated files the user edited from untouched ones.
type Lock struct {
	Esquadro   string                `json:"esquadro"`
	Blueprints []LockedBlueprint     `json:"blueprints"`
	Files      map[string]LockedFile `json:"files"`
}

// LockedBlueprint is a blueprint used by the last generation.
type LockedBlueprint struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Location string `json:"location"`
}

// LockedFile is a file written by the last generation.
type LockedFile struct {
	// Hash is the SHA-256 of the content esquadro wrote.
	Hash      string   `json:"hash"`
	Blueprint string   `json:"blueprint"`
	Template  string   `json:"template"`
	Once      bool     `json:"once,omitempty"`
	Replaced  []string `json:"replaced,omitempty"`
	Slots     []Slot   `json:"slots,omitempty"`
	// Hook names the post-gen hook that wrote the file; Template is then
	// empty.
	Hook string `json:"hook,omitempty"`
}

// Slot records who contributed to a slot in a file.
type Slot struct {
	Slot         string   `json:"slot"`
	Contributors []string `json:"contributors"`
}

// Hash returns the hex SHA-256 of content.
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// ReadLock reads the project's lockfile. A project never generated has no
// lockfile; ReadLock then returns an empty lock.
func ReadLock(projectDir string) (*Lock, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(LockPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return &Lock{Files: map[string]LockedFile{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("%s is not a valid lockfile: %w", LockPath, err)
	}
	if l.Files == nil {
		l.Files = map[string]LockedFile{}
	}
	return &l, nil
}

// encode returns the lockfile's contents: indented JSON with sorted keys, so
// it diffs well in version control.
func (l *Lock) encode() []byte {
	data, _ := json.MarshalIndent(l, "", "  ")
	return append(data, '\n')
}

// SnapshotPath is where the description of the last generated project is
// kept, so the next generation can tell what changed (for migrations).
const SnapshotPath = Dir + "/model.json"

// ReadSnapshot reads the project description saved by the last generation,
// or nil when there is none.
func ReadSnapshot(projectDir string) (*project.Description, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(SnapshotPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d project.Description
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%s is not a valid snapshot: %w", SnapshotPath, err)
	}
	return &d, nil
}

// WriteSnapshot saves the project description of a successful generation.
func WriteSnapshot(projectDir string, d project.Description) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(projectDir, Dir), 0o755); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(projectDir, filepath.FromSlash(SnapshotPath)), append(data, '\n'))
}
