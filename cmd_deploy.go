package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// printDeltas writes a human-readable summary of planned ipset operations to w.
func printDeltas(deltas []IpsetDeltaOp, w io.Writer) {
	for _, d := range deltas {
		if d.Add {
			if d.Comment != "" {
				fmt.Fprintf(w, "  add %s %s  # %s\n", d.Set, d.Entry, d.Comment)
			} else {
				fmt.Fprintf(w, "  add %s %s\n", d.Set, d.Entry)
			}
		} else {
			fmt.Fprintf(w, "  del %s %s\n", d.Set, d.Entry)
		}
	}
}

// printPeerDeltas writes a human-readable summary of planned WireGuard peer
// operations to w.
func printPeerDeltas(deltas []WGPeerDeltaOp, w io.Writer) {
	for _, d := range deltas {
		switch {
		case d.Add:
			fmt.Fprintf(w, "  add wg peer %s %s %s\n", d.User, d.PubKey, d.AllowedIP)
		case d.Remove:
			fmt.Fprintf(w, "  remove wg peer %s %s\n", d.User, d.PubKey)
		}
	}
}

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

	changeCount := len(result.Deltas) + len(result.PeerDeltas)
	if changeCount == 0 {
		fmt.Fprintln(app.Stdout, "deploy: no changes needed")
		return 0
	}

	// Report planned changes.
	fmt.Fprintf(app.Stdout, "deploy: planned changes (%d):\n", changeCount)
	printDeltas(result.Deltas, app.Stdout)
	printPeerDeltas(result.PeerDeltas, app.Stdout)

	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "deploy: dry-run, no changes applied")
		return 0
	}

	if !gf.yes {
		fmt.Fprint(app.Stdout, "Apply these changes? [y/N] ")
		scanner := bufio.NewScanner(app.Stdin)
		answer := ""
		if scanner.Scan() {
			answer = strings.TrimSpace(scanner.Text())
		}
		if answer != "y" && answer != "Y" {
			fmt.Fprintln(app.Stdout, "deploy: aborted")
			return 0
		}
	}

	if err := ApplyStateDeltas(cfg.Interface, result.Deltas, result.PeerDeltas, app.Sys); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintf(app.Stdout, "deploy: applied %d change(s)\n", changeCount)
	return 0
}
