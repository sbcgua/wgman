package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestApplyDeltasTrackedStopsAtFirstError(t *testing.T) {
	sys := newFakeSystem()
	sys.ipsetDelErr = errors.New("delete failed")
	deltas := []IpsetDeltaOp{
		{Set: "allow", Entry: "10.8.0.10", Comment: "alice", Add: true},
		{Set: "allow", Entry: "10.8.0.20"},
		{Set: "allow", Entry: "10.8.0.30", Add: true},
	}

	applied, err := ApplyDeltasTracked(deltas, sys)

	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("ApplyDeltasTracked() error = %v, want delete failure", err)
	}
	if !reflect.DeepEqual(applied, deltas[:1]) {
		t.Errorf("ApplyDeltasTracked() applied = %#v, want %#v", applied, deltas[:1])
	}
	wantOps := []string{"add:allow:10.8.0.10:alice"}
	if !reflect.DeepEqual(sys.appliedOps, wantOps) {
		t.Errorf("applied operations = %#v, want %#v", sys.appliedOps, wantOps)
	}
}

func TestInvertDeltasReversesOrderAndOperation(t *testing.T) {
	deltas := []IpsetDeltaOp{
		{Set: "allow", Entry: "10.8.0.10", Comment: "alice", Add: true},
		{Set: "allow", Entry: "10.8.0.20"},
	}

	got := InvertDeltas(deltas)
	want := []IpsetDeltaOp{
		{Set: "allow", Entry: "10.8.0.20", Add: true},
		{Set: "allow", Entry: "10.8.0.10", Comment: "alice"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("InvertDeltas() = %#v, want %#v", got, want)
	}
	if !deltas[0].Add || deltas[1].Add {
		t.Error("InvertDeltas() modified its input")
	}
}

func TestDiffExpectedIPSetsBuildsStableDeltas(t *testing.T) {
	cfg := &Config{
		Sets: ConfigSets{
			All:    "wg_allow_all",
			Matrix: "wg_allow_matrix",
		},
	}
	oldAll := map[string]string{
		"10.8.0.5": "",
	}
	oldMatrix := map[string]string{
		"10.8.0.10,192.168.122.100": "alice -> sandbox",
	}
	newAll := map[string]string{
		"10.8.0.6": "",
	}
	newMatrix := map[string]string{
		"10.8.0.10,192.168.122.101": "alice -> mailvm",
	}

	got := diffExpectedIPSets(cfg, oldAll, oldMatrix, newAll, newMatrix)
	want := []IpsetDeltaOp{
		{Set: "wg_allow_all", Entry: "10.8.0.5"},
		{Set: "wg_allow_all", Entry: "10.8.0.6", Add: true},
		{Set: "wg_allow_matrix", Entry: "10.8.0.10,192.168.122.100", Comment: "alice -> sandbox"},
		{Set: "wg_allow_matrix", Entry: "10.8.0.10,192.168.122.101", Comment: "alice -> mailvm", Add: true},
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
		{User: "bob", PubKey: "BOB", Remove: true},
		{User: "alice", PubKey: "ALICE", AllowedIP: "10.8.0.10", Add: true},
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
			{User: "alice", PubKey: "ALICE", AllowedIP: "10.8.0.10", Add: true},
			{User: "bob", PubKey: "BOB", Remove: true},
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
