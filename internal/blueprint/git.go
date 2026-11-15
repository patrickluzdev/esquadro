package blueprint

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// GitSource is where a blueprint lives in a git repository, as an app writes
// it: "github.com/acme/esquadro-stripe@v1.2.0" (the repository is the
// blueprint), "github.com/acme/blueprints/stripe@v1.2.0" (a folder of it),
// or any URL git clones, with "//" before the folder:
// "https://git.example.com/blueprints.git//stripe@main".
type GitSource struct {
	// Repo is what git clones.
	Repo string
	// Dir is the blueprint's folder in the repository, "" for its root.
	Dir string
	// Ref is the tag, branch or commit.
	Ref string
}

// hosts are the forges whose repositories are host/owner/name, so that the
// rest of a path is a folder in the repository.
var hosts = []string{"github.com", "gitlab.com", "bitbucket.org", "codeberg.org"}

// ParseGitSource reads a git source as an app writes it.
func ParseGitSource(s string) (GitSource, error) {
	at := strings.LastIndex(s, "@")
	if at < 0 || at == len(s)-1 {
		return GitSource{}, errors.New("needs a version after @: a tag, a branch or a commit, e.g. @v1.0.0")
	}
	where, ref := s[:at], s[at+1:]
	if scheme, rest, ok := strings.Cut(where, "://"); ok {
		repo, dir, _ := strings.Cut(rest, "//")
		if repo == "" {
			return GitSource{}, fmt.Errorf("%s has no repository", s)
		}
		return GitSource{Repo: scheme + "://" + repo, Dir: strings.Trim(dir, "/"), Ref: ref}, nil
	}
	parts := strings.Split(strings.Trim(where, "/"), "/")
	for _, h := range hosts {
		if parts[0] == h {
			if len(parts) < 3 {
				return GitSource{}, fmt.Errorf("%s is not %s/owner/repository", where, h)
			}
			return GitSource{Repo: "https://" + strings.Join(parts[:3], "/") + ".git", Dir: path.Join(parts[3:]...), Ref: ref}, nil
		}
	}
	return GitSource{}, fmt.Errorf("%s is neither on %s nor a URL git clones (https://, ssh://, file://)", where, strings.Join(hosts, ", "))
}

// Git fetches blueprints from git repositories into a cache: a mirror of
// each repository, and the files of each commit used.
type Git struct {
	// CacheDir holds the mirrors and checkouts.
	CacheDir string
	// Pinned are the commits the lockfile recorded, by source as written,
	// used instead of resolving the source's ref again.
	Pinned map[string]string
	// Update resolves every ref again, ignoring Pinned.
	Update bool
	// Offline never clones nor fetches: a source works only at the commit
	// Pinned records, when the cache has it. The editor works this way.
	Offline bool
}

// ErrNotFetched is a source the cache does not have, in Offline mode.
var ErrNotFetched = errors.New("blueprint not fetched yet: run esquadro gen")

