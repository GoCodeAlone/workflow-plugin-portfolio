// Package projects models the docs/PROJECTS.md file: a human-curated mapping
// of logical projects to sets of repos, with generator-written scan roll-ups.
//
// The central invariant (P-V1) is that human-curated fields (status, phase,
// repos, goal, blockers, next, design) are PRESERVED byte-identical across
// re-scans: the generator rewrites ONLY the `- scan:` row from the per-project
// roll-up, leaving everything else untouched. This keeps PROJECTS.md
// regeneratable without losing human curation.
//
// PROJECTS.md is OPT-IN: if docs/PROJECTS.md does not exist, scan does not
// create it. Only when a human authors the seed file does the generator
// populate the scan rows.
package projects

// Project is one logical project entry in PROJECTS.md.
//
// Fields are split into:
//   - identity: Name (heading), Status, Phase
//   - membership: Repos (the repo full-names that belong to this project)
//   - human-curated (NEVER rewritten by scan): Goal, Blockers, Next, Design
//   - generator-written (rewritten each scan): Scan
type Project struct {
	// Name is the project heading (the text after `## `). Match key for
	// Merge. Survives re-scans.
	Name string

	// Status is the project-level status line (e.g. "active", "in-flight").
	// Human-curated.
	Status string

	// Phase is the project phase number (e.g. "2"). Human-curated.
	Phase string

	// Repos is the list of repo full-names ("owner/name") belonging to this
	// project, parsed from the comma-separated `- repos:` row. These are the
	// canonical repo keys matched against the catalog by normalized remote.
	// Human-curated.
	Repos []string

	// Goal is the one-line project goal. Human-curated.
	Goal string

	// Blockers is the current blockers note. Human-curated.
	Blockers string

	// Next is the next-step note. Human-curated.
	Next string

	// Design is the path to the design doc. Human-curated.
	Design string

	// Scan is the generator-written roll-up row
	// ("last-activity <date>, active <n>/<total> repos, open-PRs <n>,
	// open-issues <n>"). Rewritten each scan; the parsed value is preserved
	// through Merge so the merge can detect an unchanged scan (no-op).
	Scan string
}

// Scan is the rolled-up per-project signal set written into the `- scan:` row.
//
// LastActivity is the max(last-commit) across member repos (ISO date, YYYY-MM-DD).
// ActiveRepos is the count of members with last-commit within 30 days.
// TotalRepos is len(Project.Repos).
// OpenPRs / OpenIssues are the sums across members.
//
// P-V3: when ALL member lookups fail (no repos matched the catalog),
// AllFailed is true and the caller renders "?" for every field rather than
// misleading zeros.
type Scan struct {
	LastActivity string
	ActiveRepos  int
	TotalRepos   int
	OpenPRs      int
	OpenIssues   int

	// AllFailed is true when every member repo failed to match the catalog
	// (P-V3). The caller emits "?" for the scan row instead of zero values.
	AllFailed bool

	// Missing lists project repos that did not match any catalog row (P-V6).
	// The caller warns on stderr; the roll-up does not silently drop them.
	Missing []string
}
