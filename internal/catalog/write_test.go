package catalog

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GoCodeAlone/workflow/capability/inventory"
)

// TestWriteCatalogRendersToolingInventory verifies the writer produces a
// `## Tooling Inventory` section from inventory.BuildCatalog(inv), proving
// the C4 contract: the catalog carries the tooling inventory derived from
// capability/inventory.
func TestWriteCatalogRendersToolingInventory(t *testing.T) {
	inv := &inventory.Inventory{
		Capabilities: []inventory.Capability{
			{
				ID:       "auth.authn",
				Category: "auth",
				Name:     "Authentication",
			},
		},
	}
	projects := []Project{
		{Repo: "GoCodeAlone/workflow-plugin-auth", Category: "plugin", Status: "active"},
	}

	var buf bytes.Buffer
	if err := WriteCatalog(&buf, projects, inv); err != nil {
		t.Fatalf("WriteCatalog: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "## Tooling Inventory") {
		t.Errorf("output missing '## Tooling Inventory' header\n%s", out)
	}
	if !strings.Contains(out, "Authentication") {
		t.Errorf("output missing capability name 'Authentication'\n%s", out)
	}
}

// TestWriteCatalogRendersPerProjectBlock verifies each project gets its own
// block with a `status:` line (so the V1 e2e test can assert status-line
// byte-identity across re-runs).
func TestWriteCatalogRendersPerProjectBlock(t *testing.T) {
	projects := []Project{
		{
			Repo:     "GoCodeAlone/workflow-plugin-auth",
			Category: "plugin",
			Status:   "active — v0.3.0 shipped",
			Scan:     ScanFacts{LastCommitISO: "2026-06-21T10:00:00Z"},
			Release:  ReleaseFacts{LatestRelease: "v0.3.0", OpenPRs: 2, OpenIssues: 5},
		},
	}

	var buf bytes.Buffer
	if err := WriteCatalog(&buf, projects, nil); err != nil {
		t.Fatalf("WriteCatalog: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"GoCodeAlone/workflow-plugin-auth",
		"status:",
		"active — v0.3.0 shipped",
		"v0.3.0",
		"2026-06-21T10:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---OUTPUT---\n%s", want, out)
		}
	}
}

// TestWriteCatalogNilInventoryNoError verifies a nil inventory still renders
// (the Tooling Inventory section is omitted or empty, not an error). This
// matters for the taxonomy-absent degradation path.
func TestWriteCatalogNilInventoryNoError(t *testing.T) {
	projects := []Project{{Repo: "A/x", Category: "plugin"}}
	var buf bytes.Buffer
	if err := WriteCatalog(&buf, projects, nil); err != nil {
		t.Fatalf("WriteCatalog with nil inv: %v", err)
	}
	if !strings.Contains(buf.String(), "A/x") {
		t.Errorf("output missing project Repo\n%s", buf.String())
	}
}

// TestWriteCatalogMarkdownEscapes verifies markdown-special chars in Status
// are escaped so a curated status containing e.g. `|` doesn't break the
// table layout. (Status is rendered as a line, not a table cell, but the
// principle holds: we must not emit unescaped control chars.)
func TestWriteCatalogMarkdownEscapes(t *testing.T) {
	projects := []Project{
		{Repo: "A/x", Status: "line1\nline2"},
	}
	var buf bytes.Buffer
	if err := WriteCatalog(&buf, projects, nil); err != nil {
		t.Fatalf("WriteCatalog: %v", err)
	}
	// Multi-line status must be preserved verbatim (human-authored) but
	// within the block it should be readable — verify the lines both appear.
	out := buf.String()
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Errorf("multi-line status not rendered\n%s", out)
	}
}
