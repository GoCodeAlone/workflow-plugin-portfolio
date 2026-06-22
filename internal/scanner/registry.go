package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/GoCodeAlone/workflow/capability/inventory"
)

// CapabilityInventory wraps inventory.CollectEcosystem for the portfolio
// scanner. CollectEcosystem calls LoadTaxonomy(opts.TaxonomyPath) FIRST
// (taxonomy is MANDATORY and NOT embedded), so taxonomyPath must resolve to
// a real file or this returns an error — the caller degrades (warns +
// skips the capability path) rather than hard-failing the whole scan.
//
// registryDir points at the workflow-registry root (or its plugins/ subdir;
// CollectEcosystem handles either). repoRoot is the workspace root hosting
// local workflow-plugin-* checkouts. workflowVersion is stamped into the
// inventory metadata.
func CapabilityInventory(registryDir, repoRoot, taxonomyPath, workflowVersion string) (*inventory.Inventory, error) {
	inv, err := inventory.CollectEcosystem(inventory.EcosystemOptions{
		RegistryDir:     registryDir,
		RepoRoot:        repoRoot,
		TaxonomyPath:    taxonomyPath,
		GeneratedAt:     time.Now().UTC(),
		WorkflowVersion: workflowVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("capability inventory: %w", err)
	}
	return inv, nil
}

// RegistryEntry holds the two manifest fields the portfolio scanner needs
// that are NOT exposed by inventory.Provider (which carries ReleaseStatus
// but not Private/MinEngineVersion): `private` and `minEngineVersion`.
//
// The cmd/wfctl RegistryManifest struct is package main (unimportable), so
// we define this LOCAL struct and json.Unmarshal each
// workflow-registry/plugins/*/manifest.json for these two fields only.
// ReleaseStatus comes from the Inventory, not from here.
//
// D16 — per-field fallback semantics:
//   - `private` absent   -> Private == nil (caller emits `?`; NEVER assume
//     public; ⊥ the absence of evidence is not evidence of public-ness).
//   - minEngineVersion absent -> MinEngineVersion == "" (caller emits `?`).
type RegistryEntry struct {
	// Private is nil when the field is absent (unknown), *true when
	// explicitly private, *false when explicitly public. Callers MUST emit
	// `?` on nil rather than defaulting to public.
	Private *bool `json:"private"`

	// MinEngineVersion is empty when absent (caller emits `?`).
	MinEngineVersion string `json:"minEngineVersion"`
}

// LoadRegistryEntry reads a workflow-registry manifest.json and returns its
// `private` and `minEngineVersion` fields. Unknown fields are ignored (only
// these two are decoded). A missing file or malformed JSON returns an error.
func LoadRegistryEntry(manifestPath string) (RegistryEntry, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return RegistryEntry{}, fmt.Errorf("read registry manifest %s: %w", manifestPath, err)
	}
	var entry RegistryEntry
	if jErr := json.Unmarshal(data, &entry); jErr != nil {
		return RegistryEntry{}, fmt.Errorf("parse registry manifest %s: %w", manifestPath, jErr)
	}
	return entry, nil
}
