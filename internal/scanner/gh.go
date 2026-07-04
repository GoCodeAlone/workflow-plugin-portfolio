package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// ErrGHUnavailable is a sentinel skip error returned by GHFacts when the gh
// CLI is missing, offline, unauthenticated, or rate-limited. Callers MUST
// degrade gracefully (V5) on this error rather than abort the scan: gh facts
// are a best-effort enrichment, not a hard dependency.
var ErrGHUnavailable = errors.New("gh unavailable or unauthenticated")

// ghFacts is the cached payload for one repo's gh queries.
type ghFacts struct {
	openPRs              int
	openPRsUnknown       bool
	openIssues           int
	openIssuesUnknown    bool
	latestRelease        string
	latestReleaseUnknown bool
}

// GHFactsResult is the detailed, per-field gh enrichment result for one repo.
// Unknown flags distinguish "gh could not answer this field" from real zero
// counts or a repo with no stable releases.
type GHFactsResult struct {
	OpenPRs              int
	OpenPRsUnknown       bool
	OpenIssues           int
	OpenIssuesUnknown    bool
	LatestRelease        string
	LatestReleaseUnknown bool
}

// ghCache memoizes GHFacts per-repoFullName so repeated lookups during a
// single scan don't re-invoke gh (rate-limit hygiene + speed). Keyed on the
// fully-qualified repo name ("owner/name").
var ghCache = struct {
	mu sync.Mutex
	m  map[string]ghFacts
}{
	m: make(map[string]ghFacts),
}

// GHFacts queries gh for a repo's open PR count, open issue count, and latest
// stable release tag. It preserves the legacy all-or-nothing contract: if any
// field is unknown, it returns ErrGHUnavailable so old callers do not mistake
// zero values for known zeroes. New callers that can render partial results
// should use GHFactsDetailed.
//
// The three gh invocations use CombinedOutput so stderr is available for the
// error message:
//
//	gh pr list --state open --json number
//	gh issue list --state open --json number
//	gh release view --json tagName,isPrerelease
//
// Counts derive from the JSON array length (only `number` is requested to
// minimize payload). The latest release follows GitHub's latest stable
// release semantics, with a stable-list fallback for older gh versions.
func GHFacts(repoFullName string) (openPRs, openIssues int, latestRelease string, err error) {
	facts, err := GHFactsDetailed(repoFullName)
	if err != nil {
		return 0, 0, "", err
	}
	if facts.OpenPRsUnknown || facts.OpenIssuesUnknown || facts.LatestReleaseUnknown {
		return facts.OpenPRs, facts.OpenIssues, facts.LatestRelease, fmt.Errorf("%w: partial gh facts unavailable for %s", ErrGHUnavailable, repoFullName)
	}
	return facts.OpenPRs, facts.OpenIssues, facts.LatestRelease, nil
}

// GHFactsDetailed queries gh for a repo's open PR count, open issue count, and
// latest stable release tag independently. Results are cached per-repo
// in-process for the life of the scan. Field-specific failures set Unknown
// flags; a non-nil error is returned only when gh cannot provide any field or
// the repo name is invalid.
func GHFactsDetailed(repoFullName string) (GHFactsResult, error) {
	if strings.TrimSpace(repoFullName) == "" {
		return GHFactsResult{}, fmt.Errorf("%w: empty repo name", ErrGHUnavailable)
	}

	// Cache hit.
	ghCache.mu.Lock()
	if cached, ok := ghCache.m[repoFullName]; ok {
		ghCache.mu.Unlock()
		return cached.toResult(), nil
	}
	ghCache.mu.Unlock()

	if _, lookupErr := exec.LookPath("gh"); lookupErr != nil {
		return GHFactsResult{}, fmt.Errorf("%w: gh binary not found in PATH (%v)", ErrGHUnavailable, lookupErr)
	}

	var result GHFactsResult
	failures := 0
	prs, prErr := ghCount(repoFullName, "pr", "--state", "open", "--json", "number")
	if prErr != nil {
		result.OpenPRsUnknown = true
		failures++
	} else {
		result.OpenPRs = prs
	}
	issues, issErr := ghCount(repoFullName, "issue", "--state", "open", "--json", "number")
	if issErr != nil {
		result.OpenIssuesUnknown = true
		failures++
	} else {
		result.OpenIssues = issues
	}
	rel, relErr := ghLatestRelease(repoFullName)
	if relErr != nil {
		result.LatestReleaseUnknown = true
		failures++
	} else {
		result.LatestRelease = rel
	}

	ghCache.mu.Lock()
	ghCache.m[repoFullName] = ghFactsFromResult(result)
	ghCache.mu.Unlock()
	if failures == 3 {
		return result, fmt.Errorf("%w: no gh facts available for %s", ErrGHUnavailable, repoFullName)
	}
	return result, nil
}

