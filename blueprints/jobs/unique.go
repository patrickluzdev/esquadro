package jobs

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/patrickluzdev/esquadro/blueprints/ui"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Unique is how a job is unique (@jobs(unique ...)), or nil when it is
// not: enqueued again, it is not enqueued twice.
type Unique struct {
	// By are the arguments it is unique by; empty, all of them.
	By []string
	// For is the period it is unique within (Go: 24 * time.Hour), "" for
	// ever.
	For string
	// Queued keeps it unique only while it waits or runs: once done, it may
	// be enqueued again.
	Queued bool
}

// UniqueOf is how a job is unique, nil when it is not.
func UniqueOf(j *project.Job) *Unique {
	a := j.Ann["jobs"]
	on, _ := a["unique"].(bool)
	by := ui.Strings(a["unique_by"])
	if !on && len(by) == 0 {
		return nil
	}
	u := &Unique{By: by}
	if s, _ := a["unique_for"].(string); s != "" {
		if d, err := Period(s); err == nil {
			u.For = goDuration(d)
		}
	}
	while, _ := a["unique_while"].(string)
	u.Queued = while == "queued"
	return u
}

// UniqueBy reports whether a job is unique by an argument (one of
// unique_by's), which the generated code marks river:"unique".
func UniqueBy(j *project.Job, f *project.Field) bool {
	return slices.Contains(ui.Strings(j.Ann["jobs"]["unique_by"]), f.Snake)
}

// Period reads a period as unique_for writes it: 10m, 24h, 7d.
func Period(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("%q is not a period", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%q is not a period", s)
	}
	return d, nil
}

// goDuration is a duration as Go code writes it: 24 * time.Hour.
func goDuration(d time.Duration) string {
	for _, u := range []struct {
		d    time.Duration
		name string
	}{{time.Hour, "time.Hour"}, {time.Minute, "time.Minute"}, {time.Second, "time.Second"}} {
		if d%u.d == 0 {
			return fmt.Sprintf("%d * %s", d/u.d, u.name)
		}
	}
	return fmt.Sprintf("time.Duration(%d)", d)
}

// Check checks the jobs' @jobs: who they are unique by, for how long and
// while what.
func Check(p *project.Project, _ *blueprint.Set) diag.List {
	var diags diag.List
	for _, j := range p.Jobs {
		a := j.Ann["jobs"]
		for _, name := range ui.Strings(a["unique_by"]) {
			if !slices.ContainsFunc(j.Fields, func(f *project.Field) bool { return f.Snake == name }) {
				diags.Errorf(j.Pos, j.Pos, "unique_by: %s is not an argument of %s", name, j.Raw)
			}
		}
		if s, _ := a["unique_for"].(string); s != "" {
			if _, err := Period(s); err != nil {
				diags.Errorf(j.Pos, j.Pos, "unique_for of %s: %s: write it as 10m, 24h or 7d", j.Raw, err)
			}
		}
		if while, _ := a["unique_while"].(string); while != "" && while != "ever" && while != "queued" {
			diags.Errorf(j.Pos, j.Pos, "unique_while %s of %s is not ever or queued", while, j.Raw)
		}
		if UniqueOf(j) == nil && (a["unique_for"] != nil || a["unique_while"] != nil) {
			diags.Errorf(j.Pos, j.Pos, "%s has unique_for or unique_while but is not unique: add unique, or unique_by", j.Raw)
		}
	}
	return diags
}
