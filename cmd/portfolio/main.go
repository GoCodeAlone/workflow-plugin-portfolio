// Command portfolio is the wfctl CLI plugin for the cross-repo portfolio
// catalog generator. It is invoked by wfctl as:
//
//	portfolio --wfctl-cli portfolio <subcommand> [flags...]
//
// The leading --wfctl-cli flag selects CLI-command dispatch mode (vs. the
// go-plugin host protocol used when the workflow engine loads the binary as
// an external plugin). Subcommands:
//
//	portfolio scan <workspace-root> [--taxonomy <path>]
//	    Walk sibling repos, collect git/gh/capability facts, merge with the
//	    existing docs/PORTFOLIO.md (preserving human-curated Status verbatim,
//	    V1), and write docs/PORTFOLIO.md + docs/FOLLOWUPS.md. Pre-write
//	    fail-closed: AssertTargetPrivate aborts if the workspace repo is not
//	    confirmed PRIVATE (V7).
//	portfolio status <workspace-root>
//	    Print an overview of the last catalog (counts by category +
//	    active/stale/abandoned).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GoCodeAlone/workflow-plugin-portfolio/internal/catalog"
	"github.com/GoCodeAlone/workflow-plugin-portfolio/internal/followups"
	"github.com/GoCodeAlone/workflow-plugin-portfolio/internal/scanner"
	"github.com/GoCodeAlone/workflow-plugin-portfolio/internal/visibility"
	"github.com/GoCodeAlone/workflow/capability/inventory"
	sdk "github.com/GoCodeAlone/workflow/plugin/external/sdk"
)

// Version is the build-version string. It defaults to "dev" and is
// ldflag-injected at release time via goreleaser
// (-X main.Version={{.Version}}), then resolved through the workflow SDK so
// the binary reports its release tag (or a "(devel) [@ <sha>]" fallback when
// no ldflag fires). This satisfies the workflow release contract
// (wfctl plugin validate-contract) the same way hybrid go-plugin/CLI plugins
// do — see workflow/plugin/external/sdk/buildversion.go.
var Version = "dev"

// buildVersionOption is the SDK ServeOption a hybrid plugin would pass to
// sdk.Serve. The portfolio plugin is CLI-only (no gRPC host surface), so it
// does not call sdk.Serve, but it still constructs the option via the SDK's
// prescribed wiring (sdk.WithBuildVersion(sdk.ResolveBuildVersion(Version)))
// so the resolved version is computed once and the contract pattern is
// honored. scan reads the resolved value through workflowVersion().
var buildVersionOption = sdk.WithBuildVersion(sdk.ResolveBuildVersion(Version))

const usage = `portfolio — cross-repo portfolio catalog generator

Usage:
  portfolio --wfctl-cli portfolio scan <workspace-root> [--taxonomy <path>]
  portfolio --wfctl-cli portfolio status <workspace-root>

Subcommands:
  portfolio scan <workspace-root>
      Walk sibling git repos, collect git/gh/capability facts, merge with the
      existing docs/PORTFOLIO.md (preserving human-curated status verbatim),
      and write docs/PORTFOLIO.md + docs/FOLLOWUPS.md.
      Pre-write fail-closed: aborts if the workspace repo is not PRIVATE (V7).

  portfolio status <workspace-root>
      Print an overview of the last catalog (counts by category +
      active/stale/abandoned markers).

Flags:
  --taxonomy <path>   Path to capabilities taxonomy.yaml (default:
                      <workspace-root>/workflow/data/capabilities/taxonomy.yaml).
                      If absent, the capability/inventory path is skipped with
                      a warning (degradation, not an error).
  --help, -h          Show this usage.

This binary is invoked by wfctl via capabilities.cliCommands[] in plugin.json.
The leading --wfctl-cli flag is required for CLI dispatch.
`