// DefaultCacheDir is ESQUADRO_CACHE/git, or the user's cache folder's
// esquadro/git.
func DefaultCacheDir() (string, error) {
	if dir := os.Getenv("ESQUADRO_CACHE"); dir != "" {
		return filepath.Join(dir, "git"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "esquadro", "git"), nil
}

// Checkout returns the folder holding the blueprint of source at its commit,
// and the commit.
func (g *Git) Checkout(ctx context.Context, source string) (dir, commit string, err error) {
	src, err := ParseGitSource(source)
	if err != nil {
		return "", "", err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", "", errors.New("git is not on PATH; blueprints from git need it")
	}
	sum := sha256.Sum256([]byte(src.Repo))
	key := hex.EncodeToString(sum[:8])
	mirror := filepath.Join(g.CacheDir, "repos", key)

	if g.Offline {
		return g.cached(ctx, source, src, mirror, key)
	}
	fetched := false
	if _, err := os.Stat(mirror); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
			return "", "", err
		}
		tmp, err := os.MkdirTemp(filepath.Dir(mirror), key+".tmp-")
		if err != nil {
			return "", "", err
		}
		defer os.RemoveAll(tmp)
		if _, err := git(ctx, "", "clone", "--mirror", "--quiet", src.Repo, tmp); err != nil {
			return "", "", fmt.Errorf("cannot clone %s: %w", src.Repo, err)
		}
		if err := os.Rename(tmp, mirror); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", "", err
		}
		fetched = true
	}
	fetch := func() error {
		if fetched {
			return nil
		}
		fetched = true
		if _, err := git(ctx, mirror, "fetch", "--quiet", "--prune", "origin"); err != nil {
			return fmt.Errorf("cannot fetch %s: %w", src.Repo, err)
		}
		return nil
	}

	if pinned := g.Pinned[source]; pinned != "" && !g.Update {
		commit = pinned
		if !hasCommit(ctx, mirror, commit) {
			if err := fetch(); err != nil {
				return "", "", err
			}
			if !hasCommit(ctx, mirror, commit) {
				return "", "", fmt.Errorf("%s has no commit %s, which the lockfile records; run esquadro gen -update to use %s again", src.Repo, commit, src.Ref)
			}
		}
	} else {
		// A new source, or an update: the ref is resolved against the
		// repository as it is now.
		if err := fetch(); err != nil {
			return "", "", err
		}
		out, err := git(ctx, mirror, "rev-parse", "--verify", "--quiet", src.Ref+"^{commit}")
		if err != nil {
			return "", "", fmt.Errorf("%s has no tag, branch or commit %q", src.Repo, src.Ref)
		}
		commit = out
	}

	return g.unpacked(ctx, src, mirror, key, commit)
}

// cached is Checkout without the network: the pinned commit, from its
// checkout or from the mirror.
func (g *Git) cached(ctx context.Context, source string, src GitSource, mirror, key string) (string, string, error) {
	commit := g.Pinned[source]
	if commit == "" {
		return "", "", ErrNotFetched
	}
	checkout := filepath.Join(g.CacheDir, "checkouts", key, commit)
	if _, err := os.Stat(checkout); err != nil && !hasCommit(ctx, mirror, commit) {
		return "", "", ErrNotFetched
	}
	return g.unpacked(ctx, src, mirror, key, commit)
}

// unpacked returns the blueprint's folder in the checkout of commit,
// extracting it from the mirror first when needed.
func (g *Git) unpacked(ctx context.Context, src GitSource, mirror, key, commit string) (string, string, error) {
	checkout := filepath.Join(g.CacheDir, "checkouts", key, commit)
	if _, err := os.Stat(checkout); errors.Is(err, fs.ErrNotExist) {
		if err := extract(ctx, mirror, commit, checkout); err != nil {
			return "", "", err
		}
	}
	dir := filepath.Join(checkout, filepath.FromSlash(src.Dir))
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("%s has no folder %s at %s", src.Repo, src.Dir, src.Ref)
	}
	return dir, commit, nil
}

// hasCommit reports whether the mirror has commit.
func hasCommit(ctx context.Context, mirror, commit string) bool {
	_, err := git(ctx, mirror, "cat-file", "-e", commit+"^{commit}")
	return err == nil
}

// extract writes the files of commit into dir, through a temporary folder so
// that an interrupted run leaves no half checkout behind.
func extract(ctx context.Context, mirror, commit, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), commit+".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	cmd := exec.CommandContext(ctx, "git", "archive", "--format=tar", commit)
	cmd.Dir = mirror
	cmd.Env = gitEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	untarErr := untar(out, tmp)
	// The tar reader stops at the end-of-archive marker, but git pads the
	// archive after it: read the rest, or git blocks on a full pipe (small
	// on Windows) and Wait never returns.
	_, _ = io.Copy(io.Discard, out)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive %s: %w: %s", commit, err, strings.TrimSpace(stderr.String()))
	}
	if untarErr != nil {
		return untarErr
	}
	if err := os.Rename(tmp, dir); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

// untar writes the regular files and folders of a tar stream into dir,
// keeping whether a file is executable (for hooks).
func untar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(h.Name)
		if name == "." || strings.HasPrefix(name, "../") || path.IsAbs(name) {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		}
	}
}

// git runs git in dir and returns its trimmed output.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// gitEnv is the environment git runs in: it never asks for a password or
// anything else on the terminal, so a repository that needs credentials
// fails at once instead of waiting for an answer that will not come.
func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
}
