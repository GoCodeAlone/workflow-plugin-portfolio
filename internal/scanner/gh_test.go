package scanner

import (
	"os/exec"
	"testing"
)

// hasGH reports whether the real gh CLI binary is available. GHFacts uses
// real gh subprocesses; the per-repo cache and skip-error contract are
// exercised against the actual binary's output shape (PR/issue/release JSON).
func hasGH(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh binary not available; skipping real-gh test")
	}
}

// TestGHFactsCacheReusesResult proves the in-process per-repo cache: a
// second call for the same repo returns identical values and does NOT
// re-invoke gh (verified by mutating the cache directly between calls —
// if a fresh gh invocation ran, the mutated cache entry would be
// overwritten; we assert it survives because the cache hit short-circuits).
func TestGHFactsCacheReusesResult(t *testing.T) {
	hasGH(t)
	repo := "GoCodeAlone/workflow" // a real, public repo with PRs/issues/releases

	prs1, issues1, rel1, err1 := GHFacts(repo)
	if err1 != nil {
		t.Skipf("GHFacts(%q) returned skip error (gh offline/rate-limited): %v", repo, err1)
	}

	// Poison the cache: if the second call hits the cache, it returns the
	// poisoned values rather than re-querying gh.
	ghCache.mu.Lock()
	if e, ok := ghCache.m[repo]; ok {
		e.openPRs = 999989
		e.openIssues = 999988
		e.latestRelease = "POISONED-CACHE-SENTINEL"
		ghCache.m[repo] = e
	}
	ghCache.mu.Unlock()

	prs2, issues2, rel2, err2 := GHFacts(repo)
	if err2 != nil {
		t.Fatalf("second GHFacts(%q): %v", repo, err2)
	}
	if prs2 != 999989 || issues2 != 999988 || rel2 != "POISONED-CACHE-SENTINEL" {
		t.Errorf("cache miss: expected poisoned sentinels, got prs=%d issues=%d rel=%q (first call: prs=%d issues=%d rel=%q)",
			prs2, issues2, rel2, prs1, issues1, rel1)
	}
}

// TestGHFactsMissingRepoReturnsSkipError verifies a nonexistent repo yields
// the sentinel skip error (non-nil) so callers can degrade per V5.
func TestGHFactsMissingRepoReturnsSkipError(t *testing.T) {
	hasGH(t)
	_, _, _, err := GHFacts("GoCodeAlone/__definitely_not_a_real_repo_xyz_123__")
	if err == nil {
		t.Fatal("expected non-nil skip error for nonexistent repo, got nil")
	}
}

// TestGHFactsEmptyRepoReturnsError verifies an empty-string repo name is
// rejected early (not passed to gh) with a skip error.
func TestGHFactsEmptyRepoReturnsError(t *testing.T) {
	_, _, _, err := GHFacts("")
	if err == nil {
		t.Fatal("expected skip error for empty repo name, got nil")
	}
}
