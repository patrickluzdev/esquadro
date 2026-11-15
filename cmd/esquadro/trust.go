package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

// stdin is where gen asks before running hooks; interactive reports whether
// a person is there to answer. Tests replace both.
var (
	stdin       io.Reader = os.Stdin
	interactive           = func() bool {
		info, err := os.Stdin.Stat()
		return err == nil && info.Mode()&os.ModeCharDevice != 0
	}
)

// trustHooks makes sure the user lets the hooks of each blueprint from git
// run, once per commit: hooks are programs that run on this machine. The
// lockfile remembers the answer; trustAll (gen -trust) answers yes for every
// blueprint. It returns the names of the trusted blueprints.
func trustHooks(set *blueprint.Set, lock *workspace.Lock, trustAll bool, stderr io.Writer) (map[string]bool, error) {
	trusted := map[string]bool{}
	in := bufio.NewReader(stdin)
	for _, b := range set.Blueprints {
		if b.Commit == "" || len(b.Manifest.Hooks) == 0 {
			continue
		}
		if trustAll || trustedBefore(lock, b) {
			trusted[b.Manifest.Name] = true
			continue
		}
		var hooks []string
		for _, h := range b.Manifest.Hooks {
			hooks = append(hooks, h.Event+" "+h.Path)
		}
		if !interactive() {
			return nil, fmt.Errorf("blueprint %s from %s runs programs on this machine (%s): read them in %s, then run esquadro gen -trust",
				b.Manifest.Name, b.Location, strings.Join(hooks, ", "), b.Dir)
		}
		fmt.Fprintf(stderr, "blueprint %s from %s runs programs on this machine:\n", b.Manifest.Name, b.Location)
		for _, h := range hooks {
			fmt.Fprintf(stderr, "  %s\n", h)
		}
		fmt.Fprintf(stderr, "Their code is in %s.\nRun them? [y/N] ", b.Dir)
		answer, _ := in.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			return nil, fmt.Errorf("the hooks of blueprint %s were not allowed; nothing was generated", b.Manifest.Name)
		}
		trusted[b.Manifest.Name] = true
	}
	return trusted, nil
}

// trustedBefore reports whether the lockfile records that the user let b's
// hooks run at its commit.
func trustedBefore(lock *workspace.Lock, b *blueprint.Blueprint) bool {
	for _, lb := range lock.Blueprints {
		if lb.Source == b.Source && lb.Commit == b.Commit && lb.Trusted {
			return true
		}
	}
	return false
}
