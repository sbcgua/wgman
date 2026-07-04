package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanRemoveUser(t *testing.T) {
	plan, err := planRemoveUser(makeTestCfg(), makeTestDB(), "bob")
	if err != nil {
		t.Fatalf("planRemoveUser: %v", err)
	}
	if _, ok := plan.UpdatedDB.Users["bob"]; ok {
		t.Fatal("bob still present in updated users")
	}
	if _, ok := plan.UpdatedDB.Access["bob"]; ok {
		t.Fatal("bob still present in updated access")
	}
	if plan.Pub != "BOB_PUB=" || plan.IP != "10.8.0.15" {
		t.Errorf("plan identity = pub %q ip %q", plan.Pub, plan.IP)
	}
	if strings.Join(plan.Access, ",") != "mailvm,sandbox" {
		t.Errorf("plan access = %v, want sorted mailvm,sandbox", plan.Access)
	}
	if len(plan.Deltas) != 2 {
		t.Fatalf("len(deltas) = %d, want 2: %+v", len(plan.Deltas), plan.Deltas)
	}
	for _, delta := range plan.Deltas {
		if delta.Add {
			t.Errorf("remove delta should delete, got add: %+v", delta)
		}
	}
}

func TestRemove_MissingUser(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdRemove(&globalFlags{configDir: dir, yes: true}, []string{"nobody"}, app)
	if code == 0 {
		t.Fatal("remove should fail for missing user")
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("expected not found error, got: %s", stderr.String())
	}
}

func TestRemove_RejectsPreExistingDrift(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildDriftFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdRemove(&globalFlags{configDir: dir, yes: true}, []string{"alice"}, app)
	if code == 0 {
		t.Fatal("remove should fail on pre-existing drift")
	}
	if !strings.Contains(stderr.String(), "ipset drift") {
		t.Errorf("expected drift error, got: %s", stderr.String())
	}
}

func TestRemove_DryRunDoesNotWriteOrApply(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}
	sys := buildCleanFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	code := cmdRemove(&globalFlags{configDir: dir, dryRun: true}, []string{"alice"}, app)
	if code != 0 {
		t.Fatalf("remove --dry-run exit code = %d, want 0", code)
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("db.yaml changed during dry-run")
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops in dry-run, got: %v", sys.appliedOps)
	}
	if !strings.Contains(stdout.String(), "dry-run") || !strings.Contains(stdout.String(), "planned removal") {
		t.Errorf("expected dry-run planned output, got: %s", stdout.String())
	}
}

func TestRemove_ConfirmationRejectedPreventsWriteAndApply(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}
	sys := buildCleanFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "n\n")
	code := cmdRemove(&globalFlags{configDir: dir}, []string{"alice"}, app)
	if code != 0 {
		t.Fatalf("remove rejected confirmation exit code = %d, want 0", code)
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("db.yaml changed after rejected confirmation")
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops after abort, got: %v", sys.appliedOps)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("expected aborted output, got: %s", stdout.String())
	}
}

func TestRemove_YesAppliesWithoutPrompt(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	code := cmdRemove(&globalFlags{configDir: dir, yes: true}, []string{"alice"}, app)
	if code != 0 {
		t.Fatalf("remove --yes exit code = %d, want 0", code)
	}
	if strings.Contains(stdout.String(), "Remove this user?") {
		t.Errorf("--yes should not prompt, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "removed alice") {
		t.Errorf("expected removed output, got: %s", stdout.String())
	}
}

func TestRemove_WritesDBAndAppliesSystemOps(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, _, _ := makeDeployApp(sys, "y\n")
	code := cmdRemove(&globalFlags{configDir: dir}, []string{"bob"}, app)
	if code != 0 {
		t.Fatalf("remove exit code = %d, want 0", code)
	}

	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB after remove: %v", err)
	}
	if _, ok := db.Users["bob"]; ok {
		t.Fatal("bob still present in db users")
	}
	if _, ok := db.Access["bob"]; ok {
		t.Fatal("bob still present in db access")
	}

	wantOps := []string{
		"del:wg_allow_matrix:10.8.0.15,192.168.122.100",
		"del:wg_allow_matrix:10.8.0.15,192.168.122.101",
		"wgdel:wg0:BOB_PUB=",
	}
	for _, want := range wantOps {
		found := false
		for _, op := range sys.appliedOps {
			if op == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing op %q from %v", want, sys.appliedOps)
		}
	}
}

func TestRemove_AdminDeletesAllAccessSetEntry(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	sys := buildCleanFakeSystem()
	app, _, _ := makeDeployApp(sys, "")
	code := cmdRemove(&globalFlags{configDir: dir, yes: true}, []string{"admin"}, app)
	if code != 0 {
		t.Fatalf("remove admin exit code = %d, want 0", code)
	}
	found := false
	for _, op := range sys.appliedOps {
		if op == "del:wg_allow_all:10.8.0.5" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected all-access delete, got: %v", sys.appliedOps)
	}
}
