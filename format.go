package main

import (
	"fmt"
	"net"
	"time"
)

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

// formatHandshake formats a Unix timestamp as an age relative to now.
// Returns "never" for zero. Otherwise formats as e.g. "2d23h48m40s",
// omitting leading zero components (but always including seconds).
func formatHandshake(ts int64, now time.Time) string {
	if ts == 0 {
		return "never"
	}
	age := now.Unix() - ts
	if age < 0 {
		age = 0
	}
	secs := age % 60
	age /= 60
	mins := age % 60
	age /= 60
	hours := age % 24
	days := age / 24

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

// endpointHost strips the port from a "host:port" endpoint string.
// Returns "(none)" unchanged.
func endpointHost(endpoint string) string {
	if endpoint == "(none)" {
		return "(none)"
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint // already plain host or unparseable
	}
	return host
}
