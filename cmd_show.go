package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type showCell struct {
	plain   string
	display string
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
	color := false
	if !gf.noColor && app.IsStdoutTTY != nil {
		color = app.IsStdoutTTY()
	}
	return runShowWithColor(db, result, app.Now(), app.Stdout, app.Stderr, color)
}

// runShow is the testable core of "show".
// result must be OK() and result.WGDump non-nil; returns 1 on failure.
func runShow(db *DB, result *CheckResult, now time.Time, stdout, stderr io.Writer) int {
	return runShowWithColor(db, result, now, stdout, stderr, false)
}

func runShowWithColor(db *DB, result *CheckResult, now time.Time, stdout, stderr io.Writer, color bool) int {
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

	headers := []string{"NAME", "IP", "ENDPOINT", "RX", "TX", "LAST HANDSHAKE"}
	var rows [][]showCell
	for _, name := range sortedKeys(db.Users) {
		u := db.Users[name]
		peer := nameToPeer[name]
		if peer == nil {
			rows = append(rows, []showCell{
				{plain: name, display: name},
				{plain: u.IP, display: u.IP},
				{plain: "-", display: "-"},
				{plain: "-", display: "-"},
				{plain: "-", display: "-"},
				{plain: "-", display: "-"},
			})
			continue
		}
		rxPlain := formatBytes(peer.RxBytes)
		txPlain := formatBytes(peer.TxBytes)
		handshakePlain := formatHandshake(peer.LatestHandshake, now)
		rows = append(rows, []showCell{
			{plain: name, display: name},
			{plain: u.IP, display: u.IP},
			{plain: endpointHost(peer.Endpoint), display: endpointHost(peer.Endpoint)},
			{plain: rxPlain, display: formatBytesColor(peer.RxBytes, color)},
			{plain: txPlain, display: formatBytesColor(peer.TxBytes, color)},
			{plain: handshakePlain, display: formatHandshakeColor(peer.LatestHandshake, now, color)},
		})
	}
	writeShowTable(stdout, headers, rows)
	fmt.Fprintln(stdout, "show: OK")
	return 0
}

func writeShowTable(w io.Writer, headers []string, rows [][]showCell) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell.plain) > widths[i] {
				widths[i] = len(cell.plain)
			}
		}
	}

	headerCells := make([]showCell, len(headers))
	for i, header := range headers {
		headerCells[i] = showCell{plain: header, display: header}
	}
	writeShowTableRow(w, headerCells, widths)
	for _, row := range rows {
		writeShowTableRow(w, row, widths)
	}
}

func writeShowTableRow(w io.Writer, row []showCell, widths []int) {
	for i, cell := range row {
		if i == len(row)-1 {
			fmt.Fprintln(w, cell.display)
			return
		}
		fmt.Fprint(w, cell.display)
		fmt.Fprint(w, strings.Repeat(" ", widths[i]-len(cell.plain)+2))
	}
}
