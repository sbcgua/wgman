package main

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// Check performs the full validation: offline config/db checks followed by
// live WireGuard and ipset state verification via sys.
//
// HardErrors are issues that make the state unsafe or ambiguous.
// Drift is ipset discrepancy between db.yaml and live ipsets.
// Deltas are the concrete ipset operations needed to reconcile drift.
func Check(cfg *Config, db *DB, sys SystemAdapter) *CheckResult {
	result := &CheckResult{}
	result.HardErrors = append(result.HardErrors, validateDB(db)...)
	if !result.Clean() {
		sort.Strings(result.HardErrors)
		return result
	}

	// --- Interface subnet ---
	subnetStr, err := sys.InterfaceSubnet(cfg.Interface)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("get interface subnet for %q: %s", cfg.Interface, err))
		return result
	}
	_, ipNet, err := net.ParseCIDR(subnetStr)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("parse interface subnet %q: %s", subnetStr, err))
		return result
	}

	// Validate all user IPs are within the interface subnet.
	for name, u := range db.Users {
		ip := net.ParseIP(u.IP)
		if ip == nil {
			continue // already caught by validateDB
		}
		if !ipNet.Contains(ip) {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("user %q ip %s is outside interface subnet %s", name, u.IP, subnetStr))
		}
	}

	// --- WireGuard peer validation ---
	wgRaw, err := sys.WGDump(cfg.Interface)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("wg dump %q: %s", cfg.Interface, err))
		return result
	}
	wgDump, err := ParseWGDump(wgRaw)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("parse wg dump: %s", err))
		return result
	}

	// Build pubkey lookups.
	result.WGDump = wgDump

	wgByPub := make(map[string]*WGPeer, len(wgDump.Peers))
	for i := range wgDump.Peers {
		wgByPub[wgDump.Peers[i].PublicKey] = &wgDump.Peers[i]
	}
	dbByPub := make(map[string]string, len(db.Users)) // pubkey → username
	for name, u := range db.Users {
		dbByPub[u.Pub] = name
	}

	// Every active db user should have a matching WG peer with the correct IP.
	// Missing active peers and present inactive peers are safe drift that deploy
	// can reconcile from db.yaml.
	for name, u := range db.Users {
		peer, ok := wgByPub[u.Pub]
		if u.Inactive {
			if ok {
				result.PeerDeltas = append(result.PeerDeltas, WGPeerDeltaOp{
					User:   name,
					PubKey: u.Pub,
					Remove: true,
				})
			}
			continue
		}
		if !ok {
			result.PeerDeltas = append(result.PeerDeltas, WGPeerDeltaOp{
				User:      name,
				PubKey:    u.Pub,
				AllowedIP: u.IP,
				Add:       true,
			})
			continue
		}
		if peer.AllowedIP != u.IP {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("user %q: db ip %s does not match WireGuard allowed-ip %s",
					name, u.IP, peer.AllowedIP))
		}
	}

	// Every WG peer must correspond to a db user.
	for _, peer := range wgDump.Peers {
		if _, ok := dbByPub[peer.PublicKey]; !ok {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("WireGuard peer %s not found in db users", peer.PublicKey))
		}
	}

	if !result.Clean() {
		sort.Strings(result.HardErrors)
		return result
	}

	// --- ipset drift detection ---
	allExpected, matrixExpected := computeExpectedIPSets(db)

	// All-access set.
	allRaw, err := sys.IPSetList(cfg.Sets.All)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("ipset list %q: %s (run 'wgman init-ipsets' to create managed sets)", cfg.Sets.All, err))
	} else {
		allParsed, err := ParseIPSet(allRaw)
		if err != nil {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("parse ipset %q: %s", cfg.Sets.All, err))
		} else if errs := validateAllAccessIPSet(cfg.Sets.All, allParsed); len(errs) > 0 {
			result.HardErrors = append(result.HardErrors, errs...)
		} else {
			reconcileIPSet(cfg.Sets.All, allExpected, allParsed.Entries, result)
		}
	}

	// Matrix set.
	matrixRaw, err := sys.IPSetList(cfg.Sets.Matrix)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("ipset list %q: %s (run 'wgman init-ipsets' to create managed sets)", cfg.Sets.Matrix, err))
	} else {
		matrixParsed, err := ParseIPSet(matrixRaw)
		if err != nil {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("parse ipset %q: %s", cfg.Sets.Matrix, err))
		} else if errs := validateMatrixIPSet(cfg.Sets.Matrix, matrixParsed); len(errs) > 0 {
			result.HardErrors = append(result.HardErrors, errs...)
		} else {
			reconcileIPSet(cfg.Sets.Matrix, matrixExpected, matrixParsed.Entries, result)
		}
	}

	sort.Strings(result.HardErrors)
	sort.Strings(result.Drift)
	sort.Slice(result.Deltas, func(i, j int) bool {
		if result.Deltas[i].Set != result.Deltas[j].Set {
			return result.Deltas[i].Set < result.Deltas[j].Set
		}
		return result.Deltas[i].Entry < result.Deltas[j].Entry
	})
	sort.Slice(result.PeerDeltas, func(i, j int) bool {
		if result.PeerDeltas[i].User != result.PeerDeltas[j].User {
			return result.PeerDeltas[i].User < result.PeerDeltas[j].User
		}
		return result.PeerDeltas[i].PubKey < result.PeerDeltas[j].PubKey
	})

	return result
}

