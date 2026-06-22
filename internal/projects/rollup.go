package projects

import (
	"strings"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-portfolio/internal/catalog"
)

// activeWindow is how recent a last-commit must be (relative to now) for its
// repo to count as "active" in the roll-up.
const activeWindow = 30 * 24 * time.Hour

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
