package projects

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// TestMergeRewritesOnlyScanRow is the P-V1 HEADLINE proof: seeded projects
// with curated fields, run Merge twice; curated fields (status, phase, repos,
// goal, blockers, next, design) are byte-identical across both merges, and
// ONLY the scan row changes.
func TestMergeRewritesOnlyScanRow(t *testing.T) {
	existing := []Project{
		{
			Name:     "auth-stack",
			Status:   "active",
			Phase:    "2",
			Repos:    []string{"GoCodeAlone/workflow-plugin-auth", "GoCodeAlone/auth"},
			Goal:     "cross-service asymmetric auth",
			Blockers: "JWKS refresh",
			Next:     "wire JWKS verify",
			Design:   "docs/design.md",
			Scan:     "last-activity 2026-05-01, active 1/2 repos, open-PRs 0, open-issues 0", // STALE old scan
		},
	}

	scans := map[string]Scan{
		"auth-stack": {
			LastActivity: "2026-06-02",
			ActiveRepos:  2,
			TotalRepos:   2,
			OpenPRs:      3,
			OpenIssues:   5,
		},
	}

	merged1 := Merge(existing, scans)
	// Run Merge again on the merged output — curated fields must be identical.
	merged2 := Merge(merged1, scans)

	if len(merged1) != 1 || len(merged2) != 1 {
		t.Fatalf("expected 1 project each, got %d / %d", len(merged1), len(merged2))
	}

	// Curated fields byte-identical across both merges (P-V1).
	for _, field := range []struct{ name string }{{"Name"}, {"Status"}, {"Phase"}, {"Goal"}, {"Blockers"}, {"Next"}, {"Design"}} {
		v1 := reflect.ValueOf(merged1[0]).FieldByName(field.name).String()
		v2 := reflect.ValueOf(merged2[0]).FieldByName(field.name).String()
		if v1 != v2 {
			t.Errorf("P-V1 VIOLATION: field %s drifted between merge runs: %q vs %q", field.name, v1, v2)
		}
	}
	if !reflect.DeepEqual(merged1[0].Repos, merged2[0].Repos) {
		t.Errorf("P-V1 VIOLATION: Repos drifted between merge runs")
	}

	// The scan row WAS rewritten from the stale value to the new roll-up.
	wantScan := "last-activity 2026-06-02, active 2/2 repos, open-PRs 3, open-issues 5"
	if merged1[0].Scan != wantScan {
		t.Errorf("merged1 Scan = %q, want %q", merged1[0].Scan, wantScan)
	}
	if merged2[0].Scan != wantScan {
		t.Errorf("merged2 Scan = %q, want %q (idempotent)", merged2[0].Scan, wantScan)
	}
}

// TestMergeAllFailedRendersQuestionMark: when a project's Scan has AllFailed,
// Merge renders the scan row as "?" (P-V3).
func TestMergeAllFailedRendersQuestionMark(t *testing.T) {
	existing := []Project{
		{Name: "ghost", Repos: []string{"GoCodeAlone/none"}, Goal: "g"},
	}
	scans := map[string]Scan{
		"ghost": {AllFailed: true, TotalRepos: 1},
	}
	merged := Merge(existing, scans)
	if merged[0].Scan != "?" {
		t.Errorf("AllFailed Scan = %q, want ?", merged[0].Scan)
	}
	if merged[0].Goal != "g" {
		t.Errorf("curated Goal not preserved: %q", merged[0].Goal)
	}
}

// TestMergeProjectWithoutScanKeepsEmpty: a project in existing with no
// corresponding scan entry keeps its (possibly empty) scan as-is — Merge does
// not invent scan data.
func TestMergeProjectWithoutScanKeepsEmpty(t *testing.T) {
	existing := []Project{
		{Name: "p", Goal: "g", Scan: "preexisting"},
	}
	merged := Merge(existing, nil)
	if merged[0].Scan != "preexisting" {
		t.Errorf("Scan with no roll-up = %q, want preexisting (untouched)", merged[0].Scan)
	}
}

// TestMergeFormatsScanRow verifies the exact scan row format.
func TestMergeFormatsScanRow(t *testing.T) {
	existing := []Project{{Name: "p"}}
	scans := map[string]Scan{
		"p": {LastActivity: "2026-06-05", ActiveRepos: 3, TotalRepos: 5, OpenPRs: 2, OpenIssues: 7},
	}
	merged := Merge(existing, scans)
	want := "last-activity 2026-06-05, active 3/5 repos, open-PRs 2, open-issues 7"
	if merged[0].Scan != want {
		t.Errorf("Scan = %q, want %q", merged[0].Scan, want)
	}
}

