package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// cmdShow implements "wgman show".
func cmdShow(gf *globalFlags, args []string, app *App) int {
	if len(args) > 0 {
		fmt.Fprintln(app.Stderr, "error: show takes no positional arguments")
		return 2
	}
	if rejectUnsupportedDryRun("show", gf, app.Stderr) {
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
	return runShow(db, result, app.Now(), app.Stdout, app.Stderr)
}

// runShow is the testable core of "show".
// result must be OK() and result.WGDump non-nil; returns 1 on failure.
func runShow(db *DB, result *CheckResult, now time.Time, stdout, stderr io.Writer) int {
	if !result.OK() {
		printCheckErrors(result, stderr)
		fmt.Fprintln(stderr, "show: FAILED")
		return 1
	}
	if result.WGDump == nil {
		fmt.Fprintln(stderr, "show: no WireGuard data available")
		return 1
	}

	dump := result.WGDump

	pubToUser := make(map[string]string, len(db.Users))
	for name, u := range db.Users {
		pubToUser[u.Pub] = name
	}
	nameToPeer := make(map[string]*WGPeer, len(dump.Peers))
	for i := range dump.Peers {
		if name := pubToUser[dump.Peers[i].PublicKey]; name != "" {
			nameToPeer[name] = &dump.Peers[i]
		}
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tIP\tENDPOINT\tRX\tTX\tLAST HANDSHAKE")

	for _, name := range sortedKeys(db.Users) {
		u := db.Users[name]
		peer := nameToPeer[name]
		if peer == nil {
			fmt.Fprintf(tw, "%s\t%s\t-\t-\t-\t-\n", name, u.IP)
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			name,
			u.IP,
			endpointHost(peer.Endpoint),
			formatBytes(peer.RxBytes),
			formatBytes(peer.TxBytes),
			formatHandshake(peer.LatestHandshake, now),
		)
	}
	tw.Flush()
	fmt.Fprintln(stdout, "show: OK")
	return 0
}
