package projects

import (
	"reflect"
	"testing"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-portfolio/internal/catalog"
)

// TestRollUpAggregates is the TASK 3 headline proof: a fixture catalog (mixed
// remote formats) + projects (incl. a missing-member project + an all-fail
// project) produces correct roll-up signals.
//
// Catalog remotes are intentionally in mixed formats (git@, https) to prove
// normalization via catalog.NormalizeRepo is applied to BOTH sides before
// matching. PROJECTS.md repos are canonical full-names.
func TestRollUpAggregates(t *testing.T) {
	// Catalog: 5 repos. auth-a + auth-b are members of auth-stack;
	// dns-infra is a member of dns-catalog; hover is a member of dns-catalog
	// too. The 5th (orphan) is not in any project.
	catalogRepos := []catalog.Project{
		{
			Repo:     "GoCodeAlone/workflow-plugin-auth",
			Category: "plugin",
			Scan: catalog.ScanFacts{
				Remote:       "git@github.com:GoCodeAlone/workflow-plugin-auth.git",
				LastCommitISO: "2026-06-02T10:00:00Z", // recent -> active
			},
			Release: catalog.ReleaseFacts{OpenPRs: 2, OpenIssues: 3},
		},
		{
			Repo:     "GoCodeAlone/auth",
			Category: "app",
			Scan: catalog.ScanFacts{
				Remote:       "https://github.com/GoCodeAlone/auth.git",
				LastCommitISO: "2026-06-01T08:00:00Z", // recent -> active
			},
			Release: catalog.ReleaseFacts{OpenPRs: 1, OpenIssues: 2},
		},
		{
			Repo:     "GoCodeAlone/workflow-plugin-infra",
			Category: "plugin",
			Scan: catalog.ScanFacts{
				Remote:       "ssh://git@github.com/GoCodeAlone/workflow-plugin-infra",
				LastCommitISO: "2026-06-05T12:00:00Z", // most recent -> max
			},
			Release: catalog.ReleaseFacts{OpenPRs: 1, OpenIssues: 0},
		},
		{
			Repo:     "GoCodeAlone/hover",
			Category: "plugin",
			Scan: catalog.ScanFacts{
				Remote:       "git@github.com:GoCodeAlone/hover.git",
				LastCommitISO: "2025-01-01T00:00:00Z", // >30d old -> NOT active
			},
			Release: catalog.ReleaseFacts{OpenPRs: 0, OpenIssues: 0},
		},
		{
			Repo:     "GoCodeAlone/orphan-repo",
			Category: "other",
			Scan: catalog.ScanFacts{
				Remote:       "git@github.com:GoCodeAlone/orphan-repo.git",
				LastCommitISO: "2026-06-21T00:00:00Z",
			},
		},
	}

	projects := []Project{
		{
			Name:  "auth-stack",
			Repos: []string{"GoCodeAlone/workflow-plugin-auth", "GoCodeAlone/auth"},
		},
		{
			Name:  "dns-catalog",
			Repos: []string{"GoCodeAlone/workflow-plugin-infra", "GoCodeAlone/hover", "GoCodeAlone/gocodealone-dns"}, // dns missing from catalog (P-V6)
		},
		{
			Name:  "ghost-project",
			Repos: []string{"GoCodeAlone/no-such-repo-1", "GoCodeAlone/no-such-repo-2"}, // ALL fail (P-V3)
		},
	}

	// Reference "now" for the 30d active window: fixture dates are mid-2026,
	// so anchor now at 2026-06-21 to make 2025-01-01 clearly stale and
	// 2026-06-* clearly active.
	now := mustTime(t, "2026-06-21T00:00:00Z")

	got := RollUp(projects, catalogRepos, now)

	authScan, ok := got["auth-stack"]
	if !ok {
		t.Fatalf("missing auth-stack scan")
	}
	if authScan.LastActivity != "2026-06-02" {
		t.Errorf("auth-stack LastActivity = %q, want 2026-06-02 (max of 06-02, 06-01)", authScan.LastActivity)
	}
	if authScan.ActiveRepos != 2 {
		t.Errorf("auth-stack ActiveRepos = %d, want 2", authScan.ActiveRepos)
	}
	if authScan.TotalRepos != 2 {
		t.Errorf("auth-stack TotalRepos = %d, want 2", authScan.TotalRepos)
	}
	if authScan.OpenPRs != 3 {
		t.Errorf("auth-stack OpenPRs = %d, want 3 (2+1)", authScan.OpenPRs)
	}
	if authScan.OpenIssues != 5 {
		t.Errorf("auth-stack OpenIssues = %d, want 5 (3+2)", authScan.OpenIssues)
	}
	if authScan.AllFailed {
		t.Errorf("auth-stack AllFailed = true, want false")
	}
	if len(authScan.Missing) != 0 {
		t.Errorf("auth-stack Missing = %v, want empty", authScan.Missing)
	}

	dnsScan, ok := got["dns-catalog"]
	if !ok {
		t.Fatalf("missing dns-catalog scan")
	}
	// Max of (06-05, 2025-01-01, missing) = 06-05.
	if dnsScan.LastActivity != "2026-06-05" {
		t.Errorf("dns-catalog LastActivity = %q, want 2026-06-05", dnsScan.LastActivity)
	}
	// Only infra (06-05) + hover (2025-01-01) matched; dns is missing.
	if dnsScan.ActiveRepos != 1 {
		t.Errorf("dns-catalog ActiveRepos = %d, want 1 (only infra is recent; hover stale; dns missing)", dnsScan.ActiveRepos)
	}
	if dnsScan.TotalRepos != 3 {
		t.Errorf("dns-catalog TotalRepos = %d, want 3", dnsScan.TotalRepos)
	}
	if dnsScan.OpenPRs != 1 {
		t.Errorf("dns-catalog OpenPRs = %d, want 1", dnsScan.OpenPRs)
	}
	if dnsScan.OpenIssues != 0 {
		t.Errorf("dns-catalog OpenIssues = %d, want 0", dnsScan.OpenIssues)
	}
	// P-V6: the missing member is collected, not silently dropped.
	if !reflect.DeepEqual(dnsScan.Missing, []string{"GoCodeAlone/gocodealone-dns"}) {
		t.Errorf("dns-catalog Missing = %v, want [GoCodeAlone/gocodealone-dns]", dnsScan.Missing)
	}
	if dnsScan.AllFailed {
		t.Errorf("dns-catalog AllFailed = true, want false (2/3 matched)")
	}

	// P-V3: ghost-project — ALL member lookups fail -> zero Scan + AllFailed.
	ghostScan, ok := got["ghost-project"]
	if !ok {
		t.Fatalf("missing ghost-project scan")
	}
	if !ghostScan.AllFailed {
		t.Errorf("ghost-project AllFailed = false, want true (all lookups failed)")
	}
	if ghostScan.LastActivity != "" {
		t.Errorf("ghost-project LastActivity = %q, want empty (all failed)", ghostScan.LastActivity)
	}
	if ghostScan.TotalRepos != 2 {
		t.Errorf("ghost-project TotalRepos = %d, want 2", ghostScan.TotalRepos)
	}
	if len(ghostScan.Missing) != 2 {
		t.Errorf("ghost-project Missing = %v, want both missing repos", ghostScan.Missing)
	}
}

