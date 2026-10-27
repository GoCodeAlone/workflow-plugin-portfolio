package catalog

import (
	"strings"
	"testing"
)

func TestParseCatalogStatusCompatibility(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		wantStatus   string
		wantCategory string
		wantNext     bool
	}{
		{"inline only", "status: first\n", "first", "plugin", false},
		{"canonical block", "status:\n  first\n  second\n", "first\nsecond", "plugin", false},
		{"inline continuation", "status: first\n  second\n", "first\nsecond", "plugin", false},
		{"deeper indentation", "status: first\n  second\n    - nested\n", "first\nsecond\n  - nested", "plugin", false},
		{"tab continuation", "status: first\n\tsecond\n", "first\nsecond", "plugin", false},
		{"blank terminates inline", "status: first\n  second\n\n  outside\n", "first\nsecond", "plugin", false},
		{"metadata after inline", "status: first\n- category: engine\n", "first", "engine", false},
		{"metadata after continuation", "status: first\n  second\n- category: engine\n  outside\n", "first\nsecond", "engine", false},
		{"heading terminates inline", "status: first\n  second\n## B/y\n\nstatus: next\n", "first\nsecond", "plugin", true},
		{"tooling terminates inline", "status: first\n  second\n## Tooling Inventory\n\n  outside\n", "first\nsecond", "plugin", false},
		{"existing unindented block tolerance", "status:\nfirst\nsecond\n", "first\nsecond", "plugin", false},
		{"empty inline seeds block", "status: \t\n  first\n  second\n", "first\nsecond", "plugin", false},
		{"continuation at EOF", "status: first\n  second", "first\nsecond", "plugin", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := "# Portfolio Catalog\n\n## A/x\n\n- category: plugin\n\n" + tc.body
			parsed, err := ParseCatalog(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 1
			if tc.wantNext {
				wantCount = 2
			}
			if len(parsed) != wantCount {
				t.Fatalf("entries = %d, want %d", len(parsed), wantCount)
			}
			if parsed[0].Repo != "A/x" || parsed[0].Status != tc.wantStatus || parsed[0].Category != tc.wantCategory {
				t.Fatalf("first entry = %#v; want Repo=A/x, Status=%q, Category=%q", parsed[0], tc.wantStatus, tc.wantCategory)
			}
			if tc.wantNext && (parsed[1].Repo != "B/y" || parsed[1].Status != "next") {
				t.Fatalf("next entry = %#v, want B/y with status next", parsed[1])
			}
		})
	}
}

func TestParseCatalogInlineContinuationEquivalentToBlock(t *testing.T) {
	const want = "first\nsecond\n  - nested"
	for _, body := range []string{
		"status: first\n  second\n    - nested\n",
		"status:\n  first\n  second\n    - nested\n",
	} {
		parsed, err := ParseCatalog(strings.NewReader("## A/x\n\n" + body))
		if err != nil || len(parsed) != 1 {
			t.Fatalf("parse = %#v, %v", parsed, err)
		}
		if parsed[0].Status != want {
			t.Fatalf("Status = %q, want %q", parsed[0].Status, want)
		}
	}
}