// defaultTaxonomyRelPath is the canonical location of the taxonomy relative
// to a workspace root (the sibling `workflow` repo hosts it). Used as the
// --taxonomy default when that flag is absent.
const defaultTaxonomyRelPath = "workflow/data/capabilities/taxonomy.yaml"

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch routes wfctl CLI invocations. It returns the process exit code
// rather than calling os.Exit so it is unit-testable.
//
// Argv shape: ["--wfctl-cli", "portfolio", <subcommand>, ...]
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "--wfctl-cli" {
		fmt.Fprintln(stderr, "error: missing leading --wfctl-cli flag (wfctl invokes the binary as: portfolio --wfctl-cli portfolio <subcommand>)")
		fmt.Fprintln(stderr, "")
		fmt.Fprint(stderr, usage)
		return 2
	}
	rest := args[1:]

	if len(rest) > 0 && rest[0] == "portfolio" {
		rest = rest[1:]
	}

	if len(rest) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}

	sub := rest[0]
	subArgs := rest[1:]

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
		return runScanCmd(context.Background(), subArgs, stdout, stderr)
	case "status":
		return runStatusCmd(subArgs, stdout, stderr)
	default:
		fmt.Fprintf(stdout, "unknown subcommand %q\n", sub)
		fmt.Fprint(stdout, usage)
		return 0
	}
}

// runScanCmd parses scan flags and invokes runScan. Supports flags in any
// position (before OR after the workspace-root positional), since the spec
// shows `scan <workspace-root> [--taxonomy <path>]` (flag after positional)
// and Go's stdlib flag package stops at the first non-flag arg.
func runScanCmd(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var taxonomyPath string
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprint(stdout, usage)
			return 0
		case a == "--taxonomy":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "error: --taxonomy requires a value")
				return 2
			}
			taxonomyPath = args[i+1]
			i++ // consume the value
		case strings.HasPrefix(a, "--taxonomy="):
			taxonomyPath = strings.TrimPrefix(a, "--taxonomy=")
		default:
			positionals = append(positionals, a)
		}
	}

	if len(positionals) < 1 {
		fmt.Fprintln(stderr, "error: scan requires a <workspace-root> positional argument")
		fmt.Fprintln(stderr, "")
		fmt.Fprint(stderr, usage)
		return 2
	}
	ws := positionals[0]

	// Default taxonomy derived from the workspace root (the sibling workflow
	// repo hosts it). If the flag was set, honor it verbatim.
	if taxonomyPath == "" {
		taxonomyPath = filepath.Join(ws, defaultTaxonomyRelPath)
	}

	opts := scanOptions{
		workspaceRoot:    ws,
		taxonomyPath:     taxonomyPath,
		workflowVersion:  workflowVersion(),
		visibilityTarget: visibility.DefaultTargetRepo,
	}
	return runScan(ctx, opts, stdout, stderr)
}

// scanOptions configures a scan run. Fields are exported so tests can
// construct opts directly (e.g. skipVisibility for fixture workspaces that
// have no real gh-visible repo).
type scanOptions struct {
	workspaceRoot    string
	taxonomyPath     string
	workflowVersion  string
	visibilityTarget string
	// skipVisibility bypasses AssertTargetPrivate (for e2e fixtures that
	// are not real gh repos). Production invocations leave this false.
	skipVisibility bool
}

