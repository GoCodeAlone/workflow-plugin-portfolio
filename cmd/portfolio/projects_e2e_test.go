package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScanE2EProjectsRollupPopulatesScanRows is the TASK 4 e2e proof: scan a
// tempdir workspace with a fixture PROJECTS.md + 2 fixture repos (whose remotes
// match the project's repos) → both PORTFOLIO.md + PROJECTS.md are written;
// PROJECTS.md scan rows are populated from the roll-up; curated fields are
// preserved verbatim.
func TestScanE2EProjectsRollupPopulatesScanRows(t *testing.T) {
	hasGitE2E(t)
	ws := t.TempDir()

	// Two fixture repos whose remotes match the PROJECTS.md repos list.
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

	// Fixture taxonomy (degraded path is fine — we only care about the
	// PROJECTS.md roll-up here, not the Tooling Inventory section).
	taxPath := filepath.Join(ws, "taxonomy.yaml")
	writeFixtureTaxonomy(t, taxPath)

	docsDir := filepath.Join(ws, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed PROJECTS.md with curated fields + empty scan rows.
	curatedGoal := "cross-repo infrastructure layer"
	curatedBlockers := "none yet"
	curatedNext := "wire PROJECTS.md roll-up"
	curatedDesign := "docs/plans/projects-layer-design.md"
	seedProjectsMD := `# Projects

<!-- intro + block-shape legend -->

## infra-layer

status: active
phase: 2

- repos: GoCodeAlone/workflow-plugin-auth, GoCodeAlone/workflow-plugin-infra
- goal: ` + curatedGoal + `
- blockers: ` + curatedBlockers + `
- next: ` + curatedNext + `
- design: ` + curatedDesign + `
- scan:
`
	projectsPath := filepath.Join(docsDir, "PROJECTS.md")
	if err := os.WriteFile(projectsPath, []byte(seedProjectsMD), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run scan.
	var stdout, stderr bytes.Buffer
	code := runScanForTest(t.Context(), ws, taxPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("scan exit %d; stderr:\n%s\nstdout:\n%s", code, stderr.String(), stdout.String())
	}

	// PORTFOLIO.md written (existing behavior, unchanged).
	if _, err := os.Stat(filepath.Join(docsDir, "PORTFOLIO.md")); err != nil {
		t.Fatalf("PORTFOLIO.md not written: %v", err)
	}

	// PROJECTS.md written with populated scan rows.
	data, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatalf("read PROJECTS.md: %v", err)
	}
	body := string(data)

	// The scan row must now be populated (no longer empty after "scan:").
	if !strings.Contains(body, "scan: last-activity") {
		t.Errorf("PROJECTS.md scan row not populated:\n%s", body)
	}
	if strings.Contains(body, "- scan:\n") {
		t.Errorf("PROJECTS.md still has an empty scan row:\n%s", body)
	}

	// Curated fields preserved verbatim (P-V1).
	for _, want := range []string{
		"status: active",
		"phase: 2",
		"- repos: GoCodeAlone/workflow-plugin-auth, GoCodeAlone/workflow-plugin-infra",
		"- goal: " + curatedGoal,
		"- blockers: " + curatedBlockers,
		"- next: " + curatedNext,
		"- design: " + curatedDesign,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("P-V1 VIOLATION: curated field not preserved: %q\n---\n%s", want, body)
		}
	}
}

// TestScanE2EProjectsRollupIdempotentCuratedPreserved is the P-V1 e2e proof:
// run scan TWICE with a seeded PROJECTS.md; all curated fields are byte-
// identical between the two runs (only the scan row is rewritten).
func TestScanE2EProjectsRollupIdempotentCuratedPreserved(t *testing.T) {
	hasGitE2E(t)
	ws := t.TempDir()

	repoAuth := filepath.Join(ws, "workflow-plugin-auth")
	if err := os.MkdirAll(repoAuth, 0o755); err != nil {
		t.Fatal(err)
	}
	makeFixtureGitRepo(t, repoAuth, "https://github.com/GoCodeAlone/workflow-plugin-auth.git")

	taxPath := filepath.Join(ws, "taxonomy.yaml")
	writeFixtureTaxonomy(t, taxPath)

	docsDir := filepath.Join(ws, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedProjectsMD := `# Projects

## single

status: active
phase: 1

- repos: GoCodeAlone/workflow-plugin-auth
- goal: a goal
- blockers: a blocker
- next: a next
- design: a design
- scan:
`
	projectsPath := filepath.Join(docsDir, "PROJECTS.md")
	if err := os.WriteFile(projectsPath, []byte(seedProjectsMD), 0o644); err != nil {
		t.Fatal(err)
	}

	runOnce := func() string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := runScanForTest(t.Context(), ws, taxPath, &stdout, &stderr); code != 0 {
			t.Fatalf("scan exit %d; stderr:\n%s", code, stderr.String())
		}
		data, err := os.ReadFile(projectsPath)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	run1 := runOnce()
	run2 := runOnce()

	// Extract the curated-field lines (everything except the scan row) and
	// compare byte-identical. The scan row may carry the same date on both
	// runs (same-day), but the curated lines must never drift.
	for _, field := range []string{"status:", "phase:", "- repos:", "- goal:", "- blockers:", "- next:", "- design:"} {
		l1 := extractLine(run1, field)
		l2 := extractLine(run2, field)
		if l1 == "" || l1 != l2 {
			t.Errorf("P-V1 VIOLATION: %s line drifted between run1 and run2:\n run1: %q\n run2: %q", field, l1, l2)
		}
	}
}

// TestScanE2EProjectsRollupOptInNotCreatedWhenAbsent confirms PROJECTS.md is
// OPT-IN: when it does not exist before scan, scan does NOT create it.
func TestScanE2EProjectsRollupOptInNotCreatedWhenAbsent(t *testing.T) {
	hasGitE2E(t)
	ws := t.TempDir()

	repoAuth := filepath.Join(ws, "workflow-plugin-auth")
	if err := os.MkdirAll(repoAuth, 0o755); err != nil {
		t.Fatal(err)
	}
	makeFixtureGitRepo(t, repoAuth, "https://github.com/GoCodeAlone/workflow-plugin-auth.git")

	taxPath := filepath.Join(ws, "taxonomy.yaml")
	writeFixtureTaxonomy(t, taxPath)

	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runScanForTest(t.Context(), ws, taxPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("scan exit %d; stderr:\n%s", code, stderr.String())
	}

	// PROJECTS.md must NOT have been created (opt-in).
	if _, err := os.Stat(filepath.Join(ws, "docs", "PROJECTS.md")); err == nil {
		t.Error("PROJECTS.md was created by scan — it should be opt-in (only rolled up when it already exists)")
	}
	// stdout should not claim to have written PROJECTS.md.
	if strings.Contains(stdout.String(), "PROJECTS.md") {
		t.Errorf("scan claimed to write PROJECTS.md when it should be skipped:\n%s", stdout.String())
	}
}

// extractLine returns the first line in body that contains substr (trimmed).
func extractLine(body, substr string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(strings.TrimSpace(line), substr) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
