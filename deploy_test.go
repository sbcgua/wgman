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
