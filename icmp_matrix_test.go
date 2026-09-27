package main

import (
	"fmt"
	"strings"
	"testing"
)

func icmpTestCfg() *Config {
	cfg := makeTestCfg()
	cfg.Sets.ICMPMatrix = "wg_allow_matrix_icmp"
	return cfg
}

func TestICMPMatrixExpectedEntriesDeduplicateAccess(t *testing.T) {
	db := makeTestDB()
	db.Resources = map[string]ResourceEntry{
		"ssh@sandbox":  {VM: "sandbox", Ports: ResourcePorts{{Protocol: "tcp", Port: 22}}},
		"http@sandbox": {VM: "sandbox", Ports: ResourcePorts{{Protocol: "tcp", Port: 80}}},
	}
	db.Access["alice"] = []string{"sandbox", "ssh@sandbox", "http@sandbox"}
	db.UserGroups = map[string][]string{"team": {"alice"}}
	db.Access["team"] = []string{"mailvm"}
	bob := db.Users["bob"]
	bob.Inactive = true
	db.Users["bob"] = bob

	got := computeExpectedIPSets(db).ICMPMatrix
	want := map[string]string{
		"10.8.0.10,192.168.122.100": "alice -> sandbox (ping)",
		"10.8.0.10,192.168.122.101": "alice -> mailvm (ping)",
	}
	if len(got) != len(want) {
		t.Fatalf("ICMP entries = %#v, want %#v", got, want)
	}
	for entry, comment := range want {
		if got[entry] != comment {
			t.Errorf("ICMP entry %s comment = %q, want %q", entry, got[entry], comment)
		}
	}
}

func TestICMPMatrixResourceAccessCreatesPair(t *testing.T) {
	db := makeResourceDB()
	got := computeExpectedIPSets(db).ICMPMatrix
	if got["10.8.0.10,192.168.122.100"] != "alice -> sandbox (ping)" {
		t.Errorf("resource access did not create sandbox ping pair: %#v", got)
	}
	if got["10.8.0.10,192.168.122.101"] != "alice -> mailvm (ping)" {
		t.Errorf("resource access did not create mailvm ping pair: %#v", got)
	}
}

func TestICMPMatrixCheckReconcilesMissingAndExtraEntries(t *testing.T) {
	sys := buildResourceCleanFakeSystem()
	sys.ipsetResults["wg_allow_matrix_icmp"] = "create wg_allow_matrix_icmp hash:net,net family inet comment\n" +
		"add wg_allow_matrix_icmp 10.8.0.99,192.168.122.100 comment \"stale\"\n"
	result := Check(icmpTestCfg(), makeResourceDB(), sys)
	if len(result.HardErrors) != 0 {
		t.Fatalf("unexpected hard errors: %v", result.HardErrors)
	}
	var resourcePair, staleDelete bool
	for _, delta := range result.IPSetDeltas {
		if delta.Set != "wg_allow_matrix_icmp" {
			continue
		}
		if delta.Entry == "10.8.0.10,192.168.122.100" && delta.Add && delta.Comment == "alice -> sandbox (ping)" {
			resourcePair = true
		}
		if delta.Entry == "10.8.0.99,192.168.122.100" && !delta.Add {
			staleDelete = true
		}
	}
	if !resourcePair || !staleDelete {
		t.Errorf("missing resource add or stale delete: %#v", result.IPSetDeltas)
	}
}

func TestICMPMatrixDeployCanRemoveUnexpectedNetworkEntry(t *testing.T) {
	sys := buildCleanFakeSystem()
	sys.ipsetResults["wg_allow_matrix_icmp"] = "create wg_allow_matrix_icmp hash:net,net family inet comment\n" +
		"add wg_allow_matrix_icmp 10.8.0.10,192.168.122.0/24 comment \"stale\"\n"
	result := Check(icmpTestCfg(), makeTestDB(), sys)
	if len(result.HardErrors) != 0 {
		t.Fatalf("unexpected hard errors: %v", result.HardErrors)
	}
	for _, delta := range result.IPSetDeltas {
		if delta.Set == "wg_allow_matrix_icmp" && delta.Entry == "10.8.0.10,192.168.122.0/24" && !delta.Add {
			return
		}
	}
	t.Errorf("unexpected broad entry was not marked for deletion: %#v", result.IPSetDeltas)
}

