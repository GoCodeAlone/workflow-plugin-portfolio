package catalog

import "testing"

// TestNormalizeRepo covers every remote format the scanner encounters:
// git@ SSH, https, ssh://, already-canonical, and bare names.
func TestNormalizeRepo(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"git@github.com:GoCodeAlone/workflow-plugin-auth.git", "GoCodeAlone/workflow-plugin-auth"},
		{"https://github.com/GoCodeAlone/workflow.git", "GoCodeAlone/workflow"},
		{"ssh://git@github.com/GoCodeAlone/modular", "GoCodeAlone/modular"},
		{"GoCodeAlone/workflow", "GoCodeAlone/workflow"},
		{"GoCodeAlone/workflow-plugin-infra.git", "GoCodeAlone/workflow-plugin-infra"},
		{"bare-repo", "bare-repo"},
		{"  GoCodeAlone/workflow  ", "GoCodeAlone/workflow"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeRepo(c.in); got != c.want {
			t.Errorf("NormalizeRepo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
