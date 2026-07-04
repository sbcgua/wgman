package main

import (
	"fmt"
	"testing"
)

// ---- helpers shared across check tests ----

func makeTestCfg() *Config {
	return &Config{
		Interface: "wg0",
		Sets: ConfigSets{
			All:    "wg_allow_all",
			Matrix: "wg_allow_matrix",
		},
	}
}

func makeTestDB() *DB {
	return &DB{
		Users: map[string]UserEntry{
			"admin": {IP: "10.8.0.5", Pub: "ADMIN_PUB="},
			"alice": {IP: "10.8.0.10", Pub: "ALICE_PUB="},
			"bob":   {IP: "10.8.0.15", Pub: "BOB_PUB="},
		},
		VMs: map[string]string{
			"sandbox": "192.168.122.100",
			"mailvm":  "192.168.122.101",
		},
		Access: map[string][]string{
			"admin": {"*"},
			"alice": {"sandbox"},
			"bob":   {"sandbox", "mailvm"},
		},
	}
}

// buildCleanFakeSystem builds a fakeSystem whose state exactly matches makeTestDB().
func buildCleanFakeSystem() *fakeSystem {
	sys := newFakeSystem()
	sys.subnetResult = "10.8.0.1/24"
	sys.wgDumpResult = "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n" +
		"ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n" +
		"ALICE_PUB=\t(none)\t192.168.1.100:50001\t10.8.0.10/32\t1748000000\t102400\t204800\toff\n" +
		"BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n"
	sys.ipsetResults["wg_allow_all"] =
		"create wg_allow_all hash:ip family inet\n" +
			"add wg_allow_all 10.8.0.5\n"
	sys.ipsetResults["wg_allow_matrix"] =
		"create wg_allow_matrix hash:net,net family inet comment\n" +
			"add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n"
	return sys
}

// ---- tests ----

func TestCheck_CleanState(t *testing.T) {
	result := Check(makeTestCfg(), makeTestDB(), buildCleanFakeSystem())
	if len(result.HardErrors) != 0 {
		t.Errorf("expected no hard errors, got: %v", result.HardErrors)
	}
	if len(result.Drift) != 0 {
		t.Errorf("expected no drift, got: %v", result.Drift)
	}
	if len(result.Deltas) != 0 {
		t.Errorf("expected no deltas, got: %v", result.Deltas)
	}
	if !result.OK() {
		t.Error("expected OK()")
	}
}

func TestCheck_OfflineValidationErrors(t *testing.T) {
	db := makeTestDB()
	db.Users["alice"] = UserEntry{IP: "not-an-ip", Pub: "ALICE_PUB="}
	result := Check(makeTestCfg(), db, buildCleanFakeSystem())
	if !anyContains(result.HardErrors, "invalid ip") {
		t.Errorf("expected hard error about invalid ip, got: %v", result.HardErrors)
	}
}

func TestCheck_UserIPOutsideSubnet(t *testing.T) {
	db := makeTestDB()
	db.Users["alice"] = UserEntry{IP: "10.9.0.10", Pub: "ALICE_PUB="}

	sys := buildCleanFakeSystem()
	// Update WG dump to reflect new IP so WG check doesn't also fail.
	sys.wgDumpResult = "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n" +
		"ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n" +
		"ALICE_PUB=\t(none)\t(none)\t10.9.0.10/32\t0\t0\t0\toff\n" +
		"BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n"

	result := Check(makeTestCfg(), db, sys)
	if !anyContains(result.HardErrors, "outside interface subnet") {
		t.Errorf("expected subnet error, got: %v", result.HardErrors)
	}
}

func TestCheck_MissingWGPeer(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Remove alice from WG dump.
	sys.wgDumpResult = "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n" +
		"ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n" +
		"BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "not found in WireGuard peers") {
		t.Errorf("expected missing peer error, got: %v", result.HardErrors)
	}
}

