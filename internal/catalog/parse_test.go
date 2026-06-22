package catalog

import (
	"strings"
	"testing"
)

// TestParseCatalogRoundTripsStatus verifies the writer's output can be
// parsed back, recovering the Status field verbatim — this is the round-
// trip that makes V1 work: scan reads existing PORTFOLIO.md via
// ParseCatalog, merges, writes; the Status survives because ParseCatalog
// recovers it byte-identical.
func TestParseCatalogRoundTripsStatus(t *testing.T) {
	original := []Project{
		{
			Repo:     "GoCodeAlone/workflow-plugin-auth",
			Category: "plugin",
			Status:   "active — v0.3.0 shipped\n  - multi-line\n  - preserved",
		},
		{
			Repo:     "GoCodeAlone/workflow",
			Category: "engine",
			Status:   "v0.80.25",
		},
	}
	var buf strings.Builder
	if err := WriteCatalog(&buf, original, nil); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCatalog(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}
	byRepo := map[string]string{}
	for _, p := range parsed {
		byRepo[p.Repo] = p.Status
	}
	// The multi-line status is written with "  " indent per line; ParseCatalog
	// must strip the indent to recover the verbatim status.
	for repo, want := range map[string]string{
		"GoCodeAlone/workflow-plugin-auth": "active — v0.3.0 shipped\n  - multi-line\n  - preserved",
		"GoCodeAlone/workflow":             "v0.80.25",
	} {
		if byRepo[repo] != want {
			t.Errorf("ParseCatalog(%q):\n got: %q\nwant: %q", repo, byRepo[repo], want)
		}
	}
}

// TestParseCatalogEmpty verifies parsing an empty/minimal catalog yields no
// projects without error.
func TestParseCatalogEmpty(t *testing.T) {
	parsed, err := ParseCatalog(strings.NewReader("# Portfolio Catalog\n\n"))
	if err != nil {
		t.Fatalf("ParseCatalog empty: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected 0 projects from minimal catalog, got %d", len(parsed))
	}
}

// TestParseCatalogExtractsCategory verifies machine-derived fields like
// category are also recovered (so Merge has them as a baseline — though scan
// will overwrite them).
func TestParseCatalogExtractsCategory(t *testing.T) {
	original := []Project{{Repo: "A/x", Category: "plugin", Status: "ok"}}
	var buf strings.Builder
	if err := WriteCatalog(&buf, original, nil); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCatalog(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 project, got %d", len(parsed))
	}
	if parsed[0].Category != "plugin" {
		t.Errorf("Category = %q, want plugin", parsed[0].Category)
	}
}
