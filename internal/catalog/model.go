// Package catalog models the cross-repo portfolio catalog: a curated,
// losslessly-mergeable list of Project entries plus a tooling inventory
// section derived from capability/inventory.
//
// The central invariant (V1) is that the human-curated Status field is
// PRESERVED byte-identical across re-scans: the scanner rewrites
// machine-derived facts (Scan/Release/DocsPaths/Provides) but NEVER touches
// Status, so a 2nd-run diff on Status is always empty. This keeps the
// catalog regeneratable without losing human curation.
package catalog

// Project is one portfolio entry. Fields are split into:
//   - identity: Repo (match key), Visibility, Category
//   - machine-derived (rewritten each scan): Scan, Release, DocsPaths,
//     Provides
//   - human-curated (NEVER rewritten by scan): Status
//   - FollowUpCount: derived from the follow-ups extract
type Project struct {
	// Repo is the fully-qualified repo name ("owner/name") — the MATCH KEY
	// for Merge. Survives re-scans.
	Repo string

	// Visibility is the gh-confirmed visibility ("PRIVATE"|"PUBLIC"|"?" when
	// unknown). Machine-derived.
	Visibility string

	// Category classifies the entry for the status overview
	// ("plugin"|"engine"|"registry"|"app"|"docs"|...). Machine-derived.
	Category string

	// Scan holds per-repo scanner facts (git + gh). Rewritten each scan.
	Scan ScanFacts

	// Release holds release/PR/issue facts from gh. Rewritten each scan.
	Release ReleaseFacts

	// DocsPaths holds discovered docs paths (retros, ADRs). Rewritten each
	// scan.
	DocsPaths DocsPaths

	// Provides is the registry/manifest-declared capability summary
	// (e.g. "module: auth.credential, step: step.authz_check").
	// Machine-derived.
	Provides string

	// Status is the HUMAN-CURATED narrative for this entry ("active —
	// v0.3.0 shipped; Phase II in-flight"). NEVER rewritten by Merge (V1):
	// the scanner cannot supply Status, so Merge copies it verbatim from
	// the existing entry. Empty for new repos (stub awaiting curation).
	Status string

	// FollowUpCount is the number of open follow-ups extracted from retros
	// for this repo. Machine-derived.
	FollowUpCount int
}

// ScanFacts holds git + scanner-derived facts for one repo.
type ScanFacts struct {
	// LastCommitISO is the committer-date ISO-8601 of HEAD
	// (`git log -1 --format=%cI`). RFC3339-shaped. Empty when unknown.
	LastCommitISO string

	// Uncommitted is true if `git status --porcelain` is non-empty.
	Uncommitted bool

	// Remote is the origin remote URL. Empty when no origin.
	Remote string

	// Path is the absolute on-disk path to the repo root. Empty for
	// repos that could not be located on disk.
	Path string
}

// ReleaseFacts holds gh-derived release/PR/issue facts. Unknown flags
// disambiguate "gh could not answer this field" from real zero counts or a
// repo with no stable releases.
type ReleaseFacts struct {
	// LatestRelease is the latest release tag (e.g. "v0.3.0"). Empty when
	// the repo has no stable releases or when LatestReleaseUnknown is true.
	LatestRelease        string
	LatestReleaseUnknown bool

	// OpenPRs is the count of open PRs. Zero can mean known zero when
	// OpenPRsUnknown is false.
	OpenPRs        int
	OpenPRsUnknown bool

	// OpenIssues is the count of open issues. Zero can mean known zero when
	// OpenIssuesUnknown is false.
	OpenIssues        int
	OpenIssuesUnknown bool

	// ReleaseGHAbsent is true when gh facts could not be collected (gh
	// missing/offline/rate-limited). Deprecated: retained for existing tests
	// and callers; new scanner paths should set per-field unknown flags.
	ReleaseGHAbsent bool
}

// DocsPaths holds discovered docs sub-paths for one repo.
type DocsPaths struct {
	// RetrosDir is "docs/retros" if present, else empty.
	RetrosDir string

	// ADRsDir is "docs/decisions" or "decisions" if present, else empty.
	ADRsDir string
}
