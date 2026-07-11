package main

import (
	"fmt"
)

// cmdDeploy implements "wgman deploy".
// It calls internal check, refuses on hard errors, reports planned ipset and
// WireGuard peer deltas, optionally prompts for confirmation, and applies the
// changes.
func cmdDeploy(gf *globalFlags, args []string, app *App) int {
	if len(args) > 0 {
		fmt.Fprintln(app.Stderr, "error: deploy takes no positional arguments")
		return 2
	}
	if !app.Sys.IsRoot() {
		fmt.Fprintln(app.Stderr, "error: wgman must be run as root")
		return 1
	}

	cfg, err := LoadConfig(gf.configDir)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	db, err := LoadDB(gf.configDir)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	result := Check(cfg, db, app.Sys)
	if !result.Clean() {
		fmt.Fprintln(app.Stderr, "deploy: hard errors:")
		for _, e := range result.HardErrors {
			fmt.Fprintln(app.Stderr, "  -", e)
		}
		fmt.Fprintln(app.Stderr, "deploy: FAILED (resolve hard errors before deploying)")
		return 1
	}

	changeCount := len(result.IPSetDeltas) + len(result.PeerDeltas)
	if changeCount == 0 {
		fmt.Fprintln(app.Stdout, "deploy: no changes needed")
		return 0
	}

	// Report planned changes.
	fmt.Fprintf(app.Stdout, "deploy: planned changes (%d):\n", changeCount)
	printIPSetDeltas(result.IPSetDeltas, app.Stdout)
	printPeerDeltas(result.PeerDeltas, app.Stdout)

	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "deploy: dry-run, no changes applied")
		return 0
	}

	if !gf.yes {
		if !confirmAction(app.Stdin, app.Stdout, "Apply these changes? [y/N] ") {
			fmt.Fprintln(app.Stdout, "deploy: aborted")
			return 0
		}
	}

	if err := ApplyStateDeltas(cfg.Interface, result.IPSetDeltas, result.PeerDeltas, app.Sys); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintf(app.Stdout, "deploy: applied %d change(s)\n", changeCount)
	return 0
}