// ghCount runs `gh <kind> <repo> <args...>` expecting a JSON array, returning
// the element count. A non-zero exit or unparseable JSON -> ErrGHUnavailable.
func ghCount(repoFullName, kind string, args ...string) (int, error) {
	full := append([]string{kind, "list", "--repo", repoFullName}, args...)
	out, err := exec.Command("gh", full...).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("%w: gh %s list %q: %v (%s)", ErrGHUnavailable, kind, repoFullName, err, strings.TrimSpace(string(out)))
	}
	var arr []json.RawMessage
	if jErr := json.Unmarshal(out, &arr); jErr != nil {
		return 0, fmt.Errorf("%w: gh %s list %q: unmarshal JSON (%v)", ErrGHUnavailable, kind, repoFullName, jErr)
	}
	return len(arr), nil
}

// ghLatestRelease returns GitHub's latest stable release tag. `gh release view`
// follows GitHub's latest-release semantics; fallback list mode explicitly
// excludes drafts and prereleases for older gh versions.
func ghLatestRelease(repoFullName string) (string, error) {
	if tag, err := ghLatestReleaseView(repoFullName); err == nil {
		return tag, nil
	}
	out, err := exec.Command("gh", "release", "list", "--repo", repoFullName, "--exclude-drafts", "--exclude-pre-releases", "--limit", "1").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: gh release list %q: %v (%s)", ErrGHUnavailable, repoFullName, err, strings.TrimSpace(string(out)))
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		// No releases: not an error, just empty.
		return "", nil
	}
	firstLine := trimmed
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	fields := strings.Fields(firstLine)
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

func ghLatestReleaseView(repoFullName string) (string, error) {
	out, err := exec.Command("gh", "release", "view", "--repo", repoFullName, "--json", "tagName,isPrerelease").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: gh release view %q: %v (%s)", ErrGHUnavailable, repoFullName, err, strings.TrimSpace(string(out)))
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return "", nil
	}
	var payload struct {
		TagName      string `json:"tagName"`
		IsPrerelease bool   `json:"isPrerelease"`
	}
	if jErr := json.Unmarshal([]byte(trimmed), &payload); jErr != nil {
		return "", fmt.Errorf("%w: gh release view %q: unmarshal JSON (%v)", ErrGHUnavailable, repoFullName, jErr)
	}
	if payload.IsPrerelease {
		return "", fmt.Errorf("%w: gh release view %q returned prerelease %q", ErrGHUnavailable, repoFullName, payload.TagName)
	}
	return payload.TagName, nil
}

func (f ghFacts) toResult() GHFactsResult {
	return GHFactsResult{
		OpenPRs:              f.openPRs,
		OpenPRsUnknown:       f.openPRsUnknown,
		OpenIssues:           f.openIssues,
		OpenIssuesUnknown:    f.openIssuesUnknown,
		LatestRelease:        f.latestRelease,
		LatestReleaseUnknown: f.latestReleaseUnknown,
	}
}

func ghFactsFromResult(f GHFactsResult) ghFacts {
	return ghFacts{
		openPRs:              f.OpenPRs,
		openPRsUnknown:       f.OpenPRsUnknown,
		openIssues:           f.OpenIssues,
		openIssuesUnknown:    f.OpenIssuesUnknown,
		latestRelease:        f.LatestRelease,
		latestReleaseUnknown: f.LatestReleaseUnknown,
	}
}
