package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// WGDumpResult holds the parsed output of "wg show <iface> dump".
type WGDumpResult struct {
	ServerPubKey string
	Peers        []WGPeer
}

// WGPeer is one parsed peer line from "wg show dump".
type WGPeer struct {
	PublicKey       string
	PresharedKey    string // "(none)" if not set
	Endpoint        string // raw value including port, or "(none)"
	AllowedIP       string // normalized plain IPv4 — /32 stripped
	LatestHandshake int64  // Unix timestamp; 0 means never
	RxBytes         int64
	TxBytes         int64
	Keepalive       string // "off" or seconds value
}

// ParseWGDump parses the output of "wg show <iface> dump".
// Line 1 (server): private-key\tpublic-key\tlisten-port\tfwmark
// Lines 2+  (peers): public-key\tpreshared-key\tendpoint\tallowed-ips\tlatest-handshake\ttransfer-rx\ttransfer-tx\tpersistent-keepalive
func ParseWGDump(output string) (*WGDumpResult, error) {
	lines := splitLines(output)
	if len(lines) == 0 {
		return nil, fmt.Errorf("wg dump: empty output")
	}

	serverFields := strings.Split(lines[0], "\t")
	if len(serverFields) < 4 {
		return nil, fmt.Errorf("wg dump: server line has %d fields, want >=4: %q", len(serverFields), lines[0])
	}

	result := &WGDumpResult{
		ServerPubKey: serverFields[1],
	}

	for i, line := range lines[1:] {
		peer, err := parseWGPeerLine(line)
		if err != nil {
			return nil, fmt.Errorf("wg dump: peer line %d: %w", i+2, err)
		}
		result.Peers = append(result.Peers, *peer)
	}

	return result, nil
}

func parseWGPeerLine(line string) (*WGPeer, error) {
	// public-key\tpreshared-key\tendpoint\tallowed-ips\tlatest-handshake\ttransfer-rx\ttransfer-tx\tpersistent-keepalive
	fields := strings.Split(line, "\t")
	if len(fields) < 8 {
		return nil, fmt.Errorf("peer line has %d fields, want >=8: %q", len(fields), line)
	}

	latestHandshake, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse latest-handshake %q: %w", fields[4], err)
	}

	rxBytes, err := strconv.ParseInt(fields[5], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse transfer-rx %q: %w", fields[5], err)
	}

	txBytes, err := strconv.ParseInt(fields[6], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse transfer-tx %q: %w", fields[6], err)
	}

	allowedIP, err := normalizeWGAllowedIP(fields[3])
	if err != nil {
		return nil, fmt.Errorf("normalize allowed-ip %q: %w", fields[3], err)
	}

	return &WGPeer{
		PublicKey:       fields[0],
		PresharedKey:    fields[1],
		Endpoint:        fields[2],
		AllowedIP:       allowedIP,
		LatestHandshake: latestHandshake,
		RxBytes:         rxBytes,
		TxBytes:         txBytes,
		Keepalive:       fields[7],
	}, nil
}

// normalizeWGAllowedIP converts "10.8.0.5/32" → "10.8.0.5".
// Non-/32 CIDRs are returned as-is.
// Multiple comma-separated IPs are rejected — wgman manages single-IP peers.
func normalizeWGAllowedIP(s string) (string, error) {
	if strings.Contains(s, ",") {
		return "", fmt.Errorf("unexpected multiple allowed-ips %q", s)
	}
	if !strings.Contains(s, "/") {
		if net.ParseIP(s) == nil {
			return "", fmt.Errorf("invalid allowed-ip %q", s)
		}
		return s, nil
	}
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return "", fmt.Errorf("invalid allowed-ip CIDR %q: %w", s, err)
	}
	ones, bits := ipNet.Mask.Size()
	if ip.To4() != nil && ones == 32 && bits == 32 {
		return ip.String(), nil
	}
	return s, nil
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
