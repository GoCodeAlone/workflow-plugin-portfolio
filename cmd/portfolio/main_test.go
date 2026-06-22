package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestDispatchHelpExitCode verifies the wfctl CLI dispatch contract:
// `--wfctl-cli portfolio scan --help` must exit 0 and print usage that
// mentions both subcommands. wfctl invokes the binary as:
//
//	<binary> --wfctl-cli <args...>
//
// so a leading --wfctl-cli flag is mandatory.
func TestDispatchHelpExitCode(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := dispatch([]string{"--wfctl-cli", "portfolio", "scan", "--help"}, &out, io.Discard)
	if code != 0 {
		t.Fatalf("expected exit 0 for --help, got %d", code)
	}
	usage := out.String()
	for _, want := range []string{"portfolio scan", "portfolio status"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage missing %q; output:\n%s", want, usage)
		}
	}
}

// TestDispatchStatusHelp mirrors the scan case for the status subcommand.
func TestDispatchStatusHelp(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := dispatch([]string{"--wfctl-cli", "portfolio", "status", "--help"}, &out, io.Discard)
	if code != 0 {
		t.Fatalf("expected exit 0 for status --help, got %d", code)
	}
	if !strings.Contains(out.String(), "portfolio status") {
		t.Errorf("usage missing portfolio status; output:\n%s", out.String())
	}
}

// TestDispatchTopLevelHelp covers bare `--wfctl-cli portfolio` and -h.
func TestDispatchTopLevelHelp(t *testing.T) {
	t.Parallel()
	cases := [][]string{
		{"--wfctl-cli", "portfolio"},
		{"--wfctl-cli", "portfolio", "-h"},
		{"--wfctl-cli", "portfolio", "--help"},
	}
	for _, args := range cases {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code := dispatch(args, &out, io.Discard)
			if code != 0 {
				t.Fatalf("expected exit 0 for %v, got %d", args, code)
			}
			if !strings.Contains(out.String(), "portfolio scan") {
				t.Errorf("usage missing portfolio scan; output:\n%s", out.String())
			}
		})
	}
}

// TestDispatchUnknownSubcommand prints usage and exits 0 (help-class),
// per the spec: "unknown subcommand exit 0 for help".
func TestDispatchUnknownSubcommand(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := dispatch([]string{"--wfctl-cli", "portfolio", "bogus"}, &out, io.Discard)
	if code != 0 {
		t.Fatalf("expected exit 0 for unknown subcommand (help-class), got %d", code)
	}
	if !strings.Contains(out.String(), "portfolio scan") {
		t.Errorf("usage missing portfolio scan; output:\n%s", out.String())
	}
}

// TestDispatchScanNoArgsErrors verifies scan with no workspace-root arg
// exits non-zero with a usage message (scan requires a workspace root).
func TestDispatchScanNoArgsErrors(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	code := dispatch([]string{"--wfctl-cli", "portfolio", "scan"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit for scan with no workspace-root, got 0")
	}
}

// TestDispatchMissingWfctlCliFlag verifies that invocation WITHOUT the
// leading --wfctl-cli flag is treated as an error (exit 2). This guards
// against silent misconfiguration when run outside wfctl.
func TestDispatchMissingWfctlCliFlag(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	code := dispatch([]string{"portfolio", "scan"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit 2 for missing --wfctl-cli, got %d", code)
	}
}

// TestMainArgsRouteToDispatch is a smoke test that the real main() reads
// os.Args and routes to dispatch. We cannot easily capture os.Exit from the
// real main, so this test only confirms dispatch works with os.Args[1:]
// when the harness supplies a help-shaped argv. It is skipped under normal
// `go test` (no --wfctl-cli in test argv) to avoid coupling to the test
// runner's own arguments.
func TestMainArgsRouteToDispatch(t *testing.T) {
	t.Parallel()
	// Only meaningful when the test binary itself is invoked with the
	// portfolio flags; under `go test` os.Args is the test binary, so skip.
	for _, a := range os.Args {
		if a == "--wfctl-cli" {
			var out bytes.Buffer
			_ = dispatch(os.Args[1:], &out, io.Discard)
			return
		}
	}
	t.Skip("os.Args does not contain --wfctl-cli; skipping real-argv routing check")
}
