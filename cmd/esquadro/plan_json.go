package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/patrickluzdev/esquadro/changes"
	"github.com/patrickluzdev/esquadro/internal/version"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

// planJSON is esquadro gen -json: what a generation would do, for an
// editor to show. Field names are part of esquadro's interface: change them
// as a breaking change.
type planJSON struct {
	Esquadro string `json:"esquadro"`
	// Project is the esquadro that generated the project last, "" for none.
	Project string `json:"project"`
	// Versions is same, newer (run esquadro upgrade), older (this esquadro
	// cannot generate it) or development.
	Versions string `json:"versions"`
	// Notes are each version's changes for the project, when newer.
	Notes   []noteJSON     `json:"notes"`
	Actions []actionJSON   `json:"actions"`
	Counts  map[string]int `json:"counts"`
	// Out is where the new content of each changed file was written, with
	// -out.
	Out string `json:"out,omitempty"`
}

type noteJSON struct {
	Version string `json:"version"`
	Text    string `json:"text"`
}

type actionJSON struct {
	Path      string `json:"path"`
	Change    string `json:"change"`
	Reason    string `json:"reason,omitempty"`
	Conflicts int    `json:"conflicts,omitempty"`
	// Markers: the file on disk has conflicts a merge left to resolve.
	Markers   bool   `json:"markers,omitempty"`
	Once      bool   `json:"once,omitempty"`
	Blueprint string `json:"blueprint,omitempty"`
	Template  string `json:"template,omitempty"`
	// New is the file's new content under out, relative to it.
	New string `json:"new,omitempty"`
}

var versionNames = map[workspace.Versions]string{
	workspace.SameVersion:        "same",
	workspace.NewerVersion:       "newer",
	workspace.OlderVersion:       "older",
	workspace.DevelopmentVersion: "development",
}

// newPlanJSON describes the versions of a project; plan, when there is
// one, what generating it would do, the new content of each changed file
// written under out (when not "").
func newPlanJSON(project string, plan *workspace.Plan, out string) (*planJSON, error) {
	running := version.String()
	v := workspace.CompareVersions(project, running)
	p := &planJSON{Esquadro: running, Project: project, Versions: versionNames[v], Notes: []noteJSON{}, Actions: []actionJSON{}, Counts: map[string]int{}, Out: out}
	if v == workspace.NewerVersion || v == workspace.DevelopmentVersion {
		to := running
		if !workspace.Released(running) {
			to = ""
		}
		for _, n := range changes.Between(project, to) {
			p.Notes = append(p.Notes, noteJSON{Version: n.Version, Text: n.Text})
		}
	}
	if plan == nil {
		return p, nil
	}
	for c, n := range plan.Count() {
		p.Counts[c.String()] = n
	}
	for _, a := range plan.Actions {
		if a.Change == workspace.Unchanged {
			continue
		}
		j := actionJSON{Path: a.Path, Change: a.Change.String(), Reason: a.Reason, Conflicts: a.Conflicts, Markers: workspace.HasConflictMarkers(a.Old)}
		if a.File != nil {
			j.Once, j.Blueprint, j.Template = a.File.Once, a.File.Blueprint, a.File.Template
		}
		if out != "" && a.New != nil && a.Change != workspace.Keep && !j.Markers {
			path := filepath.Join(out, filepath.FromSlash(a.Path))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(path, a.New, 0o644); err != nil {
				return nil, err
			}
			j.New = a.Path
		}
		p.Actions = append(p.Actions, j)
	}
	return p, nil
}

func writePlanJSON(w io.Writer, p *planJSON) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}
