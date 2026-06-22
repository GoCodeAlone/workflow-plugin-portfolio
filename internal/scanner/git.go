package scanner

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GitFacts captures lightweight per-repo facts used to populate the catalog:
//
//   - lastCommitISO: the committer-date ISO-8601 of HEAD
//     (`git log -1 --format=%cI`). RFC3339-shaped.
//   - uncommitted:   true if `git status --porcelain` is non-empty
//     (any staged, unstaged, or untracked change counts).
//
// All three values come from real git subprocesses (no mocks). An error is
// returned if path is not a usable git repository (e.g. no HEAD).
func GitFacts(path string) (lastCommitISO string, uncommitted bool, err error) {
	// lastCommitISO via %cI (committer date, strict ISO 8601). We capture
	// combined output so the human-readable git error ("not a git
	// repository", "unknown revision 'head'") is available for classification
	// — exec.ExitError.Error() only carries "exit status N".
	cmd := exec.Command("git", "-C", path, "log", "-1", "--format=%cI")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// `git log -1` exits 128 on: not a repo, no commits yet, bad HEAD.
		// All three are "path is not a usable repository" from the catalog's
		// perspective — classify and re-wrap with the captured message.
		if isNotARepoOrNoCommits(err, out) {
			return "", false, fmt.Errorf("git facts %q: not a git repository or no commits (%s)",
				path, strings.TrimSpace(strings.ToLower(string(out))))
		}
		return "", false, fmt.Errorf("git log %q: %w (%s)", path, err, strings.TrimSpace(string(out)))
	}
	lastCommitISO = strings.TrimSpace(string(out))

	// status --porcelain: empty output means clean.
	st, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	if err != nil {
		// log succeeded but status failed — unexpected; surface it.
		return lastCommitISO, false, fmt.Errorf("git status %q: %w", path, err)
	}
	uncommitted = len(strings.TrimSpace(string(st))) > 0
	return lastCommitISO, uncommitted, nil
}

// isNotARepoOrNoCommits classifies a git invocation failure as the
// "path is not a usable repository" class. The caller MUST pass the captured
// output (stderr) because exec.ExitError carries only "exit status N".
//
// Signals (any one):
//   - exit code 128 from `git log -1` (the canonical not-a-repo / no-commits
//     exit; robust across git versions where the human wording drifts), AND
//     the output text is not empty;
//   - the git binary is missing (exec.ErrNotFound) — treated as NOT this
//     class so callers handle the environment error distinctly;
//   - the well-known human fragments appear in the output.
func isNotARepoOrNoCommits(err error, out []byte) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		// git binary itself missing — environment error, not not-a-repo.
		return false
	}
	msg := strings.ToLower(string(out))
	// Human-wording fragments across git 2.30+.
	for _, frag := range []string{
		"not a git repository",
		"unknown revision",
		"does not have any commits",
		"bad revision",
		"ambiguous argument 'head'",
		"no such file or directory", // .git/HEAD missing
	} {
		if strings.Contains(msg, frag) {
			return true
		}
	}
	// Exit 128 on `git log -1` is, in practice, only the not-a-repo /
	// no-commits class. Treat it as such when we have no better signal.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
		return true
	}
	return false
}
