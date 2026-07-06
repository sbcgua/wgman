package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ApplyDeltas applies ipset add/delete operations from deltas through sys.
// Operations are applied in order; the first error encountered is returned.
func ApplyDeltas(deltas []IpsetDeltaOp, sys SystemAdapter) error {
	for _, d := range deltas {
		if d.Add {
			if err := sys.IPSetAdd(d.Set, d.Entry, d.Comment); err != nil {
				return err
			}
		} else {
			if err := sys.IPSetDel(d.Set, d.Entry); err != nil {
				return err
			}
		}
	}
	return nil
}

// ApplyDeltasTracked applies deltas and returns the operations that completed
// before any error. Callers can invert those operations for best-effort rollback.
func ApplyDeltasTracked(deltas []IpsetDeltaOp, sys SystemAdapter) ([]IpsetDeltaOp, error) {
	applied := make([]IpsetDeltaOp, 0, len(deltas))
	for _, d := range deltas {
		if d.Add {
			if err := sys.IPSetAdd(d.Set, d.Entry, d.Comment); err != nil {
				return applied, err
			}
		} else {
			if err := sys.IPSetDel(d.Set, d.Entry); err != nil {
				return applied, err
			}
		}
		applied = append(applied, d)
	}
	return applied, nil
}

// InvertDeltas returns inverse operations in reverse order for rollback.
func InvertDeltas(deltas []IpsetDeltaOp) []IpsetDeltaOp {
	inverted := make([]IpsetDeltaOp, 0, len(deltas))
	for i := len(deltas) - 1; i >= 0; i-- {
		d := deltas[i]
		d.Add = !d.Add
		inverted = append(inverted, d)
	}
	return inverted
}

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

// ApplyPeerDeltas applies WireGuard peer operations from deltas through sys.
// Operations are applied in order; the first error encountered is returned.
func ApplyPeerDeltas(iface string, deltas []WGPeerDeltaOp, sys SystemAdapter) error {
	for _, d := range deltas {
		switch {
		case d.Add:
			if err := sys.WGSetPeer(iface, d.PubKey, d.AllowedIP); err != nil {
				return fmt.Errorf("add WireGuard peer for %q: %w", d.User, err)
			}
		case d.Remove:
			if err := sys.WGDelPeer(iface, d.PubKey); err != nil {
				return fmt.Errorf("remove WireGuard peer for %q: %w", d.User, err)
			}
		}
	}
	return nil
}

// ApplyStateDeltas applies WireGuard and ipset operations in a dependency-aware
// order: peer additions, ipset changes, then peer removals.
func ApplyStateDeltas(iface string, ipsetDeltas []IpsetDeltaOp, peerDeltas []WGPeerDeltaOp, sys SystemAdapter) error {
	if err := ApplyPeerDeltas(iface, filterPeerDeltas(peerDeltas, true), sys); err != nil {
		return err
	}
	if err := ApplyDeltas(ipsetDeltas, sys); err != nil {
		return err
	}
	if err := ApplyPeerDeltas(iface, filterPeerDeltas(peerDeltas, false), sys); err != nil {
		return err
	}
	return nil
}

func filterPeerDeltas(deltas []WGPeerDeltaOp, add bool) []WGPeerDeltaOp {
	out := make([]WGPeerDeltaOp, 0, len(deltas))
	for _, d := range deltas {
		if add && d.Add {
			out = append(out, d)
		}
		if !add && d.Remove {
			out = append(out, d)
		}
	}
	return out
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
