package projects

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ParseProjects reads a docs/PROJECTS.md file and recovers the project
// entries — in particular their curated fields, byte-identical. This is the
// read half of the P-V1 round-trip: scan calls ParseProjects on the existing
// PROJECTS.md, rolls up fresh signals, and rewrites ONLY the `- scan:` row;
// every curated field survives because ParseProjects recovers it verbatim.
//
// The parser is line-oriented and tolerant: it scans for `## <name>` headers
// (level-2 headings, skipping the `# Projects` title and the trailing
// `## Unmapped` section which is generator-written). The status + phase are
// read from the INLINE header (`## <name>   status: X   phase: Y`, 3-space
// separators) — the canonical shape Write emits — with a tolerant fallback to
// separate `status:`/`phase:` lines for older seeds. The `- repos:`/`- goal:`/
// `- blockers:`/`- next:`/`- design:`/`- scan:` bullet rows are read as before.
//
// A missing file is an error (the caller decides opt-in skip before calling).
func ParseProjects(path string) ([]Project, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("projects: open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow long scan rows.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var projects []Project
	var current *Project

	flush := func() {
		if current != nil {
			projects = append(projects, *current)
			current = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// A level-2 heading starts a new project block (or the Unmapped
		// section, which terminates parsing of projects).
		if strings.HasPrefix(line, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			flush()
			// The Unmapped section is generator-written and regenerated, not
			// parsed as a project. Match on the bare name (it carries no
			// inline status/phase).
			if name, _, _ := splitInlineHeader(heading); name == "Unmapped" {
				break
			}
			name, status, phase := splitInlineHeader(heading)
			current = &Project{Name: name, Status: status, Phase: phase}
			continue
		}

		if current == nil {
			continue
		}

		// Tolerant fallback for older seeds that put status/phase on their
		// own lines instead of the inline header. The inline header (parsed
		// above) wins when present; these only fill an empty field so the
		// legacy separate-line shape still round-trips.
		if strings.HasPrefix(trimmed, "status:") {
			if current.Status == "" {
				current.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
			}
			continue
		}
		if strings.HasPrefix(trimmed, "phase:") {
			if current.Phase == "" {
				current.Phase = strings.TrimSpace(strings.TrimPrefix(trimmed, "phase:"))
			}
			continue
		}

		// Bullet rows: "- repos:" / "- goal:" / "- blockers:" /
		// "- next:" / "- design:" / "- scan:".
		if strings.HasPrefix(trimmed, "- ") {
			field, value, ok := splitBullet(trimmed)
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			switch field {
			case "repos":
				current.Repos = splitRepos(value)
			case "goal":
				current.Goal = value
			case "blockers":
				current.Blockers = value
			case "next":
				current.Next = value
			case "design":
				current.Design = value
			case "scan":
				current.Scan = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("projects: scan %s: %w", path, err)
	}
	flush()
	return projects, nil
}

// splitInlineHeader splits a level-2 heading body into its name + inline
// status + phase. The canonical shape Write emits is:
//
//	<name>   status: <status>   phase: <phase>
//
// with 3-space separators. name is always returned. status/phase are empty
// when the heading carries no inline fields (older seeds, or a heading whose
// name happens to contain "status:"/"phase:" but not in the canonical shape).
//
// The name is everything before the first "   status:" run; a bare heading
// with no inline fields returns the whole (trimmed) body as the name.
func splitInlineHeader(heading string) (name, status, phase string) {
	statusIdx := strings.Index(heading, "status:")
	phaseIdx := strings.Index(heading, "phase:")
	if statusIdx < 0 && phaseIdx < 0 {
		return heading, "", ""
	}
	// Canonical order is name, then status:, then phase:. If both markers are
	// present, status comes before phase; take the name as everything before
	// the earlier marker.
	first := statusIdx
	if first < 0 || (phaseIdx >= 0 && phaseIdx < first) {
		first = phaseIdx
	}
	name = strings.TrimRight(heading[:first], " ")

	if statusIdx >= 0 {
		// Status value runs from after "status:" up to the phase: marker (if
		// it follows) or end of heading.
		rest := heading[statusIdx+len("status:"):]
		end := strings.Index(rest, "phase:")
		if end >= 0 {
			rest = rest[:end]
		}
		status = strings.TrimSpace(rest)
	}
	if phaseIdx >= 0 {
		rest := heading[phaseIdx+len("phase:"):]
		// Phase value runs up to a subsequent status: marker (defensive: a
		// heading with phase before status), else end.
		end := strings.Index(rest, "status:")
		if end >= 0 {
			rest = rest[:end]
		}
		phase = strings.TrimSpace(rest)
	}
	return name, status, phase
}
func splitBullet(trimmed string) (field, value string, ok bool) {
	rest := strings.TrimPrefix(trimmed, "- ")
	idx := strings.Index(rest, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(rest[:idx]), rest[idx+1:], true
}

// splitRepos splits a comma-separated repos row into trimmed full-names,
// dropping empties.
func splitRepos(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
