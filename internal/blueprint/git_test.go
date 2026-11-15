package blueprint

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGitSource(t *testing.T) {
	for in, want := range map[string]GitSource{
		"github.com/acme/stripe@v1.2.0":                  {Repo: "https://github.com/acme/stripe.git", Ref: "v1.2.0"},
		"github.com/acme/blueprints/billing/stripe@main": {Repo: "https://github.com/acme/blueprints.git", Dir: "billing/stripe", Ref: "main"},
		"https://git.example.com/bp.git//stripe@abc123":  {Repo: "https://git.example.com/bp.git", Dir: "stripe", Ref: "abc123"},
		"file:///tmp/repo@v1":                            {Repo: "file:///tmp/repo", Ref: "v1"},
		"ssh://git@host.com/x/y.git@v2":                  {Repo: "ssh://git@host.com/x/y.git", Ref: "v2"},
	} {
		got, err := ParseGitSource(in)
		if err != nil || got != want {
			t.Errorf("ParseGitSource(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for in, msg := range map[string]string{
		"github.com/acme/stripe":     "needs a version after @",
		"github.com/acme@v1":         "is not github.com/owner/repository",
		"example.com/acme/stripe@v1": "neither on github.com",
	} {
		if _, err := ParseGitSource(in); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("ParseGitSource(%q): %v, want %q", in, err, msg)
		}
	}
}

// gitRepo makes a repository with a blueprint in bp/, tagged v1, and returns
// its file URL and a function that commits a change to bp/blueprint.esq on
// main.
func gitRepo(t *testing.T) (string, func(version string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(version string) {
		os.MkdirAll(filepath.Join(dir, "bp"), 0o755)
		os.WriteFile(filepath.Join(dir, "bp", ManifestFile), []byte("blueprint demo {\n  version \""+version+"\"\n}\n"), 0o644)
	}
	run("init", "-q")
	write("1.0.0")
	run("add", ".")
	run("commit", "-q", "-m", "v1")
	run("tag", "v1")
	return "file://" + dir, func(version string) string {
		write(version)
		run("commit", "-q", "-am", version)
		return run("rev-parse", "HEAD")
	}
}

func manifestVersion(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGitCheckoutPinsAndUpdates(t *testing.T) {
	url, commit := gitRepo(t)
	g := &Git{CacheDir: t.TempDir(), Pinned: map[string]string{}}
	ctx := context.Background()
	source := url + "//bp@main"

	dir, first, err := g.Checkout(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(manifestVersion(t, dir), "1.0.0") {
		t.Fatal("not the first commit")
	}
	second := commit("2.0.0")

	// Pinned: the lockfile's commit wins over the branch that moved.
	g.Pinned[source] = first
	if dir, got, err := g.Checkout(ctx, source); err != nil || got != first || !strings.Contains(manifestVersion(t, dir), "1.0.0") {
		t.Fatalf("pinned: %s, %v", got, err)
	}
	// Updated: the branch is resolved again.
	g.Update = true
	if dir, got, err := g.Checkout(ctx, source); err != nil || got != second || !strings.Contains(manifestVersion(t, dir), "2.0.0") {
		t.Fatalf("updated: %s, %v", got, err)
	}
	// A tag still names the first commit.
	if _, got, err := g.Checkout(ctx, url+"//bp@v1"); err != nil || got != first {
		t.Fatalf("tag: %s, %v", got, err)
	}
	if _, _, err := g.Checkout(ctx, url+"//bp@nope"); err == nil || !strings.Contains(err.Error(), `no tag, branch or commit "nope"`) {
		t.Fatalf("missing ref: %v", err)
	}
	if _, _, err := g.Checkout(ctx, url+"//other@v1"); err == nil || !strings.Contains(err.Error(), "has no folder other") {
		t.Fatalf("missing folder: %v", err)
	}
}

func TestGitOfflineReadsOnlyTheCache(t *testing.T) {
	url, _ := gitRepo(t)
	ctx := context.Background()
	cache := t.TempDir()
	source := url + "//bp@v1"

	offline := &Git{CacheDir: cache, Offline: true, Pinned: map[string]string{}}
	if _, _, err := offline.Checkout(ctx, source); !errors.Is(err, ErrNotFetched) {
		t.Fatalf("nothing cached: %v", err)
	}
	online := &Git{CacheDir: cache, Pinned: map[string]string{}}
	_, commit, err := online.Checkout(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	// Not pinned: the editor does not resolve refs.
	if _, _, err := offline.Checkout(ctx, source); !errors.Is(err, ErrNotFetched) {
		t.Fatalf("not pinned: %v", err)
	}
	offline.Pinned[source] = commit
	if dir, got, err := offline.Checkout(ctx, source); err != nil || got != commit || !strings.Contains(manifestVersion(t, dir), "1.0.0") {
		t.Fatalf("pinned and cached: %s, %v", got, err)
	}
	// The mirror has the commit, its files were never extracted: extracted.
	os.RemoveAll(filepath.Join(cache, "checkouts"))
	if _, got, err := offline.Checkout(ctx, source); err != nil || got != commit {
		t.Fatalf("extracted from the mirror: %s, %v", got, err)
	}
	offline.Pinned[source] = strings.Repeat("0", 40)
	if _, _, err := offline.Checkout(ctx, source); !errors.Is(err, ErrNotFetched) {
		t.Fatalf("unknown commit: %v", err)
	}
}
