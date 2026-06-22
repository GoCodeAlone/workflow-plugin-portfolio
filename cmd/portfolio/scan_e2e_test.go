package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoCodeAlone/workflow-plugin-portfolio/internal/catalog"
)

// hasGitE2E skips when git is unavailable (the e2e test creates real git
// fixtures via subprocess).
func hasGitE2E(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available; skipping e2e git fixture test")
	}
}

// writeFixtureTaxonomy writes a minimal taxonomy.yaml that the real
// inventory.LoadTaxonomy will accept (version + one capability mapping).
func writeFixtureTaxonomy(t *testing.T, path string) {
	t.Helper()
	content := `version: test-2026-06-21
capabilities:
  - id: auth.authn
    category: auth
    name: Authentication
    description: Verifies identity.
    lifecycle: released
    aliases:
      moduleTypes: [auth.credential]
      plugins: [workflow-plugin-auth]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// makeFixtureGitRepo creates a real git repo at dir with one commit and an
// origin remote pointing at remoteURL (so WalkRepos dedup keys on it).
func makeFixtureGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if remoteURL != "" {
		run("remote", "add", "origin", remoteURL)
	}
	run("commit", "-q", "--allow-empty", "-m", "init")
}

// TestScanE2EWritesPortfolioWithBlocks is the end-to-end proof: a tempdir
// workspace with 2 fixture git repos + a fixture registry tree + fixture
// taxonomy; run scan; assert PORTFOLIO.md has a block per repo + a status:
// line per repo. Also confirms FOLLOWUPS.md is written.
func TestScanE2EWritesPortfolioWithBlocks(t *testing.T) {
	hasGitE2E(t)
	ws := t.TempDir()

	// Two fixture repos with distinct remotes (so dedup keeps both).
	repoAuth := filepath.Join(ws, "workflow-plugin-auth")
	repoInfra := filepath.Join(ws, "workflow-plugin-infra")
	if err := os.MkdirAll(repoAuth, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoInfra, 0o755); err != nil {
		t.Fatal(err)
	}
	makeFixtureGitRepo(t, repoAuth, "https://github.com/GoCodeAlone/workflow-plugin-auth.git")
	makeFixtureGitRepo(t, repoInfra, "https://github.com/GoCodeAlone/workflow-plugin-infra.git")

	// Fixture registry tree with one plugin manifest (for CapabilityInventory).
	regPlugins := filepath.Join(ws, "workflow-registry", "plugins", "workflow-plugin-auth")
	if err := os.MkdirAll(regPlugins, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"workflow-plugin-auth","version":"0.3.0","type":"external","status":"released","repository":"github.com/GoCodeAlone/workflow-plugin-auth","capabilities":{"moduleTypes":["auth.credential"]}}`
	if err := os.WriteFile(filepath.Join(regPlugins, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Fixture taxonomy.
	taxPath := filepath.Join(ws, "taxonomy.yaml")
	writeFixtureTaxonomy(t, taxPath)

	// docs dir for output.
	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runScanForTest(t.Context(), ws, taxPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("scan exit %d; stderr:\n%s\nstdout:\n%s", code, stderr.String(), stdout.String())
	}

	portfolioPath := filepath.Join(ws, "docs", "PORTFOLIO.md")
	data, err := os.ReadFile(portfolioPath)
	if err != nil {
		t.Fatalf("read PORTFOLIO.md: %v", err)
	}
	body := string(data)

	// Each repo should have a block + a status: line.
	for _, repo := range []string{
		"GoCodeAlone/workflow-plugin-auth",
		"GoCodeAlone/workflow-plugin-infra",
	} {
		if !strings.Contains(body, repo) {
			t.Errorf("PORTFOLIO.md missing repo block %q\n---\n%s", repo, body)
		}
		// Each block has a status: line (even if empty).
		blockCount := strings.Count(body, "status:")
		if blockCount < 2 {
			t.Errorf("expected >=2 status: lines (one per repo), got %d\n---\n%s", blockCount, body)
		}
	}

	// FOLLOWUPS.md should exist (even if empty of follow-ups).
	if _, err := os.Stat(filepath.Join(ws, "docs", "FOLLOWUPS.md")); err != nil {
		t.Errorf("FOLLOWUPS.md not written: %v", err)
	}
}

// TestScanE2EV1StatusPreservedAcrossRuns is the V1 HEADLINE E2E PROOF:
// run scan TWICE on the same workspace; all status: lines must be byte-
// identical between the two runs. ALSO: a curated status planted in a seed
// PORTFOLIO.md before the first run must survive BOTH runs verbatim.
func TestScanE2EV1StatusPreservedAcrossRuns(t *testing.T) {
	hasGitE2E(t)
	ws := t.TempDir()

	repoAuth := filepath.Join(ws, "workflow-plugin-auth")
	if err := os.MkdirAll(repoAuth, 0o755); err != nil {
		t.Fatal(err)
	}
	makeFixtureGitRepo(t, repoAuth, "https://github.com/GoCodeAlone/workflow-plugin-auth.git")

	// Fixture taxonomy.
	taxPath := filepath.Join(ws, "taxonomy.yaml")
	writeFixtureTaxonomy(t, taxPath)

	docsDir := filepath.Join(ws, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed an existing PORTFOLIO.md with a curated status for the auth repo.
	// This status MUST survive both scan runs byte-identical.
	curatedStatus := "active — v0.3.0 shipped 2026-06-02; Phase II (JWKS) in-flight"
	seedProjects := []catalog.Project{{
		Repo:     "GoCodeAlone/workflow-plugin-auth",
		Category: "plugin",
		Status:   curatedStatus,
	}}
	var seedBuf bytes.Buffer
	if err := catalog.WriteCatalog(&seedBuf, seedProjects, nil); err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(docsDir, "PORTFOLIO.md")
	if err := os.WriteFile(seedPath, seedBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run 1.
	var stdout1, stderr1 bytes.Buffer
	if code := runScanForTest(t.Context(), ws, taxPath, &stdout1, &stderr1); code != 0 {
		t.Fatalf("run1 scan exit %d; stderr:\n%s", code, stderr1.String())
	}
	run1Data, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read run1 PORTFOLIO.md: %v", err)
	}
	run1Body := string(run1Data)

	// The curated status must survive run 1.
	if !strings.Contains(run1Body, curatedStatus) {
		t.Errorf("V1 VIOLATION: curated status did not survive run 1\n---\n%s", run1Body)
	}

	// Run 2 (idempotency proof).
	var stdout2, stderr2 bytes.Buffer
	if code := runScanForTest(t.Context(), ws, taxPath, &stdout2, &stderr2); code != 0 {
		t.Fatalf("run2 scan exit %d; stderr:\n%s", code, stderr2.String())
	}
	run2Data, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read run2 PORTFOLIO.md: %v", err)
	}
	run2Body := string(run2Data)

	// Extract every status: block from both runs and compare byte-identical.
	statuses1 := extractStatusLines(run1Body)
	statuses2 := extractStatusLines(run2Body)
	if len(statuses1) != len(statuses2) {
		t.Fatalf("status line count drift: run1=%d run2=%d", len(statuses1), len(statuses2))
	}
	for i := range statuses1 {
		if statuses1[i] != statuses2[i] {
			t.Errorf("V1 VIOLATION: status line %d drift between run1 and run2:\n run1: %q\n run2: %q",
				i, statuses1[i], statuses2[i])
		}
	}

	// The curated status must STILL survive run 2.
	if !strings.Contains(run2Body, curatedStatus) {
		t.Errorf("V1 VIOLATION: curated status lost after run 2\n---\n%s", run2Body)
	}
}

// extractStatusLines pulls the indented status content lines from a rendered
// PORTFOLIO.md body, in order, for cross-run comparison.
func extractStatusLines(body string) []string {
	var out []string
	inStatus := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "status:") {
			inStatus = true
			continue
		}
		if inStatus {
			if trimmed == "" || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "- ") {
				inStatus = false
				continue
			}
			content := strings.TrimSpace(line)
			if content != "" {
				out = append(out, content)
			}
		}
	}
	return out
}

