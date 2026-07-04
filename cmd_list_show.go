package main

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"
)

// cmdList implements "wgman list [filter]".
func cmdList(gf *globalFlags, args []string, app *App) int {
	if len(args) > 1 {
		fmt.Fprintln(app.Stderr, "error: list takes at most one positional argument")
		return 2
	}
	if rejectUnsupportedDryRun("list", gf, app.Stderr) {
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

	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}
	return runList(db, result, filter, app.Stdout, app.Stderr)
}

// runList is the testable core of "list".
// result must be OK() for output to be written; returns 1 on failure.
func runList(db *DB, result *CheckResult, filter string, stdout, stderr io.Writer) int {
	if !result.OK() {
		printCheckErrors(result, stderr)
		fmt.Fprintln(stderr, "list: FAILED")
		return 1
	}

	if filter != "" {
		return runListUser(db, filter, stdout, stderr)
	}

	fmt.Fprintln(stdout, "Users:")
	for _, name := range sortedKeys(db.Users) {
		fmt.Fprintf(stdout, "  %-20s %s\n", name, db.Users[name].IP)
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "VMs:")
	for _, name := range sortedKeys(db.VMs) {
		fmt.Fprintf(stdout, "  %-20s %s\n", name, db.VMs[name])
	}

	fmt.Fprintln(stdout, "list: OK")
	return 0
}

// runListUser prints the access list for a single named user.
func runListUser(db *DB, filter string, stdout, stderr io.Writer) int {
	if _, ok := db.Users[filter]; !ok {
		fmt.Fprintf(stderr, "error: user %q not found\n", filter)
		fmt.Fprintln(stderr, "list: FAILED")
		return 1
	}

	vms := db.Access[filter]
	fmt.Fprintf(stdout, "%s:\n", filter)
	if len(vms) == 0 {
		fmt.Fprintln(stdout, "  (none)")
		fmt.Fprintln(stdout, "list: OK")
		return 0
	}

	sorted := make([]string, len(vms))
	copy(sorted, vms)
	sort.Strings(sorted)
	for _, vm := range sorted {
		fmt.Fprintf(stdout, "  %s\n", vm)
	}
	fmt.Fprintln(stdout, "list: OK")
	return 0
}

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

	// Build pubkey → username and name → peer lookup maps.
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

// sortedKeys returns the keys of a string-keyed map sorted alphabetically.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
