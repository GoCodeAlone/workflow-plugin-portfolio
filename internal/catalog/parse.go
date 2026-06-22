package catalog

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ParseCatalog reads a PORTFOLIO.md written by WriteCatalog and recovers
// the Project entries — in particular their Status field, byte-identical.
// This is the read half of the V1 round-trip: scan calls ParseCatalog on
// the existing PORTFOLIO.md, merges with freshly scanned facts, and writes;
// Status survives because ParseCatalog recovers it verbatim.
//
// Only identity + machine-baseline + Status fields are recovered. The
// scanner always rewrites Scan/Release/DocsPaths/Provides from live data,
// so those parsed values are discarded in favor of fresh scans (Merge takes
// them from the scanned side).
//
// The parser is line-oriented and tolerant: it scans for `## <repo>`
// headers (level-2 headings, skipping the `# Portfolio Catalog` title and
// any `## Tooling Inventory` section) and, within each block, reads the
// `status:` sub-block (indented continuation lines until a blank line or
// the next header).
func ParseCatalog(r io.Reader) ([]Project, error) {
	scanner := bufio.NewScanner(r)
	// Allow long status lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var projects []Project
	var current *Project
	var inStatus bool
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// A level-2 heading starts a new project block (or the Tooling
		// Inventory section, which terminates parsing of projects).
		if strings.HasPrefix(line, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			// Flush any in-progress project.
			if current != nil {
				projects = append(projects, *current)
				current = nil
			}
			inStatus = false
			// The Tooling Inventory section is not a project.
			if heading == "Tooling Inventory" {
				break
			}
			// A repo heading looks like "owner/name". Heuristic: contains a
			// slash and no spaces (repo names don't have spaces).
			if strings.Contains(heading, "/") && !strings.Contains(heading, " ") {
				current = &Project{Repo: heading}
			}
			continue
		}

		if current == nil {
			continue
		}

		// status: sub-block. Lines after "status:" that are indented (start
		// with whitespace) are continuation lines of the verbatim status.
		if strings.HasPrefix(trimmed, "status:") {
			inStatus = true
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
			if rest != "" {
				// Inline status (single-line form).
				current.Status = rest
				inStatus = false
			}
			continue
		}
		if inStatus {
			if trimmed == "" {
				// Blank line ends the status block.
				inStatus = false
				continue
			}
			// Continuation line: strip exactly the 2-space indent the writer
			// adds, preserving any deeper indentation the human authored.
			content := line
			if strings.HasPrefix(content, "  ") {
				content = content[2:]
			} else {
				content = trimmed
			}
			if current.Status == "" {
				current.Status = content
			} else {
				current.Status += "\n" + content
			}
			continue
		}

		// Other bullet fields we care about recovering as baselines.
		if strings.HasPrefix(trimmed, "- category:") {
			current.Category = strings.TrimSpace(strings.TrimPrefix(trimmed, "- category:"))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan catalog: %w", err)
	}
	if current != nil {
		projects = append(projects, *current)
	}
	return projects, nil
}
