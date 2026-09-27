package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUserGroupAccessTestData(h *testHelper, dir string) {
	writeValidTestData(h, dir)
	db, err := LoadDB(dir)
	if err != nil {
		h.t.Fatalf("LoadDB: %v", err)
	}
	db.UserGroups = map[string][]string{"devs": {}}
	db.Access["devs"] = []string{"mailvm"}
	if err := SaveDBAtomic(dir, db); err != nil {
		h.t.Fatalf("SaveDBAtomic: %v", err)
	}
}

func TestParseUserGroupExpression(t *testing.T) {
	ops, err := parseUserGroupExpression("+alice,-bob")
	if err != nil {
		t.Fatalf("parseUserGroupExpression: %v", err)
	}
	if len(ops) != 2 || !ops[0].Add || ops[0].User != "alice" || ops[1].Add || ops[1].User != "bob" {
		t.Errorf("ops = %+v, want add alice remove bob", ops)
	}
	for _, expr := range []string{"", "alice", "+", "+alice,+alice", "+bad name"} {
		if _, err := parseUserGroupExpression(expr); err == nil {
			t.Errorf("parseUserGroupExpression(%q) succeeded, want error", expr)
		}
	}
}

func TestPlanUserGroupCreatesOnlyWhenAdding(t *testing.T) {
	db := makeTestDB()
	if _, err := planUserGroup(makeTestCfg(), db, "devs", []userGroupOp{{Add: false, User: "alice"}}); err == nil {
		t.Fatal("removing from missing group should fail")
	}
	plan, err := planUserGroup(makeTestCfg(), db, "devs", []userGroupOp{{Add: true, User: "alice"}})
	if err != nil {
		t.Fatalf("adding to missing group should create it: %v", err)
	}
	if got := strings.Join(plan.UpdatedDB.UserGroups["devs"], ","); got != "alice" {
		t.Errorf("devs members = %q, want alice", got)
	}
}

func TestUserGroupList(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeUserGroupAccessTestData(h, dir)
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}
	db.UserGroups["devs"] = []string{"bob", "alice"}
	if err := SaveDBAtomic(dir, db); err != nil {
		t.Fatalf("SaveDBAtomic: %v", err)
	}

	sys := buildCleanFakeSystem()
	sys.ipsetResults["wg_allow_matrix"] =
		"create wg_allow_matrix hash:net,net family inet comment\n" +
			"add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.10,192.168.122.101 comment \"alice -> mailvm\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n"
	app, stdout, stderr := makeDeployApp(sys, "")
	code := cmdUserGroup(&globalFlags{configDir: dir}, []string{"devs"}, app)
	if code != 0 {
		t.Fatalf("usergroup list exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "devs: alice,bob") {
		t.Errorf("expected sorted group members, got: %s", stdout.String())
	}
}

func TestUserGroupAddWritesDBAndAppliesIPSetDeltas(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeUserGroupAccessTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, stdout, stderr := makeDeployApp(sys, "")

	code := cmdUserGroup(&globalFlags{configDir: dir}, []string{"devs", "+alice"}, app)
	if code != 0 {
		t.Fatalf("usergroup add exit code = %d, want 0; stderr: %s", code, stderr.String())
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
		t.Errorf("missing inherited ipset add, got: %v", sys.appliedOps)
	}
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}
	if got := strings.Join(db.UserGroups["devs"], ","); got != "alice" {
		t.Errorf("devs members = %q, want alice", got)
	}
}

func TestUserGroupDryRunDoesNotWriteOrApply(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeUserGroupAccessTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	sys := buildCleanFakeSystem()
	app, stdout, stderr := makeDeployApp(sys, "")

	code := cmdUserGroup(&globalFlags{configDir: dir, dryRun: true}, []string{"devs", "+alice"}, app)
	if code != 0 {
		t.Fatalf("usergroup dry-run exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("db.yaml changed during dry-run")
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops, got: %v", sys.appliedOps)
	}
	if !strings.Contains(stdout.String(), "dry-run") {
		t.Errorf("expected dry-run output, got: %s", stdout.String())
	}
}
