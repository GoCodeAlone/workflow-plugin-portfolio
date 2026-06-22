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
// `## Unmapped` section which is generator-written), and within each block
// reads the status:/phase: inline fields plus the `- repos:`/`- goal:`/
// `- blockers:`/`- next:`/`- design:`/`- scan:` bullet rows.
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
			// parsed as a project.
			if heading == "Unmapped" {
				break
			}
			current = &Project{Name: heading}
			continue
		}

		if current == nil {
			continue
		}

		// Inline header fields: "status:" / "phase:".
		if strings.HasPrefix(trimmed, "status:") {
			current.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
			continue
		}
		if strings.HasPrefix(trimmed, "phase:") {
			current.Phase = strings.TrimSpace(strings.TrimPrefix(trimmed, "phase:"))
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

// splitBullet splits a "- field: value" bullet into (field, value). Returns
// ok=false if the bullet has no colon (not a recognized field row).
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
