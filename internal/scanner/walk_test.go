package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// hasGit reports whether the real git binary is available. These tests use
// real git (no mocks) per the reproducibility lesson — worktree .git-FILE
// vs dir, macOS /var -> /private/var symlink resolution, and remote-URL
// dedup all have shape that mocks hide.
func hasGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available; skipping real-git fixture test")
	}
}

// gitCfg is a helper to run git in a dir with deterministic identity.
func gitCfg(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
	}
}

// makeRepo init/commits a git repo at dir and sets its origin remote to url.
func makeRepo(t *testing.T, dir, url string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	gitCfg(t, dir, "init", "-q")
	gitCfg(t, dir, "config", "user.email", "test@example.com")
	gitCfg(t, dir, "config", "user.name", "Test")
	gitCfg(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	gitCfg(t, dir, "remote", "add", "origin", url)
}

// makeClone makes a clone of src into dir and sets its origin remote to url
// (overriding the local-filesystem URL a local clone leaves behind).
func makeClone(t *testing.T, src, dir, url string) {
	t.Helper()
	if out, err := exec.Command("git", "clone", "-q", src, dir).CombinedOutput(); err != nil {
		t.Fatalf("clone %s <- %s: %v\n%s", dir, src, err, out)
	}
	gitCfg(t, dir, "remote", "set-url", "origin", url)
}

// TestWalkReposDedupWorktreeNonGit is the headline fixture (V8 invariant).
//
//	root/
//	  alpha/       real repo, remote=alpha.git            (KEEP — own remote)
//	  alpha-mirror/ clone of alpha, remote=alpha.git      (DUP of alpha -> drop)
//	  beta/        real repo, remote=beta.git             (KEEP — own remote)
//	  wt/          linked worktree of alpha (.git FILE)   (worktree -> skip)
//	  _codex_worktrees/  reserved skip-name (empty)
//	  notgit/      plain directory                        (non-git -> skip)
//
// Expectation: WalkRepos returns {alpha, beta} (2 distinct). alpha-mirror
// deduped to alpha (same remote, alpha is lexically shallower). wt excluded
// (worktree). _codex_worktrees excluded (reserved name). notgit excluded.
//
// Two genuinely independent remotes (alpha.git vs beta.git) make the dedup
// assertion meaningful: a fixture where every repo shares one remote would
// trivially collapse to one survivor regardless of walker correctness.
func TestWalkReposDedupWorktreeNonGit(t *testing.T) {
	hasGit(t)
	root := t.TempDir()

	alphaURL := "https://example.com/alpha.git"
	betaURL := "https://example.com/beta.git"

	// alpha + beta: independent remotes, both kept.
	makeRepo(t, filepath.Join(root, "alpha"), alphaURL)
	makeRepo(t, filepath.Join(root, "beta"), betaURL)

	// alpha-mirror: clone of alpha with alpha's remote URL -> DUP of alpha.
	makeClone(t, filepath.Join(root, "alpha"), filepath.Join(root, "alpha-mirror"), alphaURL)

	// wt: linked worktree of alpha. `.git` is a regular FILE. Detection is
	// via that hallmark (NOT -type d -name .git, which would miss it).
	wtDir := filepath.Join(root, "wt")
	gitCfg(t, filepath.Join(root, "alpha"), "worktree", "add", "-q", "--detach", wtDir)
	// Sanity-assert the worktree really is a .git FILE — if git ever changes
	// worktree layout, we want to know loudly here, not in production.
	if fi, err := os.Stat(filepath.Join(wtDir, ".git")); err != nil {
		t.Fatalf("worktree .git stat: %v (layout changed?)", err)
	} else if !fi.Mode().IsRegular() {
		t.Fatalf("worktree .git is not a regular file: mode=%v", fi.Mode())
	}

	// Reserved skip-name dir + non-git dir.
	for _, n := range []string{"_codex_worktrees", "notgit"} {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}

	// Capture stderr (raw-vs-distinct count is logged there).
	var stderr strings.Builder
	repos, err := WalkRepos(root, &stderr)
	if err != nil {
		t.Fatalf("WalkRepos: %v\nstderr: %s", err, stderr.String())
	}

	// Key results on resolved basenames (macOS /var -> /private/var).
	got := map[string]string{}
	for _, r := range repos {
		realRoot, _ := filepath.EvalSymlinks(root)
		realPath, _ := filepath.EvalSymlinks(r.Path)
		rel, err := filepath.Rel(realRoot, realPath)
		if err != nil {
			t.Fatalf("Rel(%q, %q): %v", realRoot, realPath, err)
		}
		got[rel] = r.Remote
	}

	wantKeep := map[string]string{
		"alpha": alphaURL,
		"beta":  betaURL,
	}
	for name, wantRemote := range wantKeep {
		if gotRemote, ok := got[name]; !ok {
			t.Errorf("expected repo %q in results; got=%v", name, keysSorted(got))
		} else if gotRemote != wantRemote {
			t.Errorf("repo %q remote: got %q want %q", name, gotRemote, wantRemote)
		}
	}
	for _, name := range []string{"alpha-mirror", "wt", "_codex_worktrees", "notgit"} {
		if _, ok := got[name]; ok {
			t.Errorf("repo %q should be excluded; got=%v", name, keysSorted(got))
		}
	}

	// raw-vs-distinct count must be logged to stderr.
	if !strings.Contains(stderr.String(), "distinct") {
		t.Errorf("stderr should log raw-vs-distinct count; got:\n%s", stderr.String())
	}
	// raw should be 3 (alpha, beta, alpha-mirror — wt skipped pre-dedup).
	if !strings.Contains(stderr.String(), "raw=3") {
		t.Errorf("stderr should report raw=3 (alpha,beta,alpha-mirror; wt+reserved skipped pre-count); got:\n%s", stderr.String())
	}
}

// TestWalkReposNonExistentRoot errors cleanly.
func TestWalkReposNonExistentRoot(t *testing.T) {
	hasGit(t)
	var stderr strings.Builder
	_, err := WalkRepos(filepath.Join(t.TempDir(), "nope"), &stderr)
	if err == nil {
		t.Fatal("expected error for non-existent root")
	}
}

// TestWalkReposReservesAllSkipNames covers every anchored skip-name.
func TestWalkReposReservesAllSkipNames(t *testing.T) {
	hasGit(t)
	root := t.TempDir()
	for _, name := range []string{"_worktrees", "_codex_worktrees", ".codex-worktrees", "_tmp", ".claude", ".git"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	// One real repo to walk.
	makeRepo(t, filepath.Join(root, "real"), "https://example.com/real.git")

	var stderr strings.Builder
	repos, err := WalkRepos(root, &stderr)
	if err != nil {
		t.Fatalf("WalkRepos: %v", err)
	}
	if len(repos) != 1 {
		var names []string
		for _, r := range repos {
			names = append(names, filepath.Base(r.Path))
		}
		t.Fatalf("expected exactly 1 repo (real); got %d: %v", len(repos), names)
	}
}

// TestWalkReposOriginlessKept verifies a repo with no origin remote is still
// walked (not silently dropped) and not deduped against other origin-less
// repos (each is a distinct local).
func TestWalkReposOriginlessKept(t *testing.T) {
	hasGit(t)
	root := t.TempDir()
	// Two repos, neither has an origin remote.
	for _, name := range []string{"local1", "local2"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		gitCfg(t, dir, "init", "-q")
		gitCfg(t, dir, "config", "user.email", "test@example.com")
		gitCfg(t, dir, "config", "user.name", "Test")
		gitCfg(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
		// Deliberately no `git remote add`.
	}
	var stderr strings.Builder
	repos, err := WalkRepos(root, &stderr)
	if err != nil {
		t.Fatalf("WalkRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected both origin-less repos kept; got %d", len(repos))
	}
}

func keysSorted(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// silence unused import if platform differs (only exec used on all).
var _ = runtime.GOOS
