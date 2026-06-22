// Package visibility enforces pre-write safety invariants on the portfolio
// catalog generator. The headline guard (V7): the workspace repo is PRIVATE,
// and the scanner MUST fail closed (abort the write) if that cannot be
// confirmed — never proceed to overwrite docs on an assumed-private repo.
package visibility

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNotPrivate is returned by AssertTargetPrivate when the target repo is
// confirmed PUBLIC (or its visibility cannot be verified). Callers MUST
// treat this as fail-closed: abort the write.
var ErrNotPrivate = errors.New("target repo is not confirmed PRIVATE")

// DefaultTargetRepo is the canonical workspace repo asserted by the portfolio
// scanner when no explicit target is supplied. Parameterized in
// AssertTargetPrivate for testability.
const DefaultTargetRepo = "GoCodeAlone/workspace"

// AssertTargetPrivate verifies (V7) that the target repo is PRIVATE via
// `gh repo view <repo> --json visibility`. It returns nil only when gh
// confirms visibility == "PRIVATE". Any other outcome — PUBLIC, unknown,
// gh missing/unauthenticated/offline, repo not found, or empty target —
// returns a non-nil error wrapping ErrNotPrivate (fail closed: never
// proceed to write).
//
// The repo target is a parameter so tests can point at a known-PUBLIC repo
// (GoCodeAlone/workflow) to exercise the negative path. Callers MUST supply
// a concrete target; an empty string is rejected (fail closed) rather than
// silently substituting the default — the caller is responsible for passing
// DefaultTargetRepo when it wants the canonical workspace assertion.
func AssertTargetPrivate(ctx context.Context, repo string) error {
	target := strings.TrimSpace(repo)
	if target == "" {
		return fmt.Errorf("%w: empty target repo (pass DefaultTargetRepo explicitly)", ErrNotPrivate)
	}

	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("%w: gh binary not found in PATH, cannot verify %q is private (%v)", ErrNotPrivate, target, err)
	}

	// Bound the gh invocation by the caller's context deadline (if any).
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", target, "--json", "visibility")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: gh repo view %q failed: %v (%s)", ErrNotPrivate, target, err, strings.TrimSpace(string(out)))
	}

	var view struct {
		Visibility string `json:"visibility"`
	}
	if jErr := json.Unmarshal(out, &view); jErr != nil {
		return fmt.Errorf("%w: gh repo view %q returned unparseable JSON: %v", ErrNotPrivate, target, jErr)
	}

	if !strings.EqualFold(strings.TrimSpace(view.Visibility), "PRIVATE") {
		return fmt.Errorf("%w: %q visibility=%q", ErrNotPrivate, target, view.Visibility)
	}
	return nil
}