// TestWriteMergeRoundTripParsesBack is the full P-V1 round-trip: Write ->
// ParseProjects -> Merge produces the same curated fields. This proves the
// writer's output can be parsed back losslessly.
func TestWriteMergeRoundTripParsesBack(t *testing.T) {
	original := []Project{
		{
			Name:     "auth-stack",
			Status:   "active",
			Phase:    "2",
			Repos:    []string{"GoCodeAlone/workflow-plugin-auth", "GoCodeAlone/auth"},
			Goal:     "cross-service auth",
			Blockers: "JWKS",
			Next:     "wire verify",
			Design:   "docs/d.md",
			Scan:     "last-activity 2026-06-02, active 2/2 repos, open-PRs 3, open-issues 5",
		},
	}
	unmapped := []string{"GoCodeAlone/orphan"}

	var buf bytes.Buffer
	if err := Write(&buf, original, unmapped); err != nil {
		t.Fatal(err)
	}

	// Write to a temp file so ParseProjects can read it.
	path := writeFixture(t, "PROJECTS.md", buf.String())
	parsed, err := ParseProjects(path)
	if err != nil {
		t.Fatalf("ParseProjects: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 project, got %d", len(parsed))
	}
	// Curated fields round-trip.
	for _, field := range []struct{ name, want string }{
		{"Name", "auth-stack"},
		{"Status", "active"},
		{"Phase", "2"},
		{"Goal", "cross-service auth"},
		{"Blockers", "JWKS"},
		{"Next", "wire verify"},
		{"Design", "docs/d.md"},
	} {
		got := reflect.ValueOf(parsed[0]).FieldByName(field.name).String()
		if got != field.want {
			t.Errorf("round-trip %s = %q, want %q", field.name, got, field.want)
		}
	}
	if !reflect.DeepEqual(parsed[0].Repos, original[0].Repos) {
		t.Errorf("round-trip Repos = %v, want %v", parsed[0].Repos, original[0].Repos)
	}
	if parsed[0].Scan != original[0].Scan {
		t.Errorf("round-trip Scan = %q, want %q", parsed[0].Scan, original[0].Scan)
	}
}

// TestWriteEmitsInlineHeaderNoSeparateStatusPhase is the FIX 1 round-trip
// proof: Write emits the inline header `## <name>   status: X   phase: Y` and
// does NOT emit separate `status:` / `phase:` lines (which would duplicate the
// inline header). Parsing the output back recovers all three fields.
func TestWriteEmitsInlineHeaderNoSeparateStatusPhase(t *testing.T) {
	projects := []Project{
		{Name: "Workflow engine", Status: "active", Phase: "production", Repos: []string{"GoCodeAlone/workflow"}},
	}
	var buf bytes.Buffer
	if err := Write(&buf, projects, nil); err != nil {
		t.Fatal(err)
	}
	body := buf.String()

	// Inline header present (the ONE identity line).
	wantHeader := "## Workflow engine   status: active   phase: production"
	if !strings.Contains(body, wantHeader) {
		t.Errorf("missing inline header %q:\n%s", wantHeader, body)
	}

	// NO standalone status:/phase: bullet lines (the redundant lines FIX 1 kills).
	// Scan line-by-line so a status value containing "status:" doesn't false-match.
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		// The inline header legitimately contains "status:"/"phase:" — skip it.
		if strings.HasPrefix(trimmed, "## ") {
			continue
		}
		if strings.HasPrefix(trimmed, "status:") {
			t.Errorf("FIX 1 VIOLATION: standalone status line emitted (should be inline only): %q", line)
		}
		if strings.HasPrefix(trimmed, "phase:") {
			t.Errorf("FIX 1 VIOLATION: standalone phase line emitted (should be inline only): %q", line)
		}
	}

	// Round-trip: parse recovers Name/Status/Phase from the inline header.
	path := writeFixture(t, "PROJECTS.md", body)
	parsed, err := ParseProjects(path)
	if err != nil {
		t.Fatalf("ParseProjects: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 project, got %d", len(parsed))
	}
	if parsed[0].Name != "Workflow engine" || parsed[0].Status != "active" || parsed[0].Phase != "production" {
		t.Errorf("round-trip inline header = %+v, want Name=Workflow engine Status=active Phase=production", parsed[0])
	}
}

// TestWriteIncludesHeaderAndUnmapped verifies Write emits the file header
// (title + block-shape legend) and the ## Unmapped section listing repos not
// in any project.
func TestWriteIncludesHeaderAndUnmapped(t *testing.T) {
	projects := []Project{{Name: "p", Status: "active", Repos: []string{"GoCodeAlone/a"}}}
	unmapped := []string{"GoCodeAlone/orphan1", "GoCodeAlone/orphan2"}

	var buf bytes.Buffer
	if err := Write(&buf, projects, unmapped); err != nil {
		t.Fatal(err)
	}
	body := buf.String()

	// Header present.
	if !strings.HasPrefix(body, "# Projects") {
		t.Errorf("missing # Projects title header:\n%s", body)
	}
	// Block-shape legend present (so humans know the format).
	if !strings.Contains(body, "repos") || !strings.Contains(body, "scan") {
		t.Errorf("missing block-shape legend:\n%s", body)
	}
	// Unmapped section with both repos.
	if !strings.Contains(body, "## Unmapped") {
		t.Errorf("missing ## Unmapped section:\n%s", body)
	}
	for _, r := range unmapped {
		if !strings.Contains(body, r) {
			t.Errorf("Unmapped section missing repo %q:\n%s", r, body)
		}
	}
}

// TestWriteEmptyUnmappedOmitsSection: when there are no unmapped repos, the
// Unmapped section is omitted (not an empty header).
func TestWriteEmptyUnmappedOmitsSection(t *testing.T) {
	projects := []Project{{Name: "p", Repos: []string{"GoCodeAlone/a"}}}
	var buf bytes.Buffer
	if err := Write(&buf, projects, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "## Unmapped") {
		t.Errorf("Unmapped section should be omitted when empty:\n%s", buf.String())
	}
}