func TestCheck_ExtraWGPeer(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Add a peer that has no db entry.
	sys.wgDumpResult += "UNKNOWN_PUB=\t(none)\t(none)\t10.8.0.99/32\t0\t0\t0\toff\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "not found in db users") {
		t.Errorf("expected extra peer error, got: %v", result.HardErrors)
	}
}

func TestCheck_PeerIPMismatch(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Alice's WG peer has a different IP than db.
	sys.wgDumpResult = "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n" +
		"ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n" +
		"ALICE_PUB=\t(none)\t(none)\t10.8.0.20/32\t0\t0\t0\toff\n" +
		"BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "does not match WireGuard allowed-ip") {
		t.Errorf("expected IP mismatch error, got: %v", result.HardErrors)
	}
}

func TestCheck_MissingIPSetEntry(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Remove alice's matrix entry.
	sys.ipsetResults["wg_allow_matrix"] =
		"create wg_allow_matrix hash:net,net family inet comment\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if len(result.HardErrors) != 0 {
		t.Errorf("expected no hard errors, got: %v", result.HardErrors)
	}
	if !anyContains(result.Drift, "missing entry") {
		t.Errorf("expected drift about missing entry, got: %v", result.Drift)
	}
	// Expect an add delta for the missing entry.
	found := false
	for _, d := range result.Deltas {
		if d.Add && d.Entry == "10.8.0.10,192.168.122.100" {
			found = true
			if d.Comment != "alice -> sandbox" {
				t.Errorf("delta comment = %q, want \"alice -> sandbox\"", d.Comment)
			}
		}
	}
	if !found {
		t.Errorf("expected add delta for 10.8.0.10,192.168.122.100, deltas: %v", result.Deltas)
	}
}

func TestCheck_ExtraIPSetEntry(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Add a spurious entry to the all-access set.
	sys.ipsetResults["wg_allow_all"] =
		"create wg_allow_all hash:ip family inet\n" +
			"add wg_allow_all 10.8.0.5\n" +
			"add wg_allow_all 10.8.0.99\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if len(result.HardErrors) != 0 {
		t.Errorf("expected no hard errors, got: %v", result.HardErrors)
	}
	if !anyContains(result.Drift, "unexpected entry") {
		t.Errorf("expected drift about unexpected entry, got: %v", result.Drift)
	}
	// Expect a delete delta.
	found := false
	for _, d := range result.Deltas {
		if !d.Add && d.Entry == "10.8.0.99" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected delete delta for 10.8.0.99, deltas: %v", result.Deltas)
	}
}

func TestCheck_MissingIPSet(t *testing.T) {
	sys := buildCleanFakeSystem()
	sys.ipsetErrs["wg_allow_all"] = fmt.Errorf("ipset: set not found")

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "ipset list") {
		t.Errorf("expected hard error about missing ipset, got: %v", result.HardErrors)
	}
}

func TestCheck_HardErrorsAndDriftSeparated(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Inject an extra WG peer (hard error) AND a missing ipset entry (drift).
	sys.wgDumpResult += "UNKNOWN_PUB=\t(none)\t(none)\t10.8.0.99/32\t0\t0\t0\toff\n"
	// Note: with hard errors from WG validation, drift check is skipped; this
	// test verifies that hard errors from WG block ipset drift accumulation.
	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "not found in db users") {
		t.Errorf("expected WG hard error, got: %v", result.HardErrors)
	}
	// Drift is not populated because we returned early after WG hard errors.
	if len(result.Drift) != 0 {
		t.Errorf("expected no drift when hard errors are present, got: %v", result.Drift)
	}
}

