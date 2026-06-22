package visibility

import (
	"context"
	"os/exec"
	"testing"
)

// skipIfNoGH skips the test when the gh CLI binary is unavailable. These
// tests exercise real gh subprocesses (no mocks) so the visibility JSON
// contract (`{"visibility":"PRIVATE"|"PUBLIC"}`) and the fail-closed path
// are validated against the actual binary.
func skipIfNoGH(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh binary not available; skipping real-gh visibility test")
	}
}

// TestAssertTargetPrivatePublicRepoReturnsError proves V7 fail-closed: a
// PUBLIC repo aborts the write (returns a non-nil error). GoCodeAlone/workflow
// is PUBLIC, so it serves as a reliable non-private target.
func TestAssertTargetPrivatePublicRepoReturnsError(t *testing.T) {
	skipIfNoGH(t)
	err := AssertTargetPrivate(context.Background(), "GoCodeAlone/workflow")
	if err == nil {
		t.Fatal("expected error for PUBLIC repo, got nil (V7 fail-closed violated)")
	}
}

// TestAssertTargetPrivateUnknownRepoReturnsError proves an unknown repo also
// fails closed (never proceeds to write on gh error).
func TestAssertTargetPrivateUnknownRepoReturnsError(t *testing.T) {
	skipIfNoGH(t)
	err := AssertTargetPrivate(context.Background(), "GoCodeAlone/__nope_xyz_repo_123__")
	if err == nil {
		t.Fatal("expected error for unknown repo, got nil (V7 fail-closed violated)")
	}
}

// TestAssertTargetPrivateEmptyTargetReturnsError proves an empty target is
// rejected before invoking gh.
func TestAssertTargetPrivateEmptyTargetReturnsError(t *testing.T) {
	if err := AssertTargetPrivate(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
}
