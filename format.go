package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	ansiReset   = "\x1b[0m"
	ansiGrey    = "\x1b[90m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiCyan    = "\x1b[36m"
	ansiPink    = "\x1b[95m"
	ansiDimCyan = "\x1b[2;36m"
)

func colorGrey(s string) string {
	return ansiGrey + s + ansiReset
}

func colorRed(s string) string {
	return ansiRed + s + ansiReset
}

func colorGreen(s string) string {
	return ansiGreen + s + ansiReset
}

func colorCyan(s string) string {
	return ansiCyan + s + ansiReset
}

func colorPink(s string) string {
	return ansiPink + s + ansiReset
}

func colorDimCyan(s string) string {
	return ansiDimCyan + s + ansiReset
}

type tableCell struct {
	plain   string
	display string
}

func writeTable(w io.Writer, headers []string, rows [][]tableCell) {
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

	headerCells := make([]tableCell, len(headers))
	for i, header := range headers {
		headerCells[i] = tableCell{plain: header, display: header}
	}
	writeTableRow(w, headerCells, widths)
	for _, row := range rows {
		writeTableRow(w, row, widths)
	}
}

func writeTableRow(w io.Writer, row []tableCell, widths []int) {
	for i, cell := range row {
		if i == len(row)-1 {
			fmt.Fprintln(w, cell.display)
			return
		}
		fmt.Fprint(w, cell.display)
		fmt.Fprint(w, strings.Repeat(" ", widths[i]-len(cell.plain)+2))
	}
}

// formatBytes converts a byte count to a compact human-readable string.
// Format follows the spec example: "2.07Mb".
func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n < kb:
		return fmt.Sprintf("%dB", n)
	case n < mb:
		return fmt.Sprintf("%.2fKb", float64(n)/kb)
	case n < gb:
		return fmt.Sprintf("%.2fMb", float64(n)/mb)
	default:
		return fmt.Sprintf("%.2fGb", float64(n)/gb)
	}
}

func formatBytesColor(n int64, enabled bool) string {
	plain := formatBytes(n)
	if !enabled {
		return plain
	}
	if n == 0 {
		return colorGrey(plain)
	}
	unitStart := 0
	for unitStart < len(plain) && ((plain[unitStart] >= '0' && plain[unitStart] <= '9') || plain[unitStart] == '.') {
		unitStart++
	}
	if unitStart == len(plain) {
		return plain
	}
	return plain[:unitStart] + colorDimCyan(plain[unitStart:])
}

// formatHandshake formats a Unix timestamp as an age relative to now.
// Returns "never" for zero. Otherwise formats as e.g. "2d23h48m40s",
// omitting leading zero components (but always including seconds).
func formatHandshake(ts int64, now time.Time) string {
	if ts == 0 {
		return "never"
	}
	days, hours, mins, secs := handshakeAgeParts(ts, now)

	switch {
	case days > 0:
		return fmt.Sprintf("%dd%dh%dm%ds", days, hours, mins, secs)
	case hours > 0:
		return fmt.Sprintf("%dh%dm%ds", hours, mins, secs)
	case mins > 0:
		return fmt.Sprintf("%dm%ds", mins, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

func formatHandshakeColor(ts int64, now time.Time, enabled bool) string {
	if !enabled {
		return formatHandshake(ts, now)
	}
	if ts == 0 {
		return colorGrey("never")
	}
	days, hours, mins, secs := handshakeAgeParts(ts, now)

	switch {
	case days > 0:
		return fmt.Sprintf("%s%dh%s%ds", colorDimCyan(fmt.Sprintf("%dd", days)), hours, colorDimCyan(fmt.Sprintf("%dm", mins)), secs)
	case hours > 0:
		return fmt.Sprintf("%dh%s%ds", hours, colorDimCyan(fmt.Sprintf("%dm", mins)), secs)
	case mins > 0:
		return fmt.Sprintf("%s%ds", colorDimCyan(fmt.Sprintf("%dm", mins)), secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

func handshakeAgeParts(ts int64, now time.Time) (days, hours, mins, secs int64) {
	age := now.Unix() - ts
	if age < 0 {
		age = 0
	}
	secs = age % 60
	age /= 60
	mins = age % 60
	age /= 60
	hours = age % 24
	days = age / 24
	return days, hours, mins, secs
}