// runScan is the testable scan core. Steps:
//  1. AssertTargetPrivate (V7 fail-closed) unless skipped.
//  2. WalkRepos(workspaceRoot).
//  3. Per-repo: GitFacts + GHFacts + registry entry (if plugin + manifest).
//  4. CapabilityInventory (if taxonomy present, else degrade).
//  5. ExtractFromRetros for FOLLOWUPS.md + per-repo FollowUpCount.
//  6. Parse existing docs/PORTFOLIO.md -> Merge (V1) -> write.
func runScan(ctx context.Context, opts scanOptions, stdout, stderr io.Writer) int {
	// V7: fail-closed pre-write visibility assert. Bypassed only by tests
	// (skipVisibility) — production always asserts.
	if !opts.skipVisibility {
		if err := visibility.AssertTargetPrivate(ctx, opts.visibilityTarget); err != nil {
			fmt.Fprintf(stderr, "error: pre-write visibility assert failed (V7 fail-closed, aborting write): %v\n", err)
			return 3
		}
	}

	ws := opts.workspaceRoot
	docsDir := filepath.Join(ws, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "error: create docs dir %s: %v\n", docsDir, err)
		return 4
	}

	// 2. Walk sibling repos.
	repos, err := scanner.WalkRepos(ws, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: walk repos %s: %v\n", ws, err)
		return 5
	}
	fmt.Fprintf(stdout, "scan: discovered %d distinct repo(s) under %s\n", len(repos), ws)

	// Discover the registry tree (sibling workflow-registry) and retros root.
	registryDir := discoverRegistryDir(ws)
	retrosRoot := filepath.Join(ws, "docs", "retros")

	// 3. Per-repo facts.
	scanned := make([]catalog.Project, 0, len(repos))
	for _, r := range repos {
		repoName := deriveRepoName(r.Remote, r.Path)
		proj := catalog.Project{
			Repo:     repoName,
			Category: deriveCategory(repoName, r.Path),
			Scan: catalog.ScanFacts{
				Remote: r.Remote,
				Path:   r.Path,
			},
		}

		// Git facts (last commit + dirty).
		lastISO, uncommitted, gitErr := scanner.GitFacts(r.Path)
		if gitErr != nil {
			fmt.Fprintf(stderr, "warn: git facts %s: %v (skipping git fields)\n", repoName, gitErr)
		} else {
			proj.Scan.LastCommitISO = lastISO
			proj.Scan.Uncommitted = uncommitted
		}

		// GH facts (PRs/issues/release) — degrade on ErrGHUnavailable.
		openPRs, openIssues, latestRel, ghErr := scanner.GHFacts(repoName)
		if ghErr != nil {
			fmt.Fprintf(stderr, "warn: gh facts %s: %v (emitting ?)\n", repoName, ghErr)
			proj.Release = catalog.ReleaseFacts{ReleaseGHAbsent: true}
		} else {
			proj.Release = catalog.ReleaseFacts{
				LatestRelease: latestRel,
				OpenPRs:       openPRs,
				OpenIssues:    openIssues,
			}
		}

		// Registry entry (private + minEngineVersion) if this is a plugin
		// with a manifest in the registry tree.
		if registryDir != "" {
			if manifestPath, ok := findRegistryManifest(registryDir, repoName); ok {
				entry, entryErr := scanner.LoadRegistryEntry(manifestPath)
				if entryErr != nil {
					fmt.Fprintf(stderr, "warn: registry entry %s: %v\n", repoName, entryErr)
				} else {
					proj.Visibility = registryVisibility(entry)
					proj.Provides = "minEngine: " + registryMinEngine(entry)
				}
			}
		}

		scanned = append(scanned, proj)
	}

	// 4. CapabilityInventory (taxonomy-gated; degrade if absent).
	var inv *inventory.Inventory
	if _, statErr := os.Stat(opts.taxonomyPath); statErr != nil {
		fmt.Fprintf(stderr, "warn: taxonomy %s not found (%v) — skipping capability/inventory path (degradation)\n", opts.taxonomyPath, statErr)
	} else {
		inv, err = scanner.CapabilityInventory(registryDir, ws, opts.taxonomyPath, opts.workflowVersion)
		if err != nil {
			fmt.Fprintf(stderr, "warn: capability inventory: %v — skipping Tooling Inventory section (degradation)\n", err)
			inv = nil
		}
	}

	// 5. Follow-ups from retros (⊥ MEMORY.md never read).
	fups, fupErr := followups.ExtractFromRetros(retrosRoot)
	if fupErr != nil {
		fmt.Fprintf(stderr, "warn: follow-ups extract %s: %v\n", retrosRoot, fupErr)
	}
	// Count per-repo for the catalog FollowUpCount field.
	fupCountByRepo := make(map[string]int)
	for _, f := range fups {
		if f.Repo != "" {
			fupCountByRepo[f.Repo]++
		}
	}
	for i := range scanned {
		scanned[i].FollowUpCount = fupCountByRepo[repoBaseName(scanned[i].Repo)]
	}

	// 6. Parse existing PORTFOLIO.md (to preserve curated Status), merge, write.
	portfolioPath := filepath.Join(docsDir, "PORTFOLIO.md")
	existing := parseExistingCatalog(portfolioPath, stderr)
	merged := catalog.Merge(existing, scanned)

	portfolioFile, err := os.Create(portfolioPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: write PORTFOLIO.md: %v\n", err)
		return 6
	}
	if wErr := catalog.WriteCatalog(portfolioFile, merged, inv); wErr != nil {
		portfolioFile.Close()
		fmt.Fprintf(stderr, "error: render PORTFOLIO.md: %v\n", wErr)
		return 7
	}
	portfolioFile.Close()
	fmt.Fprintf(stdout, "scan: wrote %s (%d projects)\n", portfolioPath, len(merged))

	// Write FOLLOWUPS.md.
	if wErr := writeFollowups(filepath.Join(docsDir, "FOLLOWUPS.md"), fups); wErr != nil {
		fmt.Fprintf(stderr, "warn: write FOLLOWUPS.md: %v\n", wErr)
	} else {
		fmt.Fprintf(stdout, "scan: wrote docs/FOLLOWUPS.md (%d follow-ups)\n", len(fups))
	}
	return 0
}

