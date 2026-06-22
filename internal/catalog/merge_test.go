package catalog

import (
	"testing"
)

// TestMergePreservesCuratedStatus is the V1 HEADLINE test: a curated Status
// on an existing project MUST survive Merge byte-identical. The scanner
// never supplies Status (it's a human-curated field), so Merge must copy it
// verbatim from the existing entry.
func TestMergePreservesCuratedStatus(t *testing.T) {
	curated := "active — v0.3.0 shipped 2026-06-02; Phase II (JWKS) in-flight"
	existing := []Project{{
		Repo:     "GoCodeAlone/workflow-plugin-auth",
		Status:   curated,
		Category: "plugin",
	}}
	scanned := []Project{{
		Repo:     "GoCodeAlone/workflow-plugin-auth",
		Category: "plugin",
		Scan:     ScanFacts{LastCommitISO: "2026-06-21T10:00:00Z"},
		Release:  ReleaseFacts{LatestRelease: "v0.3.0", OpenPRs: 2, OpenIssues: 5},
	}}

	merged := Merge(existing, scanned)
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1", len(merged))
	}
	if merged[0].Status != curated {
		t.Errorf("Status after merge = %q, want %q (V1: curated Status MUST be byte-identical)", merged[0].Status, curated)
	}
	// Scanned facts MUST be rewritten (not preserved from existing).
	if merged[0].Scan.LastCommitISO != "2026-06-21T10:00:00Z" {
		t.Errorf("Scan.LastCommitISO = %q, want rewritten from scanned", merged[0].Scan.LastCommitISO)
	}
	if merged[0].Release.LatestRelease != "v0.3.0" {
		t.Errorf("Release.LatestRelease = %q, want rewritten from scanned", merged[0].Release.LatestRelease)
	}
}

// TestMergeStatusByteIdenticalAcrossTwoRuns is the V1 HEADLINE PROOF:
// running Merge TWICE (existing=output-of-first-run) must leave Status
// byte-identical. This proves the catalog generator is idempotent on the
// human-curated Status field — a 2nd-run diff on Status is empty. This is
// the test that catches a regression where Merge accidentally mutates
// Status (e.g. lowercasing, trimming, or overwriting with "").
func TestMergeStatusByteIdenticalAcrossTwoRuns(t *testing.T) {
	curated := "  active — multi-line\n  - v0.3.0 shipped\n  - Phase II pending  "
	existing := []Project{{
		Repo:     "GoCodeAlone/workflow-plugin-auth",
		Status:   curated,
		Category: "plugin",
	}}
	scanned := []Project{{
		Repo:     "GoCodeAlone/workflow-plugin-auth",
		Category: "plugin",
		Scan:     ScanFacts{LastCommitISO: "2026-06-21T10:00:00Z"},
	}}

	firstRun := Merge(existing, scanned)
	statusAfterFirst := firstRun[0].Status

	secondRun := Merge(firstRun, scanned)
	statusAfterSecond := secondRun[0].Status

	if statusAfterFirst != curated {
		t.Errorf("1st-run Status drift:\n got: %q\nwant: %q", statusAfterFirst, curated)
	}
	if statusAfterSecond != statusAfterFirst {
		t.Errorf("V1 VIOLATION: 2nd-run Status drift (must be idempotent):\n 1st: %q\n 2nd: %q", statusAfterFirst, statusAfterSecond)
	}
}

// TestMergeNewRepoGetsEmptyStatus verifies a repo present in scanned but not
// existing becomes a stub with empty Status (awaiting human curation).
func TestMergeNewRepoGetsEmptyStatus(t *testing.T) {
	scanned := []Project{{
		Repo:     "GoCodeAlone/workflow-plugin-new",
		Category: "plugin",
	}}
	merged := Merge(nil, scanned)
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1", len(merged))
	}
	if merged[0].Status != "" {
		t.Errorf("new repo Status = %q, want empty (stub awaiting curation)", merged[0].Status)
	}
}

// TestMergeStaleRepoKeptNotDeleted verifies a repo in existing but not
// scanned is KEPT (flagged stale), not deleted — the human may have just
// not scanned it this run; deleting would lose the curated Status.
func TestMergeStaleRepoKeptNotDeleted(t *testing.T) {
	existing := []Project{{
		Repo:     "GoCodeAlone/workflow-plugin-orphan",
		Status:   "abandoned 2026-01-01",
		Category: "plugin",
	}}
	merged := Merge(existing, nil)
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1 (stale repo kept, not deleted)", len(merged))
	}
	if merged[0].Repo != "GoCodeAlone/workflow-plugin-orphan" {
		t.Errorf("stale repo Repo = %q, want orphan", merged[0].Repo)
	}
}

// TestMergeMatchByRepo verifies matching is keyed on Repo (not path or name).
func TestMergeMatchByRepo(t *testing.T) {
	existing := []Project{
		{Repo: "A/x", Status: "active"},
		{Repo: "B/y", Status: "abandoned"},
	}
	scanned := []Project{
		{Repo: "B/y", Scan: ScanFacts{LastCommitISO: "2026-06-21T00:00:00Z"}},
		{Repo: "C/z", Scan: ScanFacts{LastCommitISO: "2026-06-21T00:00:00Z"}},
	}
	merged := Merge(existing, scanned)
	if len(merged) != 3 {
		t.Fatalf("merged len = %d, want 3 (A kept stale, B updated, C new)", len(merged))
	}
	byRepo := map[string]string{}
	for _, p := range merged {
		byRepo[p.Repo] = p.Status
	}
	if byRepo["A/x"] != "active" {
		t.Errorf("A/x Status = %q, want active (preserved from existing)", byRepo["A/x"])
	}
	if byRepo["B/y"] != "abandoned" {
		t.Errorf("B/y Status = %q, want abandoned (preserved; scanned supplies no Status)", byRepo["B/y"])
	}
	if byRepo["C/z"] != "" {
		t.Errorf("C/z Status = %q, want empty (new repo stub)", byRepo["C/z"])
	}
}
