package main

import (
	"net"
	"sort"
	"strings"
)

// sortedKeys returns the keys of a string-keyed map sorted alphabetically.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// splitLines splits output into non-empty lines, stripping CR.
func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// isValidIPv4OrCIDR returns true if s is a valid IPv4 address or IPv4 CIDR.
func isValidIPv4OrCIDR(s string) bool {
	if ip := net.ParseIP(s); ip != nil && ip.To4() != nil {
		return true
	}
	_, ipNet, err := net.ParseCIDR(s)
	return err == nil && ipNet.IP.To4() != nil
}

// caseFold lowercases ASCII letters for case-insensitive comparison.
func caseFold(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
