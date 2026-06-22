package projects

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// fileHeader is the canonical PROJECTS.md header (title + block-shape legend).
// It documents the format for human curators and is emitted by Write.
const fileHeader = `# Projects

<!-- Human-curated mapping of logical projects to repo sets, with generator-
     written scan roll-ups. Block shape:

       ## <project>
       status: <free-form status>
       phase: <number>
       - repos: <owner/name>, <owner/name>, ...   (comma-sep full-names)
       - goal: <one-line goal>                      (curated)
       - blockers: <current blockers>               (curated)
       - next: <next step>                          (curated)
       - design: <path to design doc>               (curated)
       - scan: last-activity <date>, active <n>/<total> repos, open-PRs <n>, open-issues <n>
                                                    (generator-written; rewritten each scan)

     Curated fields (status, phase, repos, goal, blockers, next, design) are
     preserved byte-identical across re-scans — only the - scan: row changes.
     A trailing Unmapped section lists catalog repos in no project (generator-
     written, regenerated each scan). -->
`

// Write renders the PROJECTS.md file: the canonical header, one block per
// project (preserving curated fields verbatim, P-V1), and a trailing
// `## Unmapped` section listing catalog repos in no project (omitted when
// empty).
//
// All field values are rendered verbatim — the writer does NOT mutate, trim,
// or normalize curated fields. The scan row is written exactly as stored on
// each Project (Merge has already formatted it).
func Write(w io.Writer, projects []Project, unmapped []string) error {
	var b strings.Builder

	b.WriteString(fileHeader)
	b.WriteString("\n")

	for _, p := range projects {
		writeProject(&b, p)
	}

	if len(unmapped) > 0 {
		writeUnmapped(&b, unmapped)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// writeProject renders one project block. Curated fields are rendered
// VERBATIM (P-V1) — the scan row is the only generator-written field.
func writeProject(b *strings.Builder, p Project) {
	fmt.Fprintf(b, "## %s\n\n", p.Name)
	fmt.Fprintf(b, "status: %s\n", p.Status)
	fmt.Fprintf(b, "phase: %s\n\n", p.Phase)

	fmt.Fprintf(b, "- repos: %s\n", strings.Join(p.Repos, ", "))
	fmt.Fprintf(b, "- goal: %s\n", p.Goal)
	fmt.Fprintf(b, "- blockers: %s\n", p.Blockers)
	fmt.Fprintf(b, "- next: %s\n", p.Next)
	fmt.Fprintf(b, "- design: %s\n", p.Design)
	fmt.Fprintf(b, "- scan: %s\n", p.Scan)
	b.WriteString("\n")
}

// writeUnmapped renders the ## Unmapped section listing repos in no project.
// Repos are sorted for deterministic output.
func writeUnmapped(b *strings.Builder, unmapped []string) {
	sorted := append([]string(nil), unmapped...)
	sort.Strings(sorted)
	b.WriteString("## Unmapped\n\n")
	b.WriteString("<!-- Generator-written: catalog repos in no project. Regenerated each scan. -->\n\n")
	for _, r := range sorted {
		fmt.Fprintf(b, "- %s\n", r)
	}
	b.WriteString("\n")
}
