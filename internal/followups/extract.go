// Package followups extracts open follow-up items from retro markdown files.
//
// ⊥ CONTRACT: the extractor reads ONLY `docs/retros/**/*.md` (and any sibling
// retro-tree rooted at retrosRoot). It MUST NEVER read MEMORY.md or any other
// non-retro file — MEMORY.md is an operator scratchpad whose follow-up-style
// bullets are not curated catalog entries. The extractor enforces this by
// (a) walking only the retros sub-tree and (b) skipping any file named
// MEMORY.md encountered (defense-in-depth).
package followups

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FollowUp is one extracted follow-up item.
type FollowUp struct {
	// Repo is the repo token parsed from the text (e.g.
	// "workflow-plugin-auth" from "workflow-plugin-auth#41 ..."). Empty when
	// the text carries no repo token (standalone task).
	Repo string

	// Text is the follow-up text after the FOLLOW-UP:/FOLLOW UP: marker,
	// trimmed of leading "- " and surrounding whitespace. Verbatim otherwise.
	Text string

	// SourcePath is the absolute path to the retro file this was extracted
	// from (for traceability).
	SourcePath string

	// Status is "open" by default (the extractor does not parse closed
	// status; callers may post-process). Reserved for future enrichment.
	Status string
}

// followUpRe matches a line starting with optional "- " then FOLLOW-UP: or
// FOLLOW UP: (case-insensitive on the whole marker, tolerant of the hyphen/
// space variants seen in real retros), capturing the trailing text.
var followUpRe = regexp.MustCompile(`(?i)^\s*[-*]?\s*FOLLOW[- ]UP:\s*(.+)$`)

// repoTokenRe extracts a leading repo reference like "workflow-plugin-auth#41"
// or "workflow-plugin-auth" from the start of the follow-up text. To avoid
// false positives on ordinary prose, we require EITHER:
//   - a `#NN` issue suffix (the canonical "repo#issue" form), OR
//   - a hyphenated multi-segment name (GoCodeAlone convention:
//     workflow-plugin-*, gocodealone-*, multisite*, etc. — single words like
//     "standalone" or "fix" do NOT count).
//
// Group 1 captures the repo name (without the #NN).
var repoTokenRe = regexp.MustCompile(`^((?:[a-zA-Z][a-zA-Z0-9]*-)+[a-zA-Z0-9-]+)(?:#\d+)?\s`)

// ExtractFromRetros walks retrosRoot (expected `<workspace>/docs/retros` or
// any subtree hosting retro markdown) and extracts FOLLOW-UP:/FOLLOW UP:
// bullets. Results are deduped by (text, source) so a bullet repeated in one
// file yields one entry.
//
// ⊥ MEMORY.md is NEVER read: the walker skips any file named MEMORY.md
// (defense-in-depth) and only descends into the retros sub-tree.
//
// A nonexistent retrosRoot returns an empty list (degradation: no retros).
func ExtractFromRetros(retrosRoot string) ([]FollowUp, error) {
	info, err := os.Stat(retrosRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat retros root %s: %w", retrosRoot, err)
	}
	// If the caller passed a single retro file, process it directly.
	if !info.IsDir() {
		return extractFromFile(retrosRoot)
	}

	var all []FollowUp
	seen := make(map[string]bool) // (text|source) dedupe
	walkErr := filepath.WalkDir(retrosRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// ⊥ Defense-in-depth: never read MEMORY.md regardless of where it
		// appears in the walk.
		if strings.EqualFold(filepath.Base(path), "MEMORY.md") {
			return nil
		}
		// Only .md files.
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		fups, err := extractFromFile(path)
		if err != nil {
			return err
		}
		for _, f := range fups {
			key := f.Text + "\x00" + f.SourcePath
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, f)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk retros %s: %w", retrosRoot, walkErr)
	}
	return all, nil
}

// extractFromFile reads one retro markdown file and returns its FOLLOW-UP
// bullets. Each bullet's Repo is parsed from a leading repo token if present.
func extractFromFile(path string) ([]FollowUp, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read retro %s: %w", path, err)
	}
	var out []FollowUp
	for _, line := range strings.Split(string(data), "\n") {
		m := followUpRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[1])
		if text == "" {
			continue
		}
		f := FollowUp{
			Text:       text,
			SourcePath: path,
			Status:     "open",
		}
		if tok := repoTokenRe.FindStringSubmatch(text); tok != nil {
			f.Repo = tok[1]
		}
		out = append(out, f)
	}
	return out, nil
}
