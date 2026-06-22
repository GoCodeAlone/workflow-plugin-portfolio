package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGitFactsCleanRepo verifies GitFacts on a committed-clean repo:
// lastCommitISO parses as RFC3339 and uncommitted == false.
func TestGitFactsCleanRepo(t *testing.T) {
	hasGit(t)
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("commit", "-q", "--allow-empty", "-m", "init")

	iso, uncommitted, err := GitFacts(dir)
	if err != nil {
		t.Fatalf("GitFacts: %v", err)
	}
	if uncommitted {
		t.Errorf("clean repo: uncommitted=true, want false")
	}
	if _, perr := time.Parse(time.RFC3339, iso); perr != nil {
		t.Errorf("lastCommitISO %q not RFC3339: %v", iso, perr)
	}
}

// TestGitFactsDirtyRepo verifies the uncommitted flag flips when there are
// untracked or modified files.
func TestGitFactsDirtyRepo(t *testing.T) {
	hasGit(t)
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("commit", "-q", "--allow-empty", "-m", "init")

	// Create an untracked file -> `git status --porcelain` is non-empty.
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, uncommitted, err := GitFacts(dir)
	if err != nil {
		t.Fatalf("GitFacts: %v", err)
	}
	if !uncommitted {
		t.Errorf("dirty repo: uncommitted=false, want true")
	}
}

// TestGitFactsNotARepo verifies GitFacts errors on a non-repo directory.
func TestGitFactsNotARepo(t *testing.T) {
	hasGit(t)
	dir := t.TempDir()
	_, _, err := GitFacts(dir)
	if err == nil {
		t.Fatal("expected error for non-repo directory")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not a git repository") &&
		!strings.Contains(strings.ToLower(err.Error()), "unknown revision") &&
		!strings.Contains(strings.ToLower(err.Error()), "no such") {
		// Be lenient on the exact git wording across versions, but the
		// error should clearly indicate the path is not a usable repo.
		t.Errorf("error message should indicate not-a-repo; got: %v", err)
	}
}
