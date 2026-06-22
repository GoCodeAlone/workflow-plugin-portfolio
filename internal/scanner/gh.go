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
	openPRs       int
	openIssues    int
	latestRelease string
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

// GHFacts queries gh for a repo's open PR count, open issue count, and
// latest release tag. Results are cached per-repo in-process for the life of
// the scan. On any gh failure (binary missing, offline, unauthenticated,
// rate-limited, repo not found) it returns zero values plus a non-nil
// sentinel error (ErrGHUnavailable) so callers degrade per V5 rather than
// abort the whole scan.
//
// The three gh invocations use CombinedOutput so stderr is available for the
// error message:
//
//	gh pr list --state open --json number
//	gh issue list --state open --json number
//	gh release list --limit 1
//
// Counts derive from the JSON array length (only `number` is requested to
// minimize payload). The latest release is the first line of the release
// table's name column.
func GHFacts(repoFullName string) (openPRs, openIssues int, latestRelease string, err error) {
	if strings.TrimSpace(repoFullName) == "" {
		return 0, 0, "", fmt.Errorf("%w: empty repo name", ErrGHUnavailable)
	}

	// Cache hit.
	ghCache.mu.Lock()
	if cached, ok := ghCache.m[repoFullName]; ok {
		ghCache.mu.Unlock()
		return cached.openPRs, cached.openIssues, cached.latestRelease, nil
	}
	ghCache.mu.Unlock()

	if _, lookupErr := exec.LookPath("gh"); lookupErr != nil {
		return 0, 0, "", fmt.Errorf("%w: gh binary not found in PATH (%v)", ErrGHUnavailable, lookupErr)
	}

	prs, prErr := ghCount(repoFullName, "pr", "list", "--state", "open", "--json", "number")
	if prErr != nil {
		return 0, 0, "", prErr
	}
	issues, issErr := ghCount(repoFullName, "issue", "list", "--state", "open", "--json", "number")
	if issErr != nil {
		return 0, 0, "", issErr
	}
	rel, relErr := ghLatestRelease(repoFullName)
	if relErr != nil {
		return 0, 0, "", relErr
	}

	ghCache.mu.Lock()
	ghCache.m[repoFullName] = ghFacts{
		openPRs:       prs,
		openIssues:    issues,
		latestRelease: rel,
	}
	ghCache.mu.Unlock()
	return prs, issues, rel, nil
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

// ghLatestRelease runs `gh release list --repo <repo> --limit 1` and returns
// the release tag (first whitespace-delimited field of the first row). A repo
// with no releases returns an empty string WITHOUT error (the table is empty,
// not a failure); a non-zero exit -> ErrGHUnavailable.
func ghLatestRelease(repoFullName string) (string, error) {
	out, err := exec.Command("gh", "release", "list", "--repo", repoFullName, "--limit", "1").CombinedOutput()
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
