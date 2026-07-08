package main

import (
	"net"
	"sort"
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