// TestRollUpMissingReposCollectedGlobally confirms the aggregate MissingRepos
// list (for the caller's stderr warning) is the union across all projects.
func TestRollUpMissingReposCollectedGlobally(t *testing.T) {
	catalogRepos := []catalog.Project{
		{Repo: "GoCodeAlone/a", Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/a.git"}},
	}
	projects := []Project{
		{Name: "p1", Repos: []string{"GoCodeAlone/a", "GoCodeAlone/missing-1"}},
		{Name: "p2", Repos: []string{"GoCodeAlone/missing-2"}},
	}
	now := mustTime(t, "2026-06-21T00:00:00Z")
	got := RollUp(projects, catalogRepos, now)
	if !reflect.DeepEqual(got["p1"].Missing, []string{"GoCodeAlone/missing-1"}) {
		t.Errorf("p1 Missing = %v", got["p1"].Missing)
	}
	if !reflect.DeepEqual(got["p2"].Missing, []string{"GoCodeAlone/missing-2"}) {
		t.Errorf("p2 Missing = %v", got["p2"].Missing)
	}
}

// TestRollUpEmptyProjects confirms an empty projects slice yields an empty
// (non-nil) scan map.
func TestRollUpEmptyProjects(t *testing.T) {
	got := RollUp(nil, []catalog.Project{{Repo: "GoCodeAlone/a"}}, mustTime(t, "2026-06-21T00:00:00Z"))
	if len(got) != 0 {
		t.Errorf("expected empty scan map, got %d entries", len(got))
	}
}

// TestRollUpGlobPrecedence is the FIX 2 headline proof: a project with a glob
// `GoCodeAlone/workflow-plugin-*` rolls up ALL matching catalog plugins, EXCEPT
// repos explicitly claimed by another project's exact `repos:` entry (explicit
// over glob). A non-plugin repo (workflow) is NOT matched by the glob. No
// double-counting: a repo explicitly in one project is absent from the glob
// project's roll-up.
func TestRollUpGlobPrecedence(t *testing.T) {
	// Catalog: 4 plugins + the engine + an unrelated repo.
	catalogRepos := []catalog.Project{
		{Repo: "GoCodeAlone/workflow-plugin-auth", Category: "plugin",
			Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/workflow-plugin-auth.git", LastCommitISO: "2026-06-20T00:00:00Z"},
			Release: catalog.ReleaseFacts{OpenPRs: 1, OpenIssues: 1}},
		{Repo: "GoCodeAlone/workflow-plugin-compute", Category: "plugin",
			Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/workflow-plugin-compute.git", LastCommitISO: "2026-06-19T00:00:00Z"},
			Release: catalog.ReleaseFacts{OpenPRs: 2, OpenIssues: 2}},
		{Repo: "GoCodeAlone/workflow-plugin-infra", Category: "plugin",
			Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/workflow-plugin-infra.git", LastCommitISO: "2026-06-18T00:00:00Z"},
			Release: catalog.ReleaseFacts{OpenPRs: 3, OpenIssues: 3}},
		{Repo: "GoCodeAlone/workflow-plugin-portfolio", Category: "plugin",
			Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/workflow-plugin-portfolio.git", LastCommitISO: "2026-06-17T00:00:00Z"},
			Release: catalog.ReleaseFacts{OpenPRs: 4, OpenIssues: 4}},
		{Repo: "GoCodeAlone/workflow", Category: "engine",
			Scan: catalog.ScanFacts{Remote: "https://github.com/GoCodeAlone/workflow.git", LastCommitISO: "2026-06-16T00:00:00Z"},
			Release: catalog.ReleaseFacts{OpenPRs: 9, OpenIssues: 9}},
		{Repo: "GoCodeAlone/orphan-repo", Category: "other",
			Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/orphan-repo.git", LastCommitISO: "2026-06-15T00:00:00Z"}},
	}

	projects := []Project{
		{
			Name:  "Workflow engine",
			Repos: []string{"GoCodeAlone/workflow", "GoCodeAlone/workflow-plugin-*"}, // glob
		},
		{
			Name:  "Workflow-Compute",
			Repos: []string{"GoCodeAlone/workflow-plugin-compute"}, // exact claim
		},
	}

	// Resolve globs against the catalog (explicit-over-glob precedence).
	resolved := ExpandGlobs(projects, catalogRepos)

	// Engine: workflow (exact) + auth, infra, portfolio (glob matches NOT
	// claimed by compute). compute-* is EXCLUDED (explicitly in Compute).
	engineRepos := reposByName(resolved, "Workflow engine")
	if !containsAll(engineRepos, "GoCodeAlone/workflow", "GoCodeAlone/workflow-plugin-auth",
		"GoCodeAlone/workflow-plugin-infra", "GoCodeAlone/workflow-plugin-portfolio") {
		t.Errorf("engine resolved repos = %v; want workflow + auth/infra/portfolio (glob minus compute)", engineRepos)
	}
	if containsAny(engineRepos, "GoCodeAlone/workflow-plugin-compute") {
		t.Errorf("engine must NOT include workflow-plugin-compute (explicitly claimed by Compute): %v", engineRepos)
	}

	// Compute: only its explicit repo.
	computeRepos := reposByName(resolved, "Workflow-Compute")
	if len(computeRepos) != 1 || computeRepos[0] != "GoCodeAlone/workflow-plugin-compute" {
		t.Errorf("compute resolved repos = %v; want only [workflow-plugin-compute]", computeRepos)
	}

	// Roll up the RESOLVED membership and verify no double-counting.
	now := mustTime(t, "2026-06-21T00:00:00Z")
	scans := RollUp(resolved, catalogRepos, now)

	engineScan := scans["Workflow engine"]
	// 4 members (workflow + auth + infra + portfolio); open-PRs 9+1+3+4=17,
	// open-issues 9+1+3+4=17.
	if engineScan.TotalRepos != 4 {
		t.Errorf("engine TotalRepos = %d, want 4 (workflow + 3 glob plugins, compute excluded)", engineScan.TotalRepos)
	}
	if engineScan.OpenPRs != 17 {
		t.Errorf("engine OpenPRs = %d, want 17 (9+1+3+4)", engineScan.OpenPRs)
	}
	if engineScan.OpenIssues != 17 {
		t.Errorf("engine OpenIssues = %d, want 17 (9+1+3+4)", engineScan.OpenIssues)
	}
	// LastActivity = max across members = 2026-06-20 (auth).
	if engineScan.LastActivity != "2026-06-20" {
		t.Errorf("engine LastActivity = %q, want 2026-06-20", engineScan.LastActivity)
	}
	if len(engineScan.Missing) != 0 {
		t.Errorf("engine Missing = %v, want empty (all glob matches resolved)", engineScan.Missing)
	}

	computeScan := scans["Workflow-Compute"]
	if computeScan.TotalRepos != 1 {
		t.Errorf("compute TotalRepos = %d, want 1", computeScan.TotalRepos)
	}
	if computeScan.OpenPRs != 2 {
		t.Errorf("compute OpenPRs = %d, want 2 (NOT double-counted in engine)", computeScan.OpenPRs)
	}

	// Sanity: orphan-repo is NOT claimed by any glob.
	for _, r := range engineRepos {
		if r == "GoCodeAlone/orphan-repo" {
			t.Errorf("orphan-repo matched by workflow-plugin-* glob: %v", engineRepos)
		}
	}
}

// TestRollUpGlobNoMatch: a glob that matches nothing resolves to an empty
// membership (TotalRepos 0), and does NOT error. The curated glob is
// preserved in the original Repos (separate from resolved).
func TestRollUpGlobNoMatch(t *testing.T) {
	catalogRepos := []catalog.Project{
		{Repo: "GoCodeAlone/workflow", Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/workflow.git"}},
	}
	projects := []Project{
		{Name: "p", Repos: []string{"GoCodeAlone/nope-*"}},
	}
	resolved := ExpandGlobs(projects, catalogRepos)
	if got := reposByName(resolved, "p"); len(got) != 0 {
		t.Errorf("glob with no matches resolved to %v, want empty", got)
	}
	// Curated Repos unchanged (glob preserved verbatim for Write).
	if len(projects[0].Repos) != 1 || projects[0].Repos[0] != "GoCodeAlone/nope-*" {
		t.Errorf("curated Repos mutated by ExpandGlobs: %v (must be preserved)", projects[0].Repos)
	}
}

// TestRollUpExplicitOverGlobMultipleProjects: a repo explicitly claimed by TWO
// projects cannot exist (a repo is in at most one explicit list), but a glob
// must still skip repos explicitly claimed by ANY project. Verify with two
// exact-claim projects + one glob project.
func TestRollUpExplicitOverGlobMultipleProjects(t *testing.T) {
	catalogRepos := []catalog.Project{
		{Repo: "GoCodeAlone/workflow-plugin-a", Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/workflow-plugin-a.git", LastCommitISO: "2026-06-01T00:00:00Z"}},
		{Repo: "GoCodeAlone/workflow-plugin-b", Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/workflow-plugin-b.git", LastCommitISO: "2026-06-02T00:00:00Z"}},
		{Repo: "GoCodeAlone/workflow-plugin-c", Scan: catalog.ScanFacts{Remote: "git@github.com:GoCodeAlone/workflow-plugin-c.git", LastCommitISO: "2026-06-03T00:00:00Z"}},
	}
	projects := []Project{
		{Name: "A", Repos: []string{"GoCodeAlone/workflow-plugin-a"}},      // exact
		{Name: "B", Repos: []string{"GoCodeAlone/workflow-plugin-b"}},      // exact
		{Name: "C", Repos: []string{"GoCodeAlone/workflow-plugin-*"}},      // glob -> only c remains
	}
	resolved := ExpandGlobs(projects, catalogRepos)
	if got := reposByName(resolved, "C"); len(got) != 1 || got[0] != "GoCodeAlone/workflow-plugin-c" {
		t.Errorf("C resolved = %v; want only [workflow-plugin-c] (a,b explicitly claimed)", got)
	}
}

// reposByName returns the resolved repos for the named project (test helper).
func reposByName(projects []Project, name string) []string {
	for _, p := range projects {
		if p.Name == name {
			return p.Repos
		}
	}
	return nil
}

func containsAll(haystack []string, needles ...string) bool {
	for _, n := range needles {
		found := false
		for _, h := range haystack {
			if h == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsAny(haystack []string, needles ...string) bool {
	for _, n := range needles {
		for _, h := range haystack {
			if h == n {
				return true
			}
		}
	}
	return false
}

// mustTime parses an RFC3339 time, failing the test on error.
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tv, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tv
}
