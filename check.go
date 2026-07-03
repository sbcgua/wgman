package main

import (
	"fmt"
	"net"
	"sort"
)

// Check performs the full validation: offline config/db checks followed by
// live WireGuard and ipset state verification via sys.
//
// HardErrors are issues that make the state unsafe or ambiguous.
// Drift is ipset discrepancy between db.yaml and live ipsets.
// Deltas are the concrete ipset operations needed to reconcile drift.
func Check(cfg *Config, db *DB, sys SystemAdapter) *CheckResult {
	result := ValidateOffline(cfg, db)
	if !result.Clean() {
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
			continue // already caught by ValidateOffline
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

	// Every db user must have a matching WG peer with the correct IP.
	for name, u := range db.Users {
		peer, ok := wgByPub[u.Pub]
		if !ok {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("user %q (pub %s) not found in WireGuard peers", name, u.Pub))
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
			fmt.Sprintf("ipset list %q: %s (does the set exist?)", cfg.Sets.All, err))
	} else {
		allEntries, err := ParseIPSetEntries(allRaw)
		if err != nil {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("parse ipset %q: %s", cfg.Sets.All, err))
		} else {
			reconcileIPSet(cfg.Sets.All, allExpected, allEntries, result)
		}
	}

	// Matrix set.
	matrixRaw, err := sys.IPSetList(cfg.Sets.Matrix)
	if err != nil {
		result.HardErrors = append(result.HardErrors,
			fmt.Sprintf("ipset list %q: %s (does the set exist?)", cfg.Sets.Matrix, err))
	} else {
		matrixEntries, err := ParseIPSetEntries(matrixRaw)
		if err != nil {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("parse ipset %q: %s", cfg.Sets.Matrix, err))
		} else {
			reconcileIPSet(cfg.Sets.Matrix, matrixExpected, matrixEntries, result)
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
		if !ok {
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
