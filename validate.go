package main

import (
	"fmt"
	"net"
)

// ValidateOffline performs all checks that do not require live system access:
// config/db structure validation and cross-reference checks.
// It returns a CheckResult with HardErrors populated; Drift and Deltas are
// populated by later phases when live state is available.
func ValidateOffline(cfg *Config, db *DB) *CheckResult {
	result := &CheckResult{}

	// User and VM name/IP uniqueness were already verified by LoadDB structural
	// validation. Here we perform cross-reference and semantic checks.

	// Validate that all user IPs are valid IPv4 addresses.
	for name, u := range db.Users {
		if ip := net.ParseIP(u.IP); ip == nil || ip.To4() == nil {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("user %q has invalid ip %q", name, u.IP))
		}
	}

	// Validate that all VM IPs are valid IPv4 addresses.
	for name, ip := range db.VMs {
		if parsed := net.ParseIP(ip); parsed == nil || parsed.To4() == nil {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("vm %q has invalid ip %q", name, ip))
		}
	}

	// Validate access entries (cross-reference users and vms).
	for user, vms := range db.Access {
		if _, ok := db.Users[user]; !ok {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("access references unknown user %q", user))
			continue
		}
		hasAdminStar := false
		for _, vm := range vms {
			if vm == "*" {
				hasAdminStar = true
				continue
			}
			if _, ok := db.VMs[vm]; !ok {
				result.HardErrors = append(result.HardErrors,
					fmt.Sprintf("access for user %q references unknown vm %q", user, vm))
			}
		}
		// If a user has "*" in access, it must be the only entry.
		if hasAdminStar && len(vms) > 1 {
			result.HardErrors = append(result.HardErrors,
				fmt.Sprintf("user %q: access contains \"*\" mixed with other VMs; \"*\" must be the sole entry", user))
		}
	}

	_ = cfg // cfg used by later check phases (live WireGuard/ipset checks)
	return result
}
