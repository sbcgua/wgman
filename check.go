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
					User:      name,
					PubKey:    u.Pub,
					AllowedIP: u.IP,
					Action:    WGPeerRemove,
				})
			}
			continue
		}
		if !ok {
			result.PeerDeltas = append(result.PeerDeltas, WGPeerDeltaOp{
				User:      name,
				PubKey:    u.Pub,
				AllowedIP: u.IP,
				Action:    WGPeerAdd,
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
	expected := computeExpectedIPSets(db)

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
			reconcileIPSet(cfg.Sets.All, expected.All, allParsed.Entries, result)
		}
	}

	// IP matrix set.
	matrixRaw, err := sys.IPSetList(cfg.Sets.IPMatrix)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("ipset list %q: %s (run 'wgman init-ipsets' to create managed sets)", cfg.Sets.IPMatrix, err))
	} else {
		matrixParsed, err := ParseIPSet(matrixRaw)
		if err != nil {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("parse ipset %q: %s", cfg.Sets.IPMatrix, err))
		} else if errs := validateIPMatrixIPSet(cfg.Sets.IPMatrix, matrixParsed); len(errs) > 0 {
			result.HardErrors = append(result.HardErrors, errs...)
		} else {
			reconcileIPSet(cfg.Sets.IPMatrix, expected.IPMatrix, matrixParsed.Entries, result)
		}
	}

	// Port matrix set.
	portMatrixRaw, err := sys.IPSetList(cfg.Sets.PortMatrix)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("ipset list %q: %s (run 'wgman init-ipsets' to create managed sets)", cfg.Sets.PortMatrix, err))
	} else {
		portMatrixParsed, err := ParseIPSet(portMatrixRaw)
		if err != nil {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("parse ipset %q: %s", cfg.Sets.PortMatrix, err))
		} else if errs := validatePortMatrixIPSet(cfg.Sets.PortMatrix, portMatrixParsed); len(errs) > 0 {
			result.HardErrors = append(result.HardErrors, errs...)
		} else {
			reconcileIPSet(cfg.Sets.PortMatrix, expected.PortMatrix, portMatrixParsed.Entries, result)
		}
	}

	sort.Strings(result.HardErrors)
	sort.Strings(result.Drift)
	sort.Slice(result.IPSetDeltas, func(i, j int) bool {
		if result.IPSetDeltas[i].Set != result.IPSetDeltas[j].Set {
			return result.IPSetDeltas[i].Set < result.IPSetDeltas[j].Set
		}
		return result.IPSetDeltas[i].Entry < result.IPSetDeltas[j].Entry
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
func computeExpectedIPSets(db *DB) ExpectedIPSets {
	expected := ExpectedIPSets{
		All:        make(map[string]string),
		IPMatrix:   make(map[string]string),
		PortMatrix: make(map[string]string),
	}

	for user, u := range db.Users {
		vms := effectiveAccessForUser(db, user)
		if u.Inactive {
			continue
		}
		for _, vm := range vms {
			if vm == "*" {
				expected.All[u.IP] = ""
				continue
			}
			if vmIP, ok := db.VMs[vm]; ok {
				entry := u.IP + "," + vmIP
				expected.IPMatrix[entry] = user + " -> " + vm
				continue
			}
			resource, ok := db.Resources[vm]
			if !ok {
				continue
			}
			vmIP, ok := db.VMs[resource.VM]
			if !ok {
				continue
			}
			for _, port := range resource.Ports {
				entry := u.IP + "," + port.String() + "," + vmIP
				expected.PortMatrix[entry] = fmt.Sprintf("%s -> %s %s/%d", user, vm, port.Protocol, port.Port)
			}
		}
	}
	return expected
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
			result.IPSetDeltas = append(result.IPSetDeltas, IpsetDeltaOp{
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
			result.IPSetDeltas = append(result.IPSetDeltas, IpsetDeltaOp{
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

// validateIPMatrixIPSet checks that the live matrix set has the correct type
// (hash:net,net) and that all entries are two comma-separated IPv4/net values.
// Returns hard error strings; an empty slice means validation passed.
func validateIPMatrixIPSet(setname string, parsed *ParsedIPSet) []string {
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

// validatePortMatrixIPSet checks that the live port matrix set has the correct
// type (hash:ip,port,ip) and that entries are source IP, port, destination IP.
func validatePortMatrixIPSet(setname string, parsed *ParsedIPSet) []string {
	var errs []string
	if parsed.SetName != setname {
		errs = append(errs, fmt.Sprintf(
			"ipset %q output describes set %q, expected %q",
			setname, parsed.SetName, setname))
		return errs
	}
	if parsed.SetType != "hash:ip,port,ip" {
		errs = append(errs, fmt.Sprintf(
			"ipset %q has type %q, expected hash:ip,port,ip (run 'wgman init-ipsets' to recreate)",
			setname, parsed.SetType))
		return errs
	}
	for _, e := range parsed.Entries {
		parts := strings.Split(e.Entry, ",")
		if len(parts) != 3 {
			errs = append(errs, fmt.Sprintf(
				"ipset %q: port matrix entry %q is not source-ip,port,destination-ip", setname, e.Entry))
			continue
		}
		src := net.ParseIP(parts[0])
		dst := net.ParseIP(parts[2])
		if src == nil || src.To4() == nil || dst == nil || dst.To4() == nil {
			errs = append(errs, fmt.Sprintf(
				"ipset %q: port matrix entry %q must use IPv4 source and destination addresses", setname, e.Entry))
		}
		if _, err := parseResourcePortScalar(parts[1]); err != nil {
			errs = append(errs, fmt.Sprintf(
				"ipset %q: port matrix entry %q has invalid port: %s", setname, e.Entry, err))
		}
	}
	return errs
}
