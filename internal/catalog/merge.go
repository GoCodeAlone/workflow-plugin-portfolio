package catalog

// Merge combines an existing (human-curated) project list with a freshly
// scanned list, producing the new canonical catalog.
//
// V1 — LOSSLESS Status preservation (the headline invariant):
//   - Match by Repo (fully-qualified owner/name).
//   - For matched repos: rewrite machine-derived fields (Scan, Release,
//     DocsPaths, Provides, Visibility, Category) from `scanned`, but COPY
//     Status VERBATIM from `existing` (scanned never supplies Status).
//   - Repos only in `scanned`: append as a stub with empty Status (new repo,
//     awaiting human curation).
//   - Repos only in `existing`: KEEP them (flagged stale — not deleted,
//     because deleting would lose the curated Status; the human may have
//     just not scanned it this run, or the repo may be genuinely abandoned
//     which the Status already records).
//
// The output order is deterministic: existing repos first (in existing
// order, so a human's curation order is stable), then new repos in scanned
// order.
func Merge(existing, scanned []Project) []Project {
	// Index scanned by Repo for O(1) lookup.
	scannedByRepo := make(map[string]int, len(scanned))
	for i := range scanned {
		scannedByRepo[scanned[i].Repo] = i
	}

	// Index existing by Repo to detect stale entries.
	existingByRepo := make(map[string]int, len(existing))
	for i := range existing {
		existingByRepo[existing[i].Repo] = i
	}

	out := make([]Project, 0, len(existing)+len(scanned))

	// Pass 1: walk existing in order; for each, merge with scanned if present,
	// else keep as-is (stale). Status is ALWAYS copied verbatim from existing
	// (V1) — scanned never supplies it.
	seen := make(map[string]bool, len(existing)+len(scanned))
	for i := range existing {
		ex := existing[i]
		seen[ex.Repo] = true
		if j, ok := scannedByRepo[ex.Repo]; ok {
			sc := scanned[j]
			merged := sc // take machine-derived fields from scanned
			merged.Status = ex.Status // V1: preserve verbatim from existing
			merged.FollowUpCount = ex.FollowUpCount // preserved (derived separately, not by scan)
			out = append(out, merged)
		} else {
			// Stale: keep existing verbatim (Status preserved, machine fields
			// left as they were — they'll be stale but not lost).
			out = append(out, ex)
		}
	}

	// Pass 2: append scanned repos NOT in existing (new repos), as stubs
	// with empty Status, in scanned order.
	for i := range scanned {
		sc := scanned[i]
		if seen[sc.Repo] {
			continue
		}
		seen[sc.Repo] = true
		// New repo: Status stays empty (sc never supplies it).
		out = append(out, sc)
	}

	return out
}