// runStatusCmd parses status args and invokes runStatus. Status takes only
// a workspace-root positional (no flags yet); manual parsing keeps it
// consistent with scan's flag-after-positional tolerance.
func runStatusCmd(args []string, stdout, stderr io.Writer) int {
	var positionals []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprint(stdout, usage)
			return 0
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) < 1 {
		fmt.Fprintln(stderr, "error: status requires a <workspace-root> positional argument")
		fmt.Fprintln(stderr, "")
		fmt.Fprint(stderr, usage)
		return 2
	}
	return runStatus(positionals[0], stdout, stderr)
}

// runStatus prints an overview of the catalog at <ws>/docs/PORTFOLIO.md:
// counts by category and a rough active/stale/abandoned breakdown derived
// from the Status field keywords.
func runStatus(ws string, stdout, stderr io.Writer) int {
	portfolioPath := filepath.Join(ws, "docs", "PORTFOLIO.md")
	data, err := os.ReadFile(portfolioPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: read %s: %v\n", portfolioPath, err)
		return 8
	}
	projects, pErr := catalog.ParseCatalog(strings.NewReader(string(data)))
	if pErr != nil {
		fmt.Fprintf(stderr, "error: parse %s: %v\n", portfolioPath, pErr)
		return 9
	}
	fmt.Fprintf(stdout, "portfolio status: %d project(s)\n\n", len(projects))

	// Counts by category.
	byCategory := make(map[string]int)
	for _, p := range projects {
		cat := p.Category
		if cat == "" {
			cat = "uncategorized"
		}
		byCategory[cat]++
	}
	cats := make([]string, 0, len(byCategory))
	for c := range byCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	fmt.Fprintln(stdout, "By category:")
	for _, c := range cats {
		fmt.Fprintf(stdout, "  %-16s %d\n", c, byCategory[c])
	}

	// Active/stale/abandoned breakdown from Status keywords.
	var active, stale, abandoned int
	for _, p := range projects {
		s := strings.ToLower(p.Status)
		switch {
		case strings.Contains(s, "abandon"):
			abandoned++
		case strings.Contains(s, "stale") || strings.Contains(s, "stale"):
			stale++
		case strings.Contains(s, "active") || strings.Contains(s, "shipped") || strings.Contains(s, "released"):
			active++
		}
	}
	fmt.Fprintln(stdout, "\nStatus breakdown (keyword-derived):")
	fmt.Fprintf(stdout, "  active:    %d\n", active)
	fmt.Fprintf(stdout, "  stale:     %d\n", stale)
	fmt.Fprintf(stdout, "  abandoned: %d\n", abandoned)
	fmt.Fprintf(stdout, "  (uncategorized: %d)\n", len(projects)-active-stale-abandoned)
	return 0
}

// parseExistingCatalog reads an existing PORTFOLIO.md and returns its
// projects (for Status preservation via Merge). A missing file is not an
// error (returns nil projects).
func parseExistingCatalog(path string, stderr io.Writer) []catalog.Project {
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "warn: open existing %s: %v\n", path, err)
		}
		return nil
	}
	defer f.Close()
	projects, err := catalog.ParseCatalog(f)
	if err != nil {
		fmt.Fprintf(stderr, "warn: parse existing %s: %v (Status preservation may be incomplete)\n", path, err)
		return nil
	}
	return projects
}

