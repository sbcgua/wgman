package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseModExpression_Valid(t *testing.T) {
	ops, err := parseModExpression("+sandbox,-mailvm")
	if err != nil {
		t.Fatalf("parseModExpression: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("len(ops) = %d, want 2", len(ops))
	}
	if !ops[0].Add || ops[0].Resource != "sandbox" {
		t.Errorf("ops[0] = %+v, want add sandbox", ops[0])
	}
	if ops[1].Add || ops[1].Resource != "mailvm" {
		t.Errorf("ops[1] = %+v, want remove mailvm", ops[1])
	}
}

func TestParseModExpression_Invalid(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{name: "empty", expr: ""},
		{name: "missing sign", expr: "sandbox"},
		{name: "empty resource", expr: "+"},
		{name: "empty operation", expr: "+sandbox,"},
		{name: "invalid name", expr: "+sand box"},
		{name: "duplicate", expr: "+sandbox,-sandbox"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseModExpression(tc.expr); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestPlanModAccess_AddAndRemove(t *testing.T) {
	cfg := makeTestCfg()
	db := makeTestDB()
	ops := []modAccessOp{
		{Add: true, Resource: "mailvm"},
		{Add: false, Resource: "sandbox"},
	}
	updated, deltas, err := planModAccess(cfg, db, "alice", ops)
	if err != nil {
		t.Fatalf("planModAccess: %v", err)
	}
	if got := strings.Join(updated.Access["alice"], ","); got != "mailvm" {
		t.Errorf("updated alice access = %q, want mailvm", got)
	}
	if len(deltas) != 2 {
		t.Fatalf("len(deltas) = %d, want 2: %+v", len(deltas), deltas)
	}
	wantAdd := false
	wantDel := false
	for _, delta := range deltas {
		if delta.Add && delta.Entry == "10.8.0.10,192.168.122.101" && delta.Comment == "alice -> mailvm" {
			wantAdd = true
		}
		if !delta.Add && delta.Entry == "10.8.0.10,192.168.122.100" {
			wantDel = true
		}
	}
	if !wantAdd || !wantDel {
		t.Errorf("missing expected add/delete deltas: %+v", deltas)
	}
}

func TestMod_RefusesMissingUser(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdMod(&globalFlags{configDir: dir}, []string{"nobody", "+sandbox"}, app)
	if code == 0 {
		t.Fatal("mod should fail for missing user")
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("expected not found error, got: %s", stderr.String())
	}
}

func TestMod_RefusesUnknownVM(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdMod(&globalFlags{configDir: dir}, []string{"alice", "+unknown"}, app)
	if code == 0 {
		t.Fatal("mod should fail for unknown VM")
	}
	if !strings.Contains(stderr.String(), "unknown VM") {
		t.Errorf("expected unknown VM error, got: %s", stderr.String())
	}
}

func TestMod_RefusesPreExistingDrift(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildDriftFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdMod(&globalFlags{configDir: dir}, []string{"alice", "+mailvm"}, app)
	if code == 0 {
		t.Fatal("mod should fail on pre-existing drift")
	}
	if !strings.Contains(stderr.String(), "ipset drift") {
		t.Errorf("expected drift error, got: %s", stderr.String())
	}
}

func TestMod_DryRunDoesNotWriteOrApply(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}
	sys := buildCleanFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	code := cmdMod(&globalFlags{configDir: dir, dryRun: true}, []string{"alice", "+mailvm"}, app)
	if code != 0 {
		t.Fatalf("mod --dry-run exit code = %d, want 0", code)
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("db.yaml changed during dry-run")
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops in dry-run, got: %v", sys.appliedOps)
	}
	if !strings.Contains(stdout.String(), "dry-run") {
		t.Errorf("expected dry-run output, got: %s", stdout.String())
	}
}

func TestMod_ValidationFailureDoesNotWriteOrApply(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}
	sys := buildCleanFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdMod(&globalFlags{configDir: dir}, []string{"alice", "+*"}, app)
	if code == 0 {
		t.Fatal("mod should fail when updated access would mix * with VMs")
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("db.yaml changed after validation failure")
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops after validation failure, got: %v", sys.appliedOps)
	}
	if !strings.Contains(stderr.String(), "would be invalid") {
		t.Errorf("expected validation error, got: %s", stderr.String())
	}
}

func TestMod_SuccessWritesDBAndAppliesDelta(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	code := cmdMod(&globalFlags{configDir: dir}, []string{"alice", "+mailvm"}, app)
	if code != 0 {
		t.Fatalf("mod exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "applied") {
		t.Errorf("expected applied output, got: %s", stdout.String())
	}
	found := false
	for _, op := range sys.appliedOps {
		if op == "add:wg_allow_matrix:10.8.0.10,192.168.122.101:alice -> mailvm" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected add op for alice mailvm, got: %v", sys.appliedOps)
	}
	data, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db.yaml: %v", err)
	}
	if !strings.Contains(string(data), "alice:\n    - mailvm\n    - sandbox\n") {
		t.Errorf("expected sorted alice access in db.yaml, got:\n%s", string(data))
	}
	if _, err := LoadDB(dir); err != nil {
		t.Fatalf("updated db.yaml should load: %v", err)
	}
}

func TestMod_RemoveAbsentAccessNoOp(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	code := cmdMod(&globalFlags{configDir: dir}, []string{"alice", "-mailvm"}, app)
	if code != 0 {
		t.Fatalf("mod remove absent exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "no changes needed") {
		t.Errorf("expected no changes output, got: %s", stdout.String())
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops, got: %v", sys.appliedOps)
	}
}