// computeExpectedIPSets derives the desired ipset state from db.yaml.
// Returns allExpected (entry→comment) for the all-access set and
// matrixExpected (entry→comment) for the matrix set.
func computeExpectedIPSets(db *DB) (allExpected, matrixExpected map[string]string) {
	allExpected = make(map[string]string)
	matrixExpected = make(map[string]string)

	for user, vms := range db.Access {
		u, ok := db.Users[user]
		if !ok || u.Inactive {
			continue
		}
		for _, vm := range vms {
			if vm == "*" {
				allExpected[u.IP] = ""
			} else {
				vmIP, ok := db.VMs[vm]
				if !ok {
					continue
				}
				entry := u.IP + "," + vmIP
				matrixExpected[entry] = user + " -> " + vm
			}
		}
	}
	return
}

// reconcileIPSet computes drift and required deltas for one ipset.
// expected maps entry → comment (the desired state from db.yaml).
// live is the current content of the ipset.
func reconcileIPSet(setname string, expected map[string]string, live []IPSetEntry, result *CheckResult) {
	liveSet := make(map[string]bool, len(live))
	for _, e := range live {
		liveSet[e.Entry] = true
	}

	// Missing entries need to be added.
	for entry, comment := range expected {
		if !liveSet[entry] {
			result.Drift = append(result.Drift,
				fmt.Sprintf("ipset %s: missing entry %s", setname, entry))
			result.Deltas = append(result.Deltas, IpsetDeltaOp{
				Set:     setname,
				Entry:   entry,
				Comment: comment,
				Add:     true,
			})
		}
	}

	// Extra entries need to be deleted.
	for _, e := range live {
		if _, ok := expected[e.Entry]; !ok {
			result.Drift = append(result.Drift,
				fmt.Sprintf("ipset %s: unexpected entry %s", setname, e.Entry))
			result.Deltas = append(result.Deltas, IpsetDeltaOp{
				Set:   setname,
				Entry: e.Entry,
				Add:   false,
			})
		}
	}
}

// validateAllAccessIPSet checks that the live all-access set has the correct
// type (hash:ip) and that all entries are plain IPv4 addresses.
// Returns hard error strings; an empty slice means validation passed.
func validateAllAccessIPSet(setname string, parsed *ParsedIPSet) []string {
	var errs []string
	if parsed.SetName != setname {
		errs = append(errs, fmt.Sprintf(
			"ipset %q output describes set %q, expected %q",
			setname, parsed.SetName, setname))
		return errs
	}
	if parsed.SetType != "hash:ip" {
		errs = append(errs, fmt.Sprintf(
			"ipset %q has type %q, expected hash:ip (run 'wgman init-ipsets' to recreate)",
			setname, parsed.SetType))
		return errs // can't trust entries if type is wrong
	}
	for _, e := range parsed.Entries {
		ip := net.ParseIP(e.Entry)
		if ip == nil || ip.To4() == nil {
			errs = append(errs, fmt.Sprintf(
				"ipset %q: all-access entry %q is not a valid IPv4 address", setname, e.Entry))
		}
	}
	return errs
}

// validateMatrixIPSet checks that the live matrix set has the correct type
// (hash:net,net) and that all entries are two comma-separated IPv4/net values.
// Returns hard error strings; an empty slice means validation passed.
func validateMatrixIPSet(setname string, parsed *ParsedIPSet) []string {
	var errs []string
	if parsed.SetName != setname {
		errs = append(errs, fmt.Sprintf(
			"ipset %q output describes set %q, expected %q",
			setname, parsed.SetName, setname))
		return errs
	}
	if parsed.SetType != "hash:net,net" {
		errs = append(errs, fmt.Sprintf(
			"ipset %q has type %q, expected hash:net,net (run 'wgman init-ipsets' to recreate)",
			setname, parsed.SetType))
		return errs // can't trust entries if type is wrong
	}
	for _, e := range parsed.Entries {
		parts := strings.SplitN(e.Entry, ",", 2)
		if len(parts) != 2 || !isValidIPv4OrCIDR(parts[0]) || !isValidIPv4OrCIDR(parts[1]) {
			errs = append(errs, fmt.Sprintf(
				"ipset %q: matrix entry %q is not two valid IPv4/net values", setname, e.Entry))
		}
	}
	return errs
}

// isValidIPv4OrCIDR returns true if s is a valid IPv4 address or IPv4 CIDR.
func isValidIPv4OrCIDR(s string) bool {
	if ip := net.ParseIP(s); ip != nil && ip.To4() != nil {
		return true
	}
	_, ipNet, err := net.ParseCIDR(s)
	return err == nil && ipNet.IP.To4() != nil
}
