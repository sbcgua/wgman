package main

import (
	"fmt"
	"io"
	"strings"
)

// printCheckErrors writes hard errors, WireGuard drift, and ipset drift from
// result to w.
func printCheckErrors(result *CheckResult, w io.Writer) {
	if len(result.HardErrors) > 0 {
		fmt.Fprintln(w, "check: hard errors:")
		for _, e := range result.HardErrors {
			fmt.Fprintln(w, "  -", e)
		}
	}
	if len(result.PeerDeltas) > 0 {
		fmt.Fprintln(w, "check: WireGuard drift:")
		for _, d := range result.PeerDeltas {
			switch d.Action {
			case WGPeerAdd:
				fmt.Fprintf(w, "  - active user %q peer is absent and can be added\n", d.User)
			case WGPeerRemove:
				fmt.Fprintf(w, "  - inactive user %q peer is present and can be removed\n", d.User)
			}
		}
	}
	if len(result.Drift) > 0 {
		fmt.Fprintln(w, "check: ipset drift:")
		for _, d := range result.Drift {
			fmt.Fprintln(w, "  -", d)
		}
	}
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

// printPeerDeltas writes a human-readable summary of planned WireGuard peer
// operations to w.
func printPeerDeltas(deltas []WGPeerDeltaOp, w io.Writer) {
	for _, d := range deltas {
		switch d.Action {
		case WGPeerAdd:
			fmt.Fprintf(w, "  add wg peer %s %s %s\n", d.User, d.PubKey, d.AllowedIP)
		case WGPeerRemove:
			fmt.Fprintf(w, "  remove wg peer %s %s\n", d.User, d.PubKey)
		}
	}
}

// printApplyAndRollbackError writes the primary failure and, when present,
// includes rollback failure details on the same error line.
func printApplyAndRollbackError(w io.Writer, err, rollbackErr error) {
	if rollbackErr != nil {
		fmt.Fprintf(w, "error: %v (rollback failed: %v)\n", err, rollbackErr)
		return
	}
	fmt.Fprintln(w, "error:", err)
}

// printRemovePlan writes the confirmation summary for a planned user removal.
func printRemovePlan(plan *removePlan, w io.Writer) {
	fmt.Fprintf(w, "remove: planned removal of %s (%s)\n", plan.User, plan.IP)
	if len(plan.Access) == 0 {
		fmt.Fprintln(w, "remove: access: (none)")
	} else {
		fmt.Fprintf(w, "remove: access: %s\n", strings.Join(plan.Access, ", "))
	}
	if len(plan.IPSetDeltas) > 0 {
		fmt.Fprintf(w, "remove: planned access changes (%d):\n", len(plan.IPSetDeltas))
		printDeltas(plan.IPSetDeltas, w)
	}
}
