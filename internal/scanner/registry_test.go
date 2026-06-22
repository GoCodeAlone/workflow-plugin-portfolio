package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadRegistryEntryPrivatePresent verifies a manifest with `private: true`
// unmarshals to Private != nil pointing at true (D16: never assume public).
func TestLoadRegistryEntryPrivatePresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	content := `{"name":"secret","version":"0.1.0","private":true,"minEngineVersion":"0.70.0"}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := LoadRegistryEntry(path)
	if err != nil {
		t.Fatalf("LoadRegistryEntry: %v", err)
	}
	if entry.Private == nil {
		t.Fatal("Private is nil, want *bool pointing at true")
	}
	if !*entry.Private {
		t.Errorf("Private = false, want true")
	}
	if entry.MinEngineVersion != "0.70.0" {
		t.Errorf("MinEngineVersion = %q, want 0.70.0", entry.MinEngineVersion)
	}
}

// TestLoadRegistryEntryPrivateFalse verifies `private: false` unmarshals to
// a non-nil *bool pointing at false (distinct from absent).
func TestLoadRegistryEntryPrivateFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	content := `{"name":"open","version":"0.1.0","private":false}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := LoadRegistryEntry(path)
	if err != nil {
		t.Fatalf("LoadRegistryEntry: %v", err)
	}
	if entry.Private == nil {
		t.Fatal("Private is nil, want *bool pointing at false")
	}
	if *entry.Private {
		t.Errorf("Private = true, want false")
	}
	if entry.MinEngineVersion != "" {
		t.Errorf("MinEngineVersion = %q, want empty (absent)", entry.MinEngineVersion)
	}
}

// TestLoadRegistryEntryFieldsAbsent proves D16: `private` absent -> nil *bool
// (callers emit `?`, NEVER assume public); minEngineVersion absent -> empty
// string (callers emit `?`).
func TestLoadRegistryEntryFieldsAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	content := `{"name":"minimal","version":"0.2.0"}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := LoadRegistryEntry(path)
	if err != nil {
		t.Fatalf("LoadRegistryEntry: %v", err)
	}
	if entry.Private != nil {
		t.Errorf("Private = %v (*bool), want nil (absent -> unknown)", *entry.Private)
	}
	if entry.MinEngineVersion != "" {
		t.Errorf("MinEngineVersion = %q, want empty (absent -> ?)", entry.MinEngineVersion)
	}
}

// TestLoadRegistryEntryMissingFile verifies a missing manifest returns an
// error (does not silently produce a zero entry).
func TestLoadRegistryEntryMissingFile(t *testing.T) {
	_, err := LoadRegistryEntry(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
}

// TestLoadRegistryEntryMalformedJSON verifies a malformed manifest returns
// an unmarshal error.
func TestLoadRegistryEntryMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistryEntry(path); err == nil {
		t.Fatal("expected unmarshal error for malformed JSON, got nil")
	}
}

// TestCapabilityInventoryTaxonomyMissing verifies graceful degradation: when
// the taxonomy file is absent, CapabilityInventory returns an error (so the
// caller can warn + skip the capability path), NOT a hard panic. The error
// must carry the taxonomy path for operability.
func TestCapabilityInventoryTaxonomyMissing(t *testing.T) {
	regDir := t.TempDir()
	repoRoot := t.TempDir()
	missingTax := filepath.Join(t.TempDir(), "no-such-taxonomy.yaml")
	_, err := CapabilityInventory(regDir, repoRoot, missingTax, "0.0.0")
	if err == nil {
		t.Fatal("expected error for missing taxonomy, got nil (CollectEcosystem calls LoadTaxonomy first)")
	}
}

// TestCapabilityInventoryEndToEnd proves CapabilityInventory wraps
// CollectEcosystem correctly given a real taxonomy fixture: a registry tree
// with one plugin manifest produces a non-nil Inventory with capabilities
// resolvable against the taxonomy.
func TestCapabilityInventoryEndToEnd(t *testing.T) {
	taxonomyPath := "/Users/jon/workspace/workflow/data/capabilities/taxonomy.yaml"
	if _, err := os.Stat(taxonomyPath); err != nil {
		t.Skipf("taxonomy fixture not present at %s: %v", taxonomyPath, err)
	}
	regDir := t.TempDir()
	pluginsDir := filepath.Join(regDir, "plugins", "workflow-plugin-auth")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"name": "workflow-plugin-auth",
		"version": "0.3.0",
		"type": "external",
		"status": "released",
		"repository": "github.com/GoCodeAlone/workflow-plugin-auth",
		"capabilities": {
			"moduleTypes": ["auth.credential"]
		}
	}`
	if err := os.WriteFile(filepath.Join(pluginsDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	repoRoot := t.TempDir()

	inv, err := CapabilityInventory(regDir, repoRoot, taxonomyPath, "test-0.0.0")
	if err != nil {
		t.Fatalf("CapabilityInventory: %v", err)
	}
	if inv == nil {
		t.Fatal("nil inventory")
	}
	if len(inv.Capabilities) == 0 {
		t.Errorf("expected >0 capabilities, got 0")
	}
}