func TestComputeExpectedIPSets(t *testing.T) {
	db := makeTestDB()
	allExp, matrixExp := computeExpectedIPSets(db)

	if _, ok := allExp["10.8.0.5"]; !ok {
		t.Error("expected admin ip in all-access set")
	}
	if len(allExp) != 1 {
		t.Errorf("all-access set size = %d, want 1", len(allExp))
	}

	if _, ok := matrixExp["10.8.0.10,192.168.122.100"]; !ok {
		t.Error("expected alice->sandbox in matrix set")
	}
	if _, ok := matrixExp["10.8.0.15,192.168.122.100"]; !ok {
		t.Error("expected bob->sandbox in matrix set")
	}
	if _, ok := matrixExp["10.8.0.15,192.168.122.101"]; !ok {
		t.Error("expected bob->mailvm in matrix set")
	}
	if len(matrixExp) != 3 {
		t.Errorf("matrix set size = %d, want 3", len(matrixExp))
	}
	if matrixExp["10.8.0.10,192.168.122.100"] != "alice -> sandbox" {
		t.Errorf("comment = %q, want \"alice -> sandbox\"", matrixExp["10.8.0.10,192.168.122.100"])
	}
}

func TestCheck_WGDumpError(t *testing.T) {
	sys := buildCleanFakeSystem()
	sys.wgDumpErr = fmt.Errorf("wg not found")

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "wg dump") {
		t.Errorf("expected wg dump error, got: %v", result.HardErrors)
	}
}

func TestCheck_InterfaceSubnetError(t *testing.T) {
	sys := buildCleanFakeSystem()
	sys.subnetErr = fmt.Errorf("no such interface")

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "get interface subnet") {
		t.Errorf("expected subnet error, got: %v", result.HardErrors)
	}
}

func TestCheck_AllAccessWrongSetType(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Replace the all-access set with the wrong type (hash:net instead of hash:ip).
	sys.ipsetResults["wg_allow_all"] =
		"create wg_allow_all hash:net family inet\n" +
			"add wg_allow_all 10.8.0.5\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "hash:ip") {
		t.Errorf("expected hard error about wrong set type, got: %v", result.HardErrors)
	}
	// Must be a hard error, not drift.
	if len(result.Drift) != 0 {
		t.Errorf("expected no drift when set type is wrong, got: %v", result.Drift)
	}
}

func TestCheck_MatrixWrongSetType(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Replace the matrix set with the wrong type.
	sys.ipsetResults["wg_allow_matrix"] =
		"create wg_allow_matrix hash:ip family inet\n" +
			"add wg_allow_matrix 10.8.0.10\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "hash:net,net") {
		t.Errorf("expected hard error about wrong matrix set type, got: %v", result.HardErrors)
	}
	if len(result.Drift) != 0 {
		t.Errorf("expected no drift when set type is wrong, got: %v", result.Drift)
	}
}

func TestCheck_AllAccessInvalidEntryShape(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Add an IPv6 address as an all-access entry.
	sys.ipsetResults["wg_allow_all"] =
		"create wg_allow_all hash:ip family inet\n" +
			"add wg_allow_all ::1\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "not a valid IPv4 address") {
		t.Errorf("expected hard error for invalid all-access entry, got: %v", result.HardErrors)
	}
}

func TestCheck_MatrixInvalidEntryShape(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Add a single-value entry (missing the second net) to the matrix set.
	sys.ipsetResults["wg_allow_matrix"] =
		"create wg_allow_matrix hash:net,net family inet comment\n" +
			"add wg_allow_matrix 10.8.0.10\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "not two valid IPv4/net values") {
		t.Errorf("expected hard error for invalid matrix entry, got: %v", result.HardErrors)
	}
}

func TestCheck_AddLineSetNameMismatch(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Provide output where the add line references a different set name.
	sys.ipsetResults["wg_allow_all"] =
		"create wg_allow_all hash:ip family inet\n" +
			"add other_set 10.8.0.5\n"

	result := Check(makeTestCfg(), makeTestDB(), sys)
	if !anyContains(result.HardErrors, "parse ipset") {
		t.Errorf("expected parse hard error for set name mismatch, got: %v", result.HardErrors)
	}
}
