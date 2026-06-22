// Package scanner walks sibling git repositories under a workspace root and
// captures per-repo facts (remote, last commit, dirty flag). It is the data
// source for the portfolio catalog emitted by `wfctl portfolio scan`.
//
// Invariant V8 — remote-dedup + worktree skip:
//   - Detection uses `git -C <dir> rev-parse --show-toplevel`. Worktrees
//     expose `.git` as a regular FILE (not a directory), so a naive
//     `find -type d -name .git` would miss them; rev-parse is authoritative.
//   - Reserved immediate-child names anchored at root are skipped
//     (`_worktrees`, `_codex_worktrees`, `.codex-worktrees`, `_tmp`,
//     `.claude`, `.git`) — these host scratch worktrees / state, not repos.
//   - Repositories sharing a remote URL are deduped, keeping the shallowest
//     path (by segment count, then lexical for determinism).
package scanner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Repo is one discovered git repository under the walk root.
type Repo struct {
	// Path is the absolute, symlink-resolved on-disk path to the repo root
	// (the value of `git rev-parse --show-toplevel`). Resolving symlinks
	// matters on macOS where /var -> /private/var; comparing logical paths
	// across repos would mis-cluster.
	Path string

	// Remote is the origin remote URL (`git remote get-url origin`). Empty
	// if the repo has no origin (it is still walked; dedup keys on the
	// empty string, which clusters all origin-less repos together — usually
	// unintended, so callers should treat empty origin as a distinct repo
	// by also keying on path).
	Remote string
}

// reservedTopLevelNames are immediate-child dir names anchored at the walk
// root that are skipped unconditionally: they host scratch worktrees / agent
// state rather than first-class portfolio entries. The name ".git" is
// included for completeness (a bare check would also exclude it, but a
// root-level .git dir would otherwise be detected as a repo by rev-parse).
var reservedTopLevelNames = map[string]bool{
	"_worktrees":        true,
	"_codex_worktrees":  true,
	".codex-worktrees":  true,
	"_tmp":              true,
	".claude":           true,
	".git":              true,
}

// WalkRepos walks the IMMEDIATE SIBLING directories of root, returning the
// distinct set of git repositories keyed by origin remote URL (keeping the
// shallowest path on collision). Non-git directories and reserved
// scratch/worktree names are skipped.
//
// A raw-vs-distinct count is written to stderr (logw) so operators can see
// how many candidates were considered vs. how many survived dedup.
//
// The walker uses real git subprocesses (no mocks) so worktree .git-FILE
// layout and remote-URL semantics are exercised exactly as in production.
func WalkRepos(root string, logw io.Writer) ([]Repo, error) {
	if logw == nil {
		logw = io.Discard
	}

	// Resolve root through symlinks once so all child paths share a
	// canonical base (macOS /var -> /private/var).
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("walk root %q: %w", root, err)
	}

	entries, err := os.ReadDir(realRoot)
	if err != nil {
		return nil, fmt.Errorf("read walk root %q: %w", realRoot, err)
	}

	// First pass: collect candidate repos (immediate-child dirs that are
	// git repositories and not reserved scratch names).
	var candidates []Repo
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if reservedTopLevelNames[name] {
			continue
		}
		child := filepath.Join(realRoot, name)

		// Detect via rev-parse (authoritative; handles worktree .git FILE).
		top, err := gitRevParseTopLevel(child)
		if err != nil {
			// Not a git repo (rev-parse fails). Skip silently — a workspace
			// is expected to contain a mix of repos and non-repos.
			continue
		}
		// If the worktree root resolves OUTSIDE our walk root, this is a
		// linked worktree whose source lives elsewhere — skip it (the
		// canonical repo entry is the source repo, not the worktree link).
		// We compare on resolved paths to avoid the /var vs /private/var
		// mismatch.
		realTop, _ := filepath.EvalSymlinks(top)
		if realTop != "" && !isWithin(realTop, realRoot) {
			fmt.Fprintf(logw, "skip worktree (toplevel outside root): %s -> %s\n", child, realTop)
			continue
		}
		// Also detect a worktree whose toplevel IS inside root but whose
		// .git is a regular file (a linked worktree created from a sibling
		// repo). The presence of a .git FILE (not dir) at the repo root is
		// the worktree hallmark.
		if isWorktreeDir(realTop) {
			fmt.Fprintf(logw, "skip worktree (.git is file): %s\n", realTop)
			continue
		}

		remote, _ := gitRemoteURL(realTop) // empty ok; origin may be unset
		candidates = append(candidates, Repo{Path: realTop, Remote: remote})
	}

	rawCount := len(candidates)

	// Second pass: dedup by remote URL, keeping the shallowest path.
	distinct := dedupeByRemote(candidates)

	fmt.Fprintf(logw, "walk %s: raw=%d distinct=%d (deduped %d)\n",
		realRoot, rawCount, len(distinct), rawCount-len(distinct))

	// Deterministic order: by path.
	sort.Slice(distinct, func(i, j int) bool {
		return distinct[i].Path < distinct[j].Path
	})
	return distinct, nil
}

// dedupeByRemote keeps the shallowest Repo for each remote URL. Shallowness
// is segment count (fewer = shallower), ties broken lexically for determinism.
// A repo with an empty remote is treated as its own cluster keyed on its path
// (never deduped against another origin-less repo) — origin-less repos are
// presumed intentional locals, not duplicates.
func dedupeByRemote(repos []Repo) []Repo {
	byRemote := make(map[string]Repo, len(repos))
	for _, r := range repos {
		key := r.Remote
		if key == "" {
			// Origin-less repo: keep it unconditionally (keyed on path).
			byRemote["__noremote__"+r.Path] = r
			continue
		}
		if cur, ok := byRemote[key]; ok {
			if shallower(r.Path, cur.Path) {
				byRemote[key] = r
			}
		} else {
			byRemote[key] = r
		}
	}
	out := make([]Repo, 0, len(byRemote))
	for _, r := range byRemote {
		out = append(out, r)
	}
	return out
}

// shallower reports whether a is shallower than b: fewer path segments wins;
// ties broken lexically (the shorter/earlier name is preferred, matching the
// "keep the shallowest path" contract deterministically).
func shallower(a, b string) bool {
	na, nb := strings.Count(filepath.ToSlash(a), "/"), strings.Count(filepath.ToSlash(b), "/")
	if na != nb {
		return na < nb
	}
	return a < b
}

// isWithin reports whether path is equal to or inside base. Both must be
// resolved (symlinks evaluated) beforehand.
func isWithin(path, base string) bool {
	if path == base {
		return true
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..") && !strings.HasPrefix(rel, "/")
}

// isWorktreeDir reports whether dir is a git linked worktree by checking
// that its `.git` entry is a regular file (worktrees) rather than a
// directory (normal repos / bare repos). A missing .git means this is not a
// standard repo root and the caller should rely on rev-parse instead.
func isWorktreeDir(dir string) bool {
	gp := filepath.Join(dir, ".git")
	fi, err := os.Stat(gp)
	if err != nil {
		return false
	}
	return fi.Mode().IsRegular()
}

// gitRevParseTopLevel runs `git -C <dir> rev-parse --show-toplevel` and
// returns the resolved repo root. A non-zero exit means dir is not a usable
// git repo (or git is unavailable).
func gitRevParseTopLevel(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitRemoteURL runs `git -C <dir> remote get-url origin`. Returns "" if
// origin is unset (exit 128) — not an error, just no origin configured.
func gitRemoteURL(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
