package projects

import "fmt"

// Merge combines existing (human-curated) projects with freshly rolled-up
// Scans, producing the new project list for PROJECTS.md.
//
// P-V1 — LOSSLESS curated-field preservation (the headline invariant):
//   - Match by project Name.
//   - For matched projects: rewrite ONLY the Scan field from `scans`; copy
//     every curated field (Status, Phase, Repos, Goal, Blockers, Next, Design)
//     VERBATIM from `existing`. The scan row is the ONLY generator-written
//     field; everything else is human-authored.
//   - Projects only in `existing` with no scan entry: keep their existing
//     Scan verbatim (Merge does not invent scan data; the next scan will
//     populate it).
//
// The scan row format (when a scan exists):
//
//	last-activity <date>, active <n>/<total> repos, open-PRs <n>, open-issues <n>
//
// When the scan's AllFailed flag is true (P-V3: all member lookups failed),
// the row is rendered as "?" rather than misleading zeros.
//
// Output order is deterministic: existing projects in existing order (so a
// human's curation order is stable), then any scan-keyed projects not in
// existing appended in sorted-name order (defensive — scans are normally a
// subset of existing).
func Merge(existing []Project, scans map[string]Scan) []Project {
	out := make([]Project, 0, len(existing))
	seen := make(map[string]bool, len(existing)+len(scans))

	for i := range existing {
		ex := existing[i]
		seen[ex.Name] = true
		if sc, ok := scans[ex.Name]; ok {
			ex.Scan = formatScanRow(sc)
		}
		out = append(out, ex)
	}

	// Defensive: append scan-keyed projects not in existing (as stubs).
	for name, sc := range scans {
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, Project{Name: name, Scan: formatScanRow(sc)})
	}
	return out
}

// formatScanRow renders a Scan into the canonical scan row. AllFailed -> "?".
func formatScanRow(sc Scan) string {
	if sc.AllFailed {
		return "?"
	}
	return fmt.Sprintf(
		"last-activity %s, active %d/%d repos, open-PRs %d, open-issues %d",
		sc.LastActivity, sc.ActiveRepos, sc.TotalRepos, sc.OpenPRs, sc.OpenIssues,
	)
}
