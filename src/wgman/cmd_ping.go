package main

import (
	"fmt"
	"io"
)

// cmdPing implements "wgman ping [vm]".
func cmdPing(gf *globalFlags, args []string, app *App) int {
	if len(args) > 1 {
		fmt.Fprintln(app.Stderr, "error: ping takes at most one VM name")
		return 2
	}
	if rejectUnsupportedDryRun("ping", gf, app.Stderr) {
		return 2
	}
	if !app.Sys.IsRoot() {
		fmt.Fprintln(app.Stderr, "error: wgman must be run as root")
		return 1
	}

	db, err := LoadDB(gf.configDir)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	vm := ""
	if len(args) == 1 {
		vm = args[0]
	}
	color := !gf.noColor && app.IsStdoutTTY()
	return runPingWithColor(db, app.Sys, vm, app.Stdout, app.Stderr, color)
}

// runPing is the testable core of "ping".
func runPing(db *DB, sys SystemAdapter, vm string, stdout, stderr io.Writer) int {
	return runPingWithColor(db, sys, vm, stdout, stderr, false)
}

func runPingWithColor(db *DB, sys SystemAdapter, vm string, stdout, stderr io.Writer, color bool) int {
	names := sortedKeys(db.VMs)
	if vm != "" {
		if _, ok := db.VMs[vm]; !ok {
			fmt.Fprintf(stderr, "error: VM %q does not exist\n", vm)
			return 1
		}
		names = []string{vm}
	}

	down := false
	for _, name := range names {
		status := "UP"
		if err := sys.Ping(db.VMs[name]); err != nil {
			status = "DOWN"
			down = true
		}
		if color {
			if status == "UP" {
				status = colorGreen(status)
			} else {
				status = colorRed(status)
			}
		}
		fmt.Fprintf(stdout, "%s: %s\n", name, status)
	}
	if down {
		return 1
	}
	return 0
}
