package catalog

import "strings"

// NormalizeRepo reduces a git remote URL or repo identifier to its canonical
// "owner/name" form — the MATCH KEY used across the portfolio (Merge) and
// projects (RollUp) layers.
//
// Strips:
//   - scheme prefixes: "https://", "git@github.com:", "ssh://git@github.com/"
//   - ".git" suffix
//
// "git@github.com:GoCodeAlone/workflow-plugin-auth.git" -> "GoCodeAlone/workflow-plugin-auth"
// "https://github.com/GoCodeAlone/workflow.git"          -> "GoCodeAlone/workflow"
// "GoCodeAlone/workflow"                                -> "GoCodeAlone/workflow"  (idempotent)
//
// A bare name with no slash is returned as-is (the caller may fall back to a
// path-derived name).
func NormalizeRepo(remote string) string {
	u := strings.TrimSpace(remote)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "git@github.com:")
	u = strings.TrimPrefix(u, "ssh://git@github.com/")
	u = strings.TrimSuffix(u, ".git")
	// Keep the last two path segments (owner/name).
	if parts := strings.Split(u, "/"); len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return u
}
