package projects

import (
	"sort"
	"strings"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-portfolio/internal/catalog"
)

// activeWindow is how recent a last-commit must be (relative to now) for its
// repo to count as "active" in the roll-up.
const activeWindow = 30 * 24 * time.Hour

// globSuffix marks a repos: entry as a prefix glob (match all catalog repos
// whose normalized full-name starts with the entry minus the suffix).
const globSuffix = "*"

// isGlob reports whether a repos: entry is a glob (ends with "*").
func isGlob(entry string) bool {
	return strings.HasSuffix(entry, globSuffix)
}

// ExpandGlobs resolves glob repos: entries against the catalog, returning a
// COPY of projects whose Repos holds the EFFECTIVE membership (exact entries
// plus glob matches) suitable for RollUp and the Unmapped computation.
//
// Precedence rule (critical): a repo explicitly listed in ANY project's
// repos: (exact match) is claimed by that project; it is NOT also matched by
// another project's glob. Implementation:
//  1. Collect the set of repos explicitly claimed across all projects.
//  2. For each project, its resolved membership = its exact entries PLUS glob
//     matches (catalog repos whose normalized full-name starts with the glob
//     prefix) that are NOT in the global explicit-claimed set.
//
// The ORIGINAL projects slice is untouched: the curated Repos (which may
// contain globs) is preserved verbatim for Write/round-trip (P-V1). Only the
// returned copy carries expanded membership.
//
// A glob entry with no catalog matches contributes nothing (the project's
// resolved membership simply omits it). Non-glob (exact) entries are kept as-
// is even when they do not match the catalog (RollUp reports them via
// Scan.Missing, P-V6).
func ExpandGlobs(projects []Project, catalogRepos []catalog.Project) []Project {
	// Index catalog rows by normalized repo key for O(1) lookup + iteration.
	catalogKeys := make([]string, 0, len(catalogRepos))
	for _, c := range catalogRepos {
		key := catalog.NormalizeRepo(c.Scan.Remote)
		if key == "" || !strings.Contains(key, "/") {
			key = c.Repo
		}
		catalogKeys = append(catalogKeys, key)
	}

	// 1. Global set of explicitly-claimed repos (exact, non-glob entries only).
	explicitClaimed := make(map[string]struct{})
	for _, p := range projects {
		for _, r := range p.Repos {
			if !isGlob(r) {
				explicitClaimed[r] = struct{}{}
			}
		}
	}

	// 2. Resolve each project's effective membership.
	out := make([]Project, len(projects))
	for i, p := range projects {
		resolved := make([]string, 0, len(p.Repos))
		seen := make(map[string]struct{}, len(p.Repos))
		add := func(r string) {
			if _, ok := seen[r]; ok {
				return
			}
			seen[r] = struct{}{}
			resolved = append(resolved, r)
		}
		for _, r := range p.Repos {
			if !isGlob(r) {
				add(r) // exact entry kept as-is (Missing reported downstream)
				continue
			}
			prefix := strings.TrimSuffix(r, globSuffix)
			// Match catalog repos (normalized) that start with the prefix and
			// are NOT explicitly claimed by any project (explicit over glob).
			matches := make([]string, 0)
			for _, key := range catalogKeys {
				if !strings.HasPrefix(key, prefix) {
					continue
				}
				if _, claimed := explicitClaimed[key]; claimed {
					continue
				}
				matches = append(matches, key)
			}
			sort.Strings(matches) // deterministic membership / output order
			for _, m := range matches {
				add(m)
			}
		}
		// Copy the project; replace only Repos (curated fields preserved).
		copy := p
		copy.Repos = resolved
		out[i] = copy
	}
	return out
}

// RollUp aggregates per-repo catalog signals into per-project Scans.
//
// Each project's repos (canonical full-names) are matched to catalog rows by
// normalized remote full-name (catalog.NormalizeRepo). For matched members:
//   - LastActivity  = max(last-commit) across members, as YYYY-MM-DD
//   - OpenPRs       = sum across members
//   - OpenIssues    = sum across members
//   - ActiveRepos   = count of members with last-commit within 30d of now
//   - TotalRepos    = len(project.Repos)
//
// P-V6: a project repo not found in the catalog is collected into Scan.Missing
// (the caller warns; it is not silently dropped).
//
// P-V3: if ALL member lookups fail, the Scan is zero-valued with AllFailed=true
// so the caller renders "?" rather than misleading zeros.
//
// The result is keyed by project Name.
func RollUp(projects []Project, catalogRepos []catalog.Project, now time.Time) map[string]Scan {
	// Index catalog rows by normalized repo key for O(1) lookup. Normalize the
	// Remote (the canonical key); fall back to the Repo field if Remote is empty
	// (already-canonical in tests / degraded scans).
	byKey := make(map[string]catalog.Project, len(catalogRepos))
	for _, c := range catalogRepos {
		key := catalog.NormalizeRepo(c.Scan.Remote)
		if key == "" || !strings.Contains(key, "/") {
			// Remote empty or unnormalizable — try the Repo field directly.
			key = c.Repo
		}
		byKey[key] = c
	}

	out := make(map[string]Scan, len(projects))
	for _, p := range projects {
		out[p.Name] = rollUpProject(p, byKey, now)
	}
	return out
}

// rollUpProject aggregates one project's member signals.
func rollUpProject(p Project, byKey map[string]catalog.Project, now time.Time) Scan {
	scan := Scan{
		TotalRepos: len(p.Repos),
	}
	var (
		latest   time.Time
		matched  int
		hasCommit bool
	)
	for _, repoKey := range p.Repos {
		c, ok := byKey[repoKey]
		if !ok {
			// P-V6: collect missing; don't silently drop.
			scan.Missing = append(scan.Missing, repoKey)
			continue
		}
		matched++
		scan.OpenPRs += c.Release.OpenPRs
		scan.OpenIssues += c.Release.OpenIssues

		if c.Scan.LastCommitISO != "" {
			if commitTime, err := time.Parse(time.RFC3339, c.Scan.LastCommitISO); err == nil {
				hasCommit = true
				if commitTime.After(latest) {
					latest = commitTime
				}
				if now.Sub(commitTime) <= activeWindow {
					scan.ActiveRepos++
				}
			}
		}
	}

	if hasCommit {
		scan.LastActivity = latest.Format("2006-01-02")
	}

	// P-V3: if ALL member lookups failed, flag AllFailed so the caller emits "?".
	if matched == 0 && len(p.Repos) > 0 {
		scan.AllFailed = true
	}
	return scan
}