func TestICMPMatrixCheckRejectsMissingOrInvalidSet(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		err          error
		want         string
	}{
		{name: "missing", err: fmt.Errorf("set does not exist"), want: "init-ipsets"},
		{name: "wrong type", output: "create wg_allow_matrix_icmp hash:ip,port family inet comment\n", want: "expected hash:net,net"},
		{name: "malformed entry", output: "create wg_allow_matrix_icmp hash:net,net family inet comment\nadd wg_allow_matrix_icmp 10.8.0.10,not-an-ip\n", want: "valid IPv4/net values"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys := buildCleanFakeSystem()
			if tc.err != nil {
				sys.ipsetErrs["wg_allow_matrix_icmp"] = tc.err
			} else {
				sys.ipsetResults["wg_allow_matrix_icmp"] = tc.output
			}
			result := Check(icmpTestCfg(), makeTestDB(), sys)
			if !anyContains(result.HardErrors, tc.want) {
				t.Errorf("hard errors = %v, want %q", result.HardErrors, tc.want)
			}
		})
	}
}

func TestICMPMatrixInitAndLifecycle(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	h.writeFile(dir, "config.yaml", "interface: wg0\nsets:\n  all: a\n  ip_matrix: b\n  port_matrix: c\n  icmp_matrix: ping_set\n")
	for _, tc := range []struct {
		name  string
		flags globalFlags
		want  string
	}{
		{name: "create", flags: globalFlags{}, want: "create:ping_set:hash:net,net:comment"},
		{name: "flush", flags: globalFlags{initIPSetsFlush: true}, want: "flush:ping_set"},
		{name: "destroy", flags: globalFlags{initIPSetsDestroy: true}, want: "destroy:ping_set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys := newFakeSystem()
			app, _, stderr := makeInitIPSetsApp(sys)
			flags := tc.flags
			flags.configDir = dir
			if code := cmdInitIPSets(&flags, nil, app); code != 0 {
				t.Fatalf("init-ipsets exit = %d: %s", code, stderr.String())
			}
			if !strings.Contains(strings.Join(sys.appliedOps, "\n"), tc.want) {
				t.Errorf("operations = %v, want %s", sys.appliedOps, tc.want)
			}
		})
	}
}

func TestICMPMatrixExpectedDiffIsOptional(t *testing.T) {
	old := ExpectedIPSets{ICMPMatrix: map[string]string{}}
	newState := ExpectedIPSets{ICMPMatrix: map[string]string{"10.8.0.10,192.168.122.100": "alice -> sandbox (ping)"}}
	if got := diffExpectedIPSets(makeTestCfg(), old, newState); len(got) != 0 {
		t.Errorf("unconfigured ICMP set produced deltas: %#v", got)
	}
	got := diffExpectedIPSets(icmpTestCfg(), old, newState)
	if len(got) != 1 || got[0].Set != "wg_allow_matrix_icmp" || !got[0].Add {
		t.Errorf("configured ICMP set deltas = %#v", got)
	}
}

func TestICMPMatrixDeployAppliesComputedPairs(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeValidTestData(h, dir)
	h.writeFile(dir, "config.yaml", "interface: wg0\nsets:\n  all: wg_allow_all\n  ip_matrix: wg_allow_matrix\n  port_matrix: wg_allow_matrix_ports\n  icmp_matrix: wg_allow_matrix_icmp\n")
	sys := buildCleanFakeSystem()
	sys.ipsetResults["wg_allow_matrix_icmp"] = "create wg_allow_matrix_icmp hash:net,net family inet comment\n"
	app, _, stderr := makeDeployApp(sys, "")
	if code := cmdDeploy(&globalFlags{configDir: dir, yes: true}, nil, app); code != 0 {
		t.Fatalf("deploy exit = %d: %s", code, stderr.String())
	}
	if !strings.Contains(strings.Join(sys.appliedOps, "\n"), "add:wg_allow_matrix_icmp:10.8.0.10,192.168.122.100:alice -> sandbox (ping)") {
		t.Errorf("deploy did not apply Alice's ping pair: %v", sys.appliedOps)
	}
}
