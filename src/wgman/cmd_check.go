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
		fmt.Fprintf(app.Stderr, "check: %s\n", formatStatus("FAILED", !gf.noColor && app.Sys.IsTerminal(app.Stderr)))
		return 1
	}
	fmt.Fprintf(app.Stdout, "check: %s\n", formatStatus("OK", !gf.noColor && app.Sys.IsTerminal(app.Stdout)))
	return 0
}

func formatStatus(status string, color bool) string {
	if !color {
		return status
	}
	switch status {
	case "OK":
		return colorGreen(status)
	case "FAILED":
		return colorRed(status)
	default:
		return status
	}
}
