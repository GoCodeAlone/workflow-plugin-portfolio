package projects

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestParseProjectsRecoversMapping is the TASK 2 headline proof: a fixture
// PROJECTS.md with 3 project blocks is parsed into Project structs with the
// repos list split on comma + trimmed, and every curated field recovered.
// The existing scan: value is preserved (so Merge can detect unchanged).
func TestParseProjectsRecoversMapping(t *testing.T) {
	const body = `# Projects

<!-- Intro. Block shape: ## <project>, status:, phase:, - repos: (comma-sep
     full-names), - goal/- blockers/- next/- design (curated), - scan
     (generator-written). -->

## auth-stack

status: active
phase: 2

- repos: GoCodeAlone/workflow-plugin-auth, GoCodeAlone/auth
- goal: cross-service asymmetric auth
- blockers: JWKS refresh not implemented
- next: wire JWKS verify-only mode
- design: docs/plans/auth-design.md
- scan: last-activity 2026-06-02, active 2/2 repos, open-PRs 3, open-issues 5

## dns-catalog

status: in-flight
phase: 1

- repos: GoCodeAlone/workflow-plugin-infra, GoCodeAlone/gocodealone-dns, GoCodeAlone/hover
- goal: canonical DNS catalog
- blockers: Hover write-path unverified
- next: live-test migration
- design: docs/plans/dns-design.md
- scan: last-activity 2026-06-05, active 3/3 repos, open-PRs 1, open-issues 0

## portfolio-layer

status: planning
phase: 0

- repos: GoCodeAlone/workflow-plugin-portfolio
- goal: cross-repo portfolio roll-up
- blockers: none
- next: implement PROJECTS.md roll-up
- design: docs/plans/projects-layer-design.md
- scan: last-activity 2026-06-21, active 1/1 repos, open-PRs 0, open-issues 0

## Unmapped

- GoCodeAlone/workflow
- GoCodeAlone/workflow-registry
`
	path := writeFixture(t, "PROJECTS.md", body)

	got, err := ParseProjects(path)
	if err != nil {
		t.Fatalf("ParseProjects: %v", err)
	}

	want := []Project{
		{
			Name:     "auth-stack",
			Status:   "active",
			Phase:    "2",
			Repos:    []string{"GoCodeAlone/workflow-plugin-auth", "GoCodeAlone/auth"},
			Goal:     "cross-service asymmetric auth",
			Blockers: "JWKS refresh not implemented",
			Next:     "wire JWKS verify-only mode",
			Design:   "docs/plans/auth-design.md",
			Scan:     "last-activity 2026-06-02, active 2/2 repos, open-PRs 3, open-issues 5",
		},
		{
			Name:     "dns-catalog",
			Status:   "in-flight",
			Phase:    "1",
			Repos:    []string{"GoCodeAlone/workflow-plugin-infra", "GoCodeAlone/gocodealone-dns", "GoCodeAlone/hover"},
			Goal:     "canonical DNS catalog",
			Blockers: "Hover write-path unverified",
			Next:     "live-test migration",
			Design:   "docs/plans/dns-design.md",
			Scan:     "last-activity 2026-06-05, active 3/3 repos, open-PRs 1, open-issues 0",
		},
		{
			Name:     "portfolio-layer",
			Status:   "planning",
			Phase:    "0",
			Repos:    []string{"GoCodeAlone/workflow-plugin-portfolio"},
			Goal:     "cross-repo portfolio roll-up",
			Blockers: "none",
			Next:     "implement PROJECTS.md roll-up",
			Design:   "docs/plans/projects-layer-design.md",
			Scan:     "last-activity 2026-06-21, active 1/1 repos, open-PRs 0, open-issues 0",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseProjects mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// TestParseProjectsUnmappedIgnoredAsProject confirms the ## Unmapped section
// is NOT parsed as a project (it is generator-written, regenerated not
// parsed-as-project).
func TestParseProjectsUnmappedIgnoredAsProject(t *testing.T) {
	const body = `# Projects

## real-project

status: active
phase: 1

- repos: GoCodeAlone/workflow-plugin-auth
- goal: a goal
- scan: last-activity 2026-06-02, active 1/1 repos, open-PRs 0, open-issues 0

## Unmapped

- GoCodeAlone/orphan-repo
`
	path := writeFixture(t, "PROJECTS.md", body)
	got, err := ParseProjects(path)
	if err != nil {
		t.Fatalf("ParseProjects: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 project (Unmapped must not be parsed as project), got %d: %#v", len(got), got)
	}
	if got[0].Name != "real-project" {
		t.Errorf("expected real-project, got %q", got[0].Name)
	}
}

// TestParseProjectsMissingFileIsError: a missing path is an error (caller
// decides opt-in skip before calling).
func TestParseProjectsMissingFileIsError(t *testing.T) {
	_, err := ParseProjects(filepath.Join(t.TempDir(), "nope.md"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestParseProjectsEmptyScanPreserved: a project with an empty scan: value
// (newly curated, not yet scanned) preserves the empty string so Merge can
// fill it.
func TestParseProjectsEmptyScanPreserved(t *testing.T) {
	const body = `# Projects

## new-project

status: planning
phase: 0

- repos: GoCodeAlone/new-repo
- goal: just started
- scan:
`
	path := writeFixture(t, "PROJECTS.md", body)
	got, err := ParseProjects(path)
	if err != nil {
		t.Fatalf("ParseProjects: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 project, got %d", len(got))
	}
	if got[0].Scan != "" {
		t.Errorf("expected empty scan for unscanned project, got %q", got[0].Scan)
	}
	if got[0].Goal != "just started" {
		t.Errorf("goal not recovered: %q", got[0].Goal)
	}
}

// writeFixture writes content to a temp file and returns its path.
func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
