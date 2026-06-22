// Command portfolio is the wfctl CLI plugin for the cross-repo portfolio
// catalog generator. It is invoked by wfctl as:
//
//	portfolio --wfctl-cli portfolio <subcommand> [flags...]
//
// The leading --wfctl-cli flag selects CLI-command dispatch mode (vs. the
// go-plugin host protocol used when the workflow engine loads the binary as
// an external plugin). Subcommands:
//
//	portfolio scan    Walk sibling repos and emit a portfolio catalog
//	portfolio status  Show portfolio catalog status / freshness
//
// Both subcommands are stubs in this scaffold; the walker lands in Task 4
// and the catalog emitter in a later PR.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `portfolio — cross-repo portfolio catalog generator

Usage:
  portfolio --wfctl-cli portfolio scan [flags]
  portfolio --wfctl-cli portfolio status [flags]

Subcommands:
  portfolio scan    Walk sibling git repos under a root and emit a portfolio
                    catalog (path, remote, last commit, dirty flag).
  portfolio status  Report freshness / staleness of the last catalog.

Flags:
  --help, -h        Show this usage.

This binary is invoked by wfctl via capabilities.cliCommands[] in plugin.json.
The leading --wfctl-cli flag is required for CLI dispatch.
`

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch routes wfctl CLI invocations. It returns the process exit code
// rather than calling os.Exit so it is unit-testable.
//
// Argv shape: ["--wfctl-cli", "portfolio", <subcommand>, ...]
//   - missing leading --wfctl-cli  -> error (exit 2)
//   - help / unknown subcommand     -> usage to stdout, exit 0
//   - scan / status                 -> subcommand handler (stub for now)
func dispatch(args []string, stdout, stderr io.Writer) int {
	// wfctl always passes a leading --wfctl-cli flag to select CLI mode.
	if len(args) == 0 || args[0] != "--wfctl-cli" {
		fmt.Fprintln(stderr, "error: missing leading --wfctl-cli flag (wfctl invokes the binary as: portfolio --wfctl-cli portfolio <subcommand>)")
		fmt.Fprintln(stderr, "")
		fmt.Fprint(stderr, usage)
		return 2
	}
	rest := args[1:]

	// Strip the "portfolio" command word. wfctl passes the cliCommand name
	// ("portfolio") as the first positional after --wfctl-cli.
	if len(rest) > 0 && rest[0] == "portfolio" {
		rest = rest[1:]
	}

	// No subcommand -> top-level help.
	if len(rest) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}

	sub := rest[0]
	subArgs := rest[1:]

	// Help flags anywhere before a real handler short-circuit to usage.
	if isHelp(sub) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	for _, a := range subArgs {
		if isHelp(a) {
			fmt.Fprint(stdout, usage)
			return 0
		}
	}

	switch sub {
	case "scan":
		fmt.Fprintln(stdout, "portfolio scan: not implemented (stub)")
		_ = subArgs // flags wired in a later task
		return 0
	case "status":
		fmt.Fprintln(stdout, "portfolio status: not implemented (stub)")
		return 0
	default:
		// Unknown subcommand: treat as help-class (exit 0) per spec —
		// surfaces usage rather than a hard error for interactive typos.
		fmt.Fprintf(stdout, "unknown subcommand %q\n", sub)
		fmt.Fprint(stdout, usage)
		return 0
	}
}

func isHelp(s string) bool {
	return s == "-h" || s == "--help"
}
