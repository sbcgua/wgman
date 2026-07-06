package main

import (
	"fmt"
	"io"
)

// cmdCheck implements "wgman check".
func cmdCheck(gf *globalFlags, args []string, app *App) int {
	if len(args) > 0 {
		fmt.Fprintln(app.Stderr, "error: check takes no positional arguments")
		return 2
	}
	if rejectUnsupportedDryRun("check", gf, app.Stderr) {
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
	printCheckErrors(result, app.Stderr)
	if !result.OK() {
		fmt.Fprintln(app.Stderr, "check: FAILED")
		return 1
	}
	fmt.Fprintln(app.Stdout, "check: OK")
	return 0
}

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
			switch {
			case d.Add:
				fmt.Fprintf(w, "  - active user %q peer is absent and can be added\n", d.User)
			case d.Remove:
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
