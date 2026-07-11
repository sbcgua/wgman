package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestApplyIPSetDeltasTrackedStopsAtFirstError(t *testing.T) {
	sys := newFakeSystem()
	sys.ipsetDelErr = errors.New("delete failed")
	deltas := []IpsetDeltaOp{
		{Set: "allow", Entry: "10.8.0.10", Comment: "alice", Add: true},
		{Set: "allow", Entry: "10.8.0.20"},
		{Set: "allow", Entry: "10.8.0.30", Add: true},
	}

	applied, err := ApplyIPSetDeltasTracked(deltas, sys)

	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("ApplyIPSetDeltasTracked() error = %v, want delete failure", err)
	}
	if !reflect.DeepEqual(applied, deltas[:1]) {
		t.Errorf("ApplyIPSetDeltasTracked() applied = %#v, want %#v", applied, deltas[:1])
	}
	wantOps := []string{"add:allow:10.8.0.10:alice"}
	if !reflect.DeepEqual(sys.appliedOps, wantOps) {
		t.Errorf("applied operations = %#v, want %#v", sys.appliedOps, wantOps)
	}
}

func TestInvertIPSetDeltasReversesOrderAndOperation(t *testing.T) {
	deltas := []IpsetDeltaOp{
		{Set: "allow", Entry: "10.8.0.10", Comment: "alice", Add: true},
		{Set: "allow", Entry: "10.8.0.20"},
	}

	got := InvertIPSetDeltas(deltas)
	want := []IpsetDeltaOp{
		{Set: "allow", Entry: "10.8.0.20", Add: true},
		{Set: "allow", Entry: "10.8.0.10", Comment: "alice"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("InvertIPSetDeltas() = %#v, want %#v", got, want)
	}
	if !deltas[0].Add || deltas[1].Add {
		t.Error("InvertIPSetDeltas() modified its input")
	}
}

func TestInvertPeerDeltasReversesOrderAndOperation(t *testing.T) {
	deltas := []WGPeerDeltaOp{
		{User: "alice", PubKey: "ALICE", AllowedIP: "10.8.0.10", Action: WGPeerAdd},
		{User: "bob", PubKey: "BOB", AllowedIP: "10.8.0.15", Action: WGPeerRemove},
	}

	got := InvertPeerDeltas(deltas)
	want := []WGPeerDeltaOp{
		{User: "bob", PubKey: "BOB", AllowedIP: "10.8.0.15", Action: WGPeerAdd},
		{User: "alice", PubKey: "ALICE", AllowedIP: "10.8.0.10", Action: WGPeerRemove},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("InvertPeerDeltas() = %#v, want %#v", got, want)
	}
	if deltas[0].Action != WGPeerAdd || deltas[1].Action != WGPeerRemove {
		t.Error("InvertPeerDeltas() modified its input")
	}
}

func TestDiffExpectedIPSetsBuildsStableDeltas(t *testing.T) {
	cfg := &Config{
		Sets: ConfigSets{
			All:        "wg_allow_all",
			IPMatrix:   "wg_allow_matrix",
			PortMatrix: "wg_allow_matrix_ports",
		},
	}
	oldExpected := ExpectedIPSets{
		All: map[string]string{
			"10.8.0.5": "",
		},
		IPMatrix: map[string]string{
			"10.8.0.10,192.168.122.100": "alice -> sandbox",
		},
		PortMatrix: map[string]string{
			"10.8.0.10,tcp:22,192.168.122.100": "alice -> ssh@sandbox tcp/22",
		},
	}
	newExpected := ExpectedIPSets{
		All: map[string]string{
			"10.8.0.6": "",
		},
		IPMatrix: map[string]string{
			"10.8.0.10,192.168.122.101": "alice -> mailvm",
		},
		PortMatrix: map[string]string{
			"10.8.0.10,udp:53,192.168.122.101": "alice -> dns@mailvm udp/53",
		},
	}

	got := diffExpectedIPSets(cfg, oldExpected, newExpected)
	want := []IpsetDeltaOp{
		{Set: "wg_allow_all", Entry: "10.8.0.5"},
		{Set: "wg_allow_all", Entry: "10.8.0.6", Add: true},
		{Set: "wg_allow_matrix", Entry: "10.8.0.10,192.168.122.100", Comment: "alice -> sandbox"},
		{Set: "wg_allow_matrix", Entry: "10.8.0.10,192.168.122.101", Comment: "alice -> mailvm", Add: true},
		{Set: "wg_allow_matrix_ports", Entry: "10.8.0.10,tcp:22,192.168.122.100", Comment: "alice -> ssh@sandbox tcp/22"},
		{Set: "wg_allow_matrix_ports", Entry: "10.8.0.10,udp:53,192.168.122.101", Comment: "alice -> dns@mailvm udp/53", Add: true},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("diffExpectedIPSets() = %#v, want %#v", got, want)
	}
}

func TestDiffExpectedIPSetNoChanges(t *testing.T) {
	expected := map[string]string{
		"10.8.0.10,192.168.122.100": "alice -> sandbox",
	}

	got := diffExpectedIPSet("wg_allow_matrix", expected, expected)

	if len(got) != 0 {
		t.Errorf("diffExpectedIPSet() = %#v, want no deltas", got)
	}
}

func TestApplyStateDeltasUsesDependencyOrder(t *testing.T) {
	sys := newFakeSystem()
	ipsetDeltas := []IpsetDeltaOp{
		{Set: "allow", Entry: "10.8.0.10", Comment: "alice", Add: true},
		{Set: "allow", Entry: "10.8.0.20"},
	}
	peerDeltas := []WGPeerDeltaOp{
		{User: "bob", PubKey: "BOB", AllowedIP: "10.8.0.20", Action: WGPeerRemove},
		{User: "alice", PubKey: "ALICE", AllowedIP: "10.8.0.10", Action: WGPeerAdd},
	}

	if err := ApplyStateDeltas("wg0", ipsetDeltas, peerDeltas, sys); err != nil {
		t.Fatalf("ApplyStateDeltas() error = %v", err)
	}

	want := []string{
		"wgset:wg0:ALICE:10.8.0.10",
		"add:allow:10.8.0.10:alice",
		"del:allow:10.8.0.20",
		"wgdel:wg0:BOB",
	}
	if !reflect.DeepEqual(sys.appliedOps, want) {
		t.Errorf("applied operations = %#v, want %#v", sys.appliedOps, want)
	}
}

func TestApplyStateDeltasStopsBeforePeerRemovalOnIPSetError(t *testing.T) {
	sys := newFakeSystem()
	sys.ipsetAddErr = errors.New("add failed")

	err := ApplyStateDeltas(
		"wg0",
		[]IpsetDeltaOp{{Set: "allow", Entry: "10.8.0.10", Add: true}},
		[]WGPeerDeltaOp{
			{User: "alice", PubKey: "ALICE", AllowedIP: "10.8.0.10", Action: WGPeerAdd},
			{User: "bob", PubKey: "BOB", AllowedIP: "10.8.0.20", Action: WGPeerRemove},
		},
		sys,
	)

	if err == nil || !strings.Contains(err.Error(), "add failed") {
		t.Fatalf("ApplyStateDeltas() error = %v, want add failure", err)
	}
	want := []string{"wgset:wg0:ALICE:10.8.0.10"}
	if !reflect.DeepEqual(sys.appliedOps, want) {
		t.Errorf("applied operations = %#v, want %#v", sys.appliedOps, want)
	}
}

func TestApplyStateDeltasTrackedReturnsCompletedOperations(t *testing.T) {
	sys := newFakeSystem()
	sys.wgDelErr = errors.New("delete failed")
	ipsetDeltas := []IpsetDeltaOp{
		{Set: "allow", Entry: "10.8.0.10", Comment: "alice", Add: true},
	}
	peerDeltas := []WGPeerDeltaOp{
		{User: "alice", PubKey: "ALICE", AllowedIP: "10.8.0.10", Action: WGPeerAdd},
		{User: "bob", PubKey: "BOB", AllowedIP: "10.8.0.20", Action: WGPeerRemove},
	}

	applied, err := ApplyStateDeltasTracked("wg0", ipsetDeltas, peerDeltas, sys)

	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("ApplyStateDeltasTracked() error = %v, want delete failure", err)
	}
	if !reflect.DeepEqual(applied.PeerAdds, peerDeltas[:1]) {
		t.Errorf("PeerAdds = %#v, want %#v", applied.PeerAdds, peerDeltas[:1])
	}
	if !reflect.DeepEqual(applied.IPSetDeltas, ipsetDeltas) {
		t.Errorf("IPSetDeltas = %#v, want %#v", applied.IPSetDeltas, ipsetDeltas)
	}
	if len(applied.PeerRemoves) != 0 {
		t.Errorf("PeerRemoves = %#v, want none", applied.PeerRemoves)
	}
}

func TestRollbackStateDeltasUsesReverseDependencyOrder(t *testing.T) {
	sys := newFakeSystem()
	applied := AppliedStateDeltas{
		PeerAdds: []WGPeerDeltaOp{
			{User: "alice", PubKey: "ALICE", AllowedIP: "10.8.0.10", Action: WGPeerAdd},
		},
		IPSetDeltas: []IpsetDeltaOp{
			{Set: "allow", Entry: "10.8.0.10", Comment: "alice", Add: true},
		},
		PeerRemoves: []WGPeerDeltaOp{
			{User: "bob", PubKey: "BOB", AllowedIP: "10.8.0.20", Action: WGPeerRemove},
		},
	}

	if err := RollbackStateDeltas("wg0", applied, sys); err != nil {
		t.Fatalf("RollbackStateDeltas() error = %v", err)
	}

	want := []string{
		"wgset:wg0:BOB:10.8.0.20",
		"del:allow:10.8.0.10",
		"wgdel:wg0:ALICE",
	}
	if !reflect.DeepEqual(sys.appliedOps, want) {
		t.Errorf("applied operations = %#v, want %#v", sys.appliedOps, want)
	}
}
