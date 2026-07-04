package main

import (
	"fmt"
)

// cmdInitIPSets implements "wgman init-ipsets".
// It creates the two managed ipsets defined in config.yaml using idempotent
// creation (-exist), so re-running the command on an already-configured host
// is safe.  It does not populate any access entries.
func cmdInitIPSets(gf *globalFlags, args []string, app *App) int {
	if len(args) > 0 {
		fmt.Fprintln(app.Stderr, "error: init-ipsets takes no positional arguments")
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

	// All-access set: one IPv4 address per admin user.
	if err := app.Sys.IPSetCreate(cfg.Sets.All, "hash:ip", false); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(app.Stdout, "ipset %q: created (hash:ip)\n", cfg.Sets.All)

	// Matrix set: pairs of source/destination IPv4 networks with comments.
	if err := app.Sys.IPSetCreate(cfg.Sets.Matrix, "hash:net,net", true); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(app.Stdout, "ipset %q: created (hash:net,net)\n", cfg.Sets.Matrix)

	return 0
}