// TestScanE2EMissingTaxonomyDegrades proves the degradation path: when the
// taxonomy file is absent, scan still succeeds (exit 0), still writes
// PORTFOLIO.md with per-repo blocks, but OMITS the Tooling Inventory section.
// The capability path is skipped with a stderr warning (V5/V16).
func TestScanE2EMissingTaxonomyDegrades(t *testing.T) {
	hasGitE2E(t)
	ws := t.TempDir()

	repoAuth := filepath.Join(ws, "workflow-plugin-auth")
	if err := os.MkdirAll(repoAuth, 0o755); err != nil {
		t.Fatal(err)
	}
	makeFixtureGitRepo(t, repoAuth, "https://github.com/GoCodeAlone/workflow-plugin-auth.git")

	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}

	missingTax := filepath.Join(ws, "no-such-taxonomy.yaml")
	var stdout, stderr bytes.Buffer
	code := runScanForTest(t.Context(), ws, missingTax, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("degraded scan should exit 0, got %d; stderr:\n%s", code, stderr.String())
	}
	// PORTFOLIO.md still written with the repo block.
	data, err := os.ReadFile(filepath.Join(ws, "docs", "PORTFOLIO.md"))
	if err != nil {
		t.Fatalf("read PORTFOLIO.md: %v", err)
	}
	if !strings.Contains(string(data), "GoCodeAlone/workflow-plugin-auth") {
		t.Errorf("degraded scan missing repo block\n---\n%s", string(data))
	}
	// Tooling Inventory section should be absent (no taxonomy -> no inventory).
	if strings.Contains(string(data), "## Tooling Inventory") {
		t.Errorf("degraded scan should OMIT Tooling Inventory (no taxonomy)\n---\n%s", string(data))
	}
	// A stderr warning should mention taxonomy.
	if !strings.Contains(strings.ToLower(stderr.String()), "taxonomy") {
		t.Errorf("degraded scan should warn about taxonomy on stderr; got:\n%s", stderr.String())
	}
}

