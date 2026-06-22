package followups

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractFromRetrosFindsFollowUpBullets proves the extractor greps
// `FOLLOW-UP:` / `FOLLOW UP:` markers in retro markdown and produces
// FollowUp entries with Repo/Text/SourcePath.
func TestExtractFromRetrosFindsFollowUpBullets(t *testing.T) {
	root := t.TempDir()
	// Fixture retro 1: workflow-plugin-auth, two FOLLOW-UP bullets.
	retro1Dir := filepath.Join(root, "docs", "retros", "2026-06-02-auth-bootstrap")
	if err := os.MkdirAll(retro1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	retro1 := `# Retro 2026-06-02 auth bootstrap

## Follow-ups
- FOLLOW-UP: workflow-plugin-auth#41 Phase II IDP (JWKS/refresh/asymmetric)
- FOLLOW UP: multisite#54 migrate admin_bootstrap.go
`
	if err := os.WriteFile(filepath.Join(retro1Dir, "retro.md"), []byte(retro1), 0o644); err != nil {
		t.Fatal(err)
	}

	// Fixture retro 2: different repo.
	retro2Dir := filepath.Join(root, "docs", "retros", "2026-05-27-dns-cascade")
	if err := os.MkdirAll(retro2Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	retro2 := `# Retro DNS cascade
- FOLLOW-UP: workflow-plugin-infra live write-path test
`
	if err := os.WriteFile(filepath.Join(retro2Dir, "retro.md"), []byte(retro2), 0o644); err != nil {
		t.Fatal(err)
	}

	fups, err := ExtractFromRetros(root)
	if err != nil {
		t.Fatalf("ExtractFromRetros: %v", err)
	}
	if len(fups) != 3 {
		t.Fatalf("expected 3 follow-ups, got %d: %+v", len(fups), fups)
	}

	// Verify each text appears.
	texts := make(map[string]bool, len(fups))
	for _, f := range fups {
		texts[strings.TrimSpace(f.Text)] = true
		if f.SourcePath == "" {
			t.Errorf("follow-up %q has empty SourcePath", f.Text)
		}
	}
	for _, want := range []string{
		"workflow-plugin-auth#41 Phase II IDP (JWKS/refresh/asymmetric)",
		"multisite#54 migrate admin_bootstrap.go",
		"workflow-plugin-infra live write-path test",
	} {
		if !texts[want] {
			t.Errorf("missing follow-up text %q\nhave: %v", want, texts)
		}
	}
}

// TestExtractFromRetrosDedupe proves dedup by (text, source): the same
// FOLLOW-UP text appearing twice in ONE retro yields ONE entry.
func TestExtractFromRetrosDedupe(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "retros", "x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	retro := `- FOLLOW-UP: dup text here
- FOLLOW-UP: dup text here
- FOLLOW-UP: unique text
`
	if err := os.WriteFile(filepath.Join(dir, "r.md"), []byte(retro), 0o644); err != nil {
		t.Fatal(err)
	}
	fups, err := ExtractFromRetros(root)
	if err != nil {
		t.Fatalf("ExtractFromRetros: %v", err)
	}
	if len(fups) != 2 {
		t.Errorf("expected 2 (deduped), got %d: %+v", len(fups), fups)
	}
}

// TestExtractFromRetrosDoesNotReadMemory is the ⊥ contract proof: the
// extractor MUST NOT read MEMORY.md. We plant a MEMORY.md with a FOLLOW-UP
// bullet and assert it does NOT appear in the output.
func TestExtractFromRetrosDoesNotReadMemory(t *testing.T) {
	root := t.TempDir()
	// Plant MEMORY.md at root (where the real one lives).
	memory := `- FOLLOW-UP: THIS-SHOULD-NEVER-APPEAR from MEMORY.md
`
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(memory), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also plant a real retro with a legit follow-up.
	dir := filepath.Join(root, "docs", "retros", "ok")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "r.md"), []byte("- FOLLOW-UP: legit retro follow-up\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fups, err := ExtractFromRetros(root)
	if err != nil {
		t.Fatalf("ExtractFromRetros: %v", err)
	}
	for _, f := range fups {
		if strings.Contains(f.Text, "THIS-SHOULD-NEVER-APPEAR") {
			t.Errorf("VIOLATION: extractor read MEMORY.md — found %q (source=%s)", f.Text, f.SourcePath)
		}
		if strings.Contains(f.SourcePath, "MEMORY.md") {
			t.Errorf("VIOLATION: extractor traversed MEMORY.md (source=%s)", f.SourcePath)
		}
	}
}

// TestExtractFromRetrosEmptyRoot verifies a nonexistent retros root returns
// an empty list (not an error) — degradation when no retros exist.
func TestExtractFromRetrosEmptyRoot(t *testing.T) {
	fups, err := ExtractFromRetros(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("ExtractFromRetros on missing root: %v", err)
	}
	if len(fups) != 0 {
		t.Errorf("expected 0 follow-ups from missing root, got %d", len(fups))
	}
}

// TestExtractFromRetrosRepoDerived proves the Repo field is derived from the
// retro's path or content when possible. The extractor parses a leading
// `owner/name#NN` or `repo#NN` token from the text; absent that, Repo is
// empty (caller infers from SourcePath).
func TestExtractFromRetrosRepoDerived(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "retros", "x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	retro := `- FOLLOW-UP: workflow-plugin-auth#41 do the thing
- FOLLOW-UP: standalone task with no repo token
`
	if err := os.WriteFile(filepath.Join(dir, "r.md"), []byte(retro), 0o644); err != nil {
		t.Fatal(err)
	}
	fups, err := ExtractFromRetros(root)
	if err != nil {
		t.Fatalf("ExtractFromRetros: %v", err)
	}
	repoFound := false
	emptyFound := false
	for _, f := range fups {
		if strings.Contains(f.Text, "workflow-plugin-auth#41") {
			if f.Repo != "workflow-plugin-auth" {
				t.Errorf("Repo = %q, want workflow-plugin-auth", f.Repo)
			}
			repoFound = true
		}
		if strings.Contains(f.Text, "standalone task") {
			if f.Repo != "" {
				t.Errorf("standalone Repo = %q, want empty", f.Repo)
			}
			emptyFound = true
		}
	}
	if !repoFound || !emptyFound {
		t.Errorf("expected both repo-tokenized and standalone follow-ups; got repoFound=%v emptyFound=%v", repoFound, emptyFound)
	}
}