// discoverRegistryDir locates the workflow-registry tree under ws. Returns
// the path to the registry root (or its plugins/ subdir) if present, else "".
func discoverRegistryDir(ws string) string {
	candidates := []string{
		filepath.Join(ws, "workflow-registry"),
		filepath.Join(ws, "workflow-cloud-registry"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return ""
}

// findRegistryManifest locates the manifest.json for repoName (base name)
// within the registry plugins tree. Returns the path and true if found.
func findRegistryManifest(registryDir, repoName string) (string, bool) {
	base := repoBaseName(repoName)
	// Registry layout: <registry>/plugins/<name>/manifest.json, OR
	// <registry>/<name>/manifest.json (CollectEcosystem handles both; we
	// check both here too).
	for _, rel := range []string{
		filepath.Join("plugins", base, "manifest.json"),
		filepath.Join(base, "manifest.json"),
	} {
		p := filepath.Join(registryDir, rel)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// deriveRepoName derives the owner/name from a remote URL or falls back to
// the on-disk dir name. Used as the Merge match key.
func deriveRepoName(remote, path string) string {
	remote = strings.TrimSpace(remote)
	if remote != "" {
		// Strip scheme + .git suffix; keep owner/name.
		u := remote
		u = strings.TrimPrefix(u, "https://")
		u = strings.TrimPrefix(u, "git@github.com:")
		u = strings.TrimPrefix(u, "ssh://git@github.com/")
		u = strings.TrimSuffix(u, ".git")
		// u is now "owner/name".
		if parts := strings.Split(u, "/"); len(parts) >= 2 {
			return strings.Join(parts[len(parts)-2:], "/")
		}
		return u
	}
	// Fall back to the dir name.
	return filepath.Base(path)
}

// deriveCategory classifies a repo by name/path for the status overview.
func deriveCategory(repoName, path string) string {
	base := repoBaseName(repoName)
	switch {
	case strings.HasPrefix(base, "workflow-plugin-"):
		return "plugin"
	case base == "workflow":
		return "engine"
	case strings.Contains(base, "registry"):
		return "registry"
	case strings.HasPrefix(base, "gocodealone-"):
		return "app"
	case strings.Contains(base, "docs") || strings.Contains(base, "blog"):
		return "docs"
	default:
		return "other"
	}
}

// repoBaseName returns the last path segment of an owner/name (the repo
// name without owner).
func repoBaseName(repoName string) string {
	if idx := strings.LastIndex(repoName, "/"); idx >= 0 {
		return repoName[idx+1:]
	}
	return repoName
}

// registryVisibility renders the Private *bool as a display string.
// D16: nil (absent) -> "?" (never assume public).
func registryVisibility(entry scanner.RegistryEntry) string {
	if entry.Private == nil {
		return "?"
	}
	if *entry.Private {
		return "PRIVATE"
	}
	return "PUBLIC"
}

// registryMinEngine renders minEngineVersion with "?" for absent.
func registryMinEngine(entry scanner.RegistryEntry) string {
	if entry.MinEngineVersion == "" {
		return "?"
	}
	return entry.MinEngineVersion
}

// writeFollowups renders the extracted follow-ups to FOLLOWUPS.md.
func writeFollowups(path string, fups []followups.FollowUp) error {
	var b strings.Builder
	b.WriteString("# Open Follow-ups\n\n")
	b.WriteString("<!-- Extracted from docs/retros/**/*.md by `wfctl portfolio scan`.\n")
	b.WriteString("     MEMORY.md is NEVER read. Regenerated each scan. -->\n\n")
	if len(fups) == 0 {
		b.WriteString("_No open follow-ups found in retros._\n")
	} else {
		// Group by repo for readability.
		byRepo := make(map[string][]followups.FollowUp)
		var repos []string
		for _, f := range fups {
			r := f.Repo
			if r == "" {
				r = "(standalone)"
			}
			if _, ok := byRepo[r]; !ok {
				repos = append(repos, r)
			}
			byRepo[r] = append(byRepo[r], f)
		}
		sort.Strings(repos)
		for _, r := range repos {
			fmt.Fprintf(&b, "## %s\n\n", r)
			for _, f := range byRepo[r] {
				fmt.Fprintf(&b, "- %s\n", f.Text)
				fmt.Fprintf(&b, "  - source: `%s`\n", f.SourcePath)
			}
			b.WriteString("\n")
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// workflowVersion returns the resolved build-version string for the scan
// inventory metadata. It is the value the SDK computed from the ldflag-
// injected Version var (buildVersionOption carries it; we read it back via
// sdk.ResolveBuildVersion so the CLI path and any future Serve wiring share
// one source of truth).
func workflowVersion() string {
	_ = buildVersionOption // ensure the SDK-wired option is referenced
	return sdk.ResolveBuildVersion(Version)
}

func isHelp(s string) bool {
	return s == "-h" || s == "--help"
}