// TestStatusE2EPrintsOverview verifies the status subcommand prints an
// overview (counts by category + active/stale/abandoned markers).
func TestStatusE2EPrintsOverview(t *testing.T) {
	hasGitE2E(t)
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Seed a PORTFOLIO.md with a couple of projects.
	seed := `# Portfolio Catalog

## GoCodeAlone/workflow-plugin-auth

- category: plugin

status:
  active

## GoCodeAlone/workflow

- category: engine

status:
  active

## GoCodeAlone/workflow-plugin-orphan

- category: plugin

status:
  abandoned
`
	if err := os.WriteFile(filepath.Join(ws, "docs", "PORTFOLIO.md"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runStatusForTest(ws, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status exit %d; stderr:\n%s", code, stderr.String())
	}
	out := stdout.String()
	// Should mention category counts.
	for _, want := range []string{"plugin", "engine"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing category %q\n---\n%s", want, out)
		}
	}
}

// runScanForTest invokes the scan subcommand against a tempdir workspace,
// bypassing the gh visibility assert (which would fail on a fake workspace
// repo). Returns the exit code. Uses context.Background semantics.
func runScanForTest(ctx context.Context, ws, taxonomyPath string, stdout, stderr *bytes.Buffer) int {
	opts := scanOptions{
		workspaceRoot:     ws,
		taxonomyPath:      taxonomyPath,
		skipVisibility:    true, // e2e fixtures aren't real gh repos
		workflowVersion:   "test",
		visibilityTarget:  "GoCodeAlone/workspace",
	}
	return runScan(ctx, opts, stdout, stderr)
}

// runStatusForTest invokes the status subcommand.
func runStatusForTest(ws string, stdout, stderr *bytes.Buffer) int {
	return runStatus(ws, stdout, stderr)
}
