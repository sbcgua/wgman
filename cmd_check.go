package main

import (
	"fmt"
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
