package main

import (
	"bufio"
	"fmt"
	"io"
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

// sortedBoolKeys returns the keys of a bool-valued map sorted alphabetically.
func sortedBoolKeys(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
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

// confirmAction prints prompt and returns true only for a trimmed "y" or "Y".
func confirmAction(in io.Reader, out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	scanner := bufio.NewScanner(in)
	answer := ""
	if scanner.Scan() {
		answer = strings.TrimSpace(scanner.Text())
	}
	return answer == "y" || answer == "Y"
}

// isValidIPv4OrCIDR returns true if s is a valid IPv4 address or IPv4 CIDR.
func isValidIPv4OrCIDR(s string) bool {
	if ip := net.ParseIP(s); ip != nil && ip.To4() != nil {
		return true
	}
	_, ipNet, err := net.ParseCIDR(s)
	return err == nil && ipNet.IP.To4() != nil
}

// parseIPv4CIDR parses an IPv4 CIDR string and returns the host IP and network.
func parseIPv4CIDR(subnet string) (net.IP, *net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(subnet)
	if err != nil || ip.To4() == nil {
		return nil, nil, fmt.Errorf("invalid interface subnet %q", subnet)
	}
	ones, bits := ipNet.Mask.Size()
	if ones < 0 || bits != 32 {
		return nil, nil, fmt.Errorf("interface subnet %q is not IPv4", subnet)
	}
	return ip, ipNet, nil
}

// ipv4ToUint32 converts an IPv4 address to its big-endian integer form.
func ipv4ToUint32(ip net.IP) uint32 {
	ip4 := ip.To4()
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

// uint32ToIPv4 converts a big-endian IPv4 integer to dotted decimal form.
func uint32ToIPv4(n uint32) string {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)).String()
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
