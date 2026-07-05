package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// makeDeployApp builds an App wired to sys with captured I/O.
// stdin provides bytes for any confirmation prompt.
func makeDeployApp(sys *fakeSystem, stdin string) (*App, *strings.Builder, *strings.Builder) {
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	return &App{
		Sys:    sys,
		Stdin:  strings.NewReader(stdin),
		Stdout: stdout,
		Stderr: stderr,
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, stdout, stderr
}

// writeDeployTestData writes config.yaml and db.yaml matching makeTestCfg() /
// makeTestDB() to dir for use in deploy command tests.
func writeDeployTestData(h *testHelper, dir string) {
	h.writeFile(dir, "config.yaml", `interface: wg0
sets:
  all: wg_allow_all
  matrix: wg_allow_matrix
`)
	h.writeFile(dir, "db.yaml", `users:
  admin:
    ip: 10.8.0.5
    pub: ADMIN_PUB=
  alice:
    ip: 10.8.0.10
    pub: ALICE_PUB=
  bob:
    ip: 10.8.0.15
    pub: BOB_PUB=
vms:
  sandbox: 192.168.122.100
  mailvm: 192.168.122.101
access:
  admin:
    - "*"
  alice:
    - sandbox
  bob:
    - sandbox
    - mailvm
`)
}

func writeDeployInactiveBobTestData(h *testHelper, dir string) {
	h.writeFile(dir, "config.yaml", `interface: wg0
sets:
  all: wg_allow_all
  matrix: wg_allow_matrix
`)
	h.writeFile(dir, "db.yaml", `users:
  admin:
    ip: 10.8.0.5
    pub: ADMIN_PUB=
  alice:
    ip: 10.8.0.10
    pub: ALICE_PUB=
  bob:
    ip: 10.8.0.15
    pub: BOB_PUB=
    comment: on leave
    inactive: true
vms:
  sandbox: 192.168.122.100
  mailvm: 192.168.122.101
access:
  admin:
    - "*"
  alice:
    - sandbox
  bob:
    - sandbox
    - mailvm
`)
}

// buildDriftFakeSystem returns a fakeSystem that matches makeTestDB() for
// WireGuard but is missing alice's sandbox entry in the matrix ipset, so
// Check returns drift but no hard errors.
func buildDriftFakeSystem() *fakeSystem {
	sys := buildCleanFakeSystem()
	// Drop alice's matrix entry to create one missing-entry drift.
	sys.ipsetResults["wg_allow_matrix"] =
		"create wg_allow_matrix hash:net,net family inet comment\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n"
	return sys
}

func buildInactivePeerFakeSystem() *fakeSystem {
	sys := buildInactiveBobAbsentFakeSystem()
	sys.wgDumpResult += "BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n"
	return sys
}

func buildInactivePeerAndIPSetDriftFakeSystem() *fakeSystem {
	sys := buildInactivePeerFakeSystem()
	sys.ipsetResults["wg_allow_matrix"] =
		"create wg_allow_matrix hash:net,net family inet comment\n" +
			"add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n"
	return sys
}

// ---- tests ----

func TestDeploy_RejectsPositionalArgs(t *testing.T) {
	sys := newFakeSystem()
	app, _, _ := makeDeployApp(sys, "")
	gf := &globalFlags{configDir: "testdata/valid-offline"}
	code := cmdDeploy(gf, []string{"extra"}, app)
	if code == 0 {
		t.Error("deploy with positional arg should return non-zero")
	}
}

func TestDeploy_RequiresRoot(t *testing.T) {
	sys := newFakeSystem()
	sys.isRoot = false
	app, _, stderr := makeDeployApp(sys, "")
	gf := &globalFlags{configDir: "testdata/valid-offline"}
	code := cmdDeploy(gf, nil, app)
	if code == 0 {
		t.Error("deploy without root should return non-zero")
	}
	if !strings.Contains(stderr.String(), "root") {
		t.Errorf("expected root error, got: %s", stderr.String())
	}
}

func TestDeploy_RefusesHardErrors(t *testing.T) {
	sys := buildCleanFakeSystem()
	sys.wgDumpErr = fmt.Errorf("connection refused")
	app, _, stderr := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir}
	code := cmdDeploy(gf, nil, app)
	if code == 0 {
		t.Error("deploy should refuse on hard errors")
	}
	if !strings.Contains(stderr.String(), "FAILED") {
		t.Errorf("expected FAILED in stderr, got: %s", stderr.String())
	}
}

func TestDeploy_NoChanges(t *testing.T) {
	sys := buildCleanFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy clean state exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "no changes needed") {
		t.Errorf("expected 'no changes needed', got: %s", stdout.String())
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no ops applied, got: %v", sys.appliedOps)
	}
}

func TestDeploy_DryRun(t *testing.T) {
	sys := buildDriftFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir, dryRun: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy --dry-run exit code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "planned changes") {
		t.Errorf("expected planned changes in output, got: %s", out)
	}
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected dry-run message, got: %s", out)
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops in dry-run, got: %v", sys.appliedOps)
	}
}

func TestDeploy_DryRunReportsInactivePeerRemoval(t *testing.T) {
	sys := buildInactivePeerFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployInactiveBobTestData(h, dir)
	gf := &globalFlags{configDir: dir, dryRun: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy --dry-run inactive peer exit code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "remove wg peer bob BOB_PUB=") {
		t.Errorf("expected inactive peer removal in output, got: %s", out)
	}
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected dry-run message, got: %s", out)
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops in dry-run, got: %v", sys.appliedOps)
	}
}

func TestDeploy_YesFlag(t *testing.T) {
	sys := buildDriftFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir, yes: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy --yes exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "applied") {
		t.Errorf("expected 'applied' in output, got: %s", stdout.String())
	}
	if len(sys.appliedOps) == 0 {
		t.Error("expected applied ops, got none")
	}
}

func TestDeploy_ConfirmationYes(t *testing.T) {
	sys := buildDriftFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "y\n")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy with 'y' confirmation exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "applied") {
		t.Errorf("expected 'applied' in output, got: %s", stdout.String())
	}
	if len(sys.appliedOps) == 0 {
		t.Error("expected applied ops after confirmation, got none")
	}
}

func TestDeploy_ConfirmationRejected(t *testing.T) {
	sys := buildDriftFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "n\n")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy with 'n' confirmation exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("expected 'aborted' in output, got: %s", stdout.String())
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops after abort, got: %v", sys.appliedOps)
	}
}

func TestDeploy_ConfirmationRejectedAppliesNoInactiveCleanup(t *testing.T) {
	sys := buildInactivePeerAndIPSetDriftFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "n\n")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployInactiveBobTestData(h, dir)
	gf := &globalFlags{configDir: dir}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy inactive cleanup rejected exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("expected aborted output, got: %s", stdout.String())
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops after abort, got: %v", sys.appliedOps)
	}
}

func TestDeploy_AppliesAddDelta(t *testing.T) {
	sys := buildDriftFakeSystem()
	app, _, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir, yes: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy exit code = %d, want 0", code)
	}
	// alice's sandbox entry should have been added to the matrix set.
	found := false
	for _, op := range sys.appliedOps {
		if strings.HasPrefix(op, "add:wg_allow_matrix:10.8.0.10,192.168.122.100:") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected add op for alice's sandbox, got: %v", sys.appliedOps)
	}
}

func TestDeploy_AppliesDeleteDelta(t *testing.T) {
	sys := buildCleanFakeSystem()
	// Inject an extra matrix entry that is not in db.yaml.
	sys.ipsetResults["wg_allow_matrix"] =
		"create wg_allow_matrix hash:net,net family inet comment\n" +
			"add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n" +
			"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n" +
			"add wg_allow_matrix 10.8.0.20,192.168.122.100 comment \"ghost -> sandbox\"\n"
	app, _, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir, yes: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy exit code = %d, want 0", code)
	}
	found := false
	for _, op := range sys.appliedOps {
		if op == "del:wg_allow_matrix:10.8.0.20,192.168.122.100" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected delete op for ghost entry, got: %v", sys.appliedOps)
	}
}

func TestDeploy_AppliesInactivePeerRemoval(t *testing.T) {
	sys := buildInactivePeerFakeSystem()
	app, _, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployInactiveBobTestData(h, dir)
	gf := &globalFlags{configDir: dir, yes: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy inactive peer cleanup exit code = %d, want 0", code)
	}
	found := false
	for _, op := range sys.appliedOps {
		if op == "wgdel:wg0:BOB_PUB=" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected WireGuard peer removal for inactive bob, got: %v", sys.appliedOps)
	}
}

func TestDeploy_DryRunReportsActivePeerAdd(t *testing.T) {
	sys := buildInactiveBobAbsentFakeSystem()
	app, stdout, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir, dryRun: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy --dry-run active peer add exit code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "add wg peer bob BOB_PUB= 10.8.0.15") {
		t.Errorf("expected active peer add in output, got: %s", out)
	}
	if !strings.Contains(out, "add wg_allow_matrix 10.8.0.15,192.168.122.100") ||
		!strings.Contains(out, "add wg_allow_matrix 10.8.0.15,192.168.122.101") {
		t.Errorf("expected bob ipset adds in output, got: %s", out)
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no applied ops in dry-run, got: %v", sys.appliedOps)
	}
}

func TestDeploy_AppliesActivePeerAddBeforeIPSetAdds(t *testing.T) {
	sys := buildInactiveBobAbsentFakeSystem()
	app, _, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir, yes: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy active peer add exit code = %d, want 0", code)
	}
	wantOps := []string{
		"wgset:wg0:BOB_PUB=:10.8.0.15",
		"add:wg_allow_matrix:10.8.0.15,192.168.122.100:bob -> sandbox",
		"add:wg_allow_matrix:10.8.0.15,192.168.122.101:bob -> mailvm",
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
	wgIndex := -1
	firstAddIndex := -1
	for i, op := range sys.appliedOps {
		if op == "wgset:wg0:BOB_PUB=:10.8.0.15" {
			wgIndex = i
		}
		if strings.HasPrefix(op, "add:wg_allow_matrix:10.8.0.15,") && firstAddIndex == -1 {
			firstAddIndex = i
		}
	}
	if wgIndex == -1 || firstAddIndex == -1 || wgIndex > firstAddIndex {
		t.Errorf("expected WireGuard add before ipset adds, got: %v", sys.appliedOps)
	}
}

func TestDeploy_AppliesInactiveIPSetDeletesBeforePeerRemoval(t *testing.T) {
	sys := buildInactivePeerAndIPSetDriftFakeSystem()
	app, _, _ := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployInactiveBobTestData(h, dir)
	gf := &globalFlags{configDir: dir, yes: true}
	code := cmdDeploy(gf, nil, app)
	if code != 0 {
		t.Fatalf("deploy inactive cleanup exit code = %d, want 0", code)
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
	wgIndex := -1
	lastDelIndex := -1
	for i, op := range sys.appliedOps {
		if strings.HasPrefix(op, "del:wg_allow_matrix:10.8.0.15,") && i > lastDelIndex {
			lastDelIndex = i
		}
		if op == "wgdel:wg0:BOB_PUB=" {
			wgIndex = i
		}
	}
	if wgIndex == -1 || lastDelIndex == -1 || wgIndex < lastDelIndex {
		t.Errorf("expected ipset deletes before WireGuard removal, got: %v", sys.appliedOps)
	}
}

func TestDeploy_PeerRemovalFailure(t *testing.T) {
	sys := buildInactivePeerFakeSystem()
	sys.wgDelErr = fmt.Errorf("wg: operation failed")
	app, _, stderr := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployInactiveBobTestData(h, dir)
	gf := &globalFlags{configDir: dir, yes: true}
	code := cmdDeploy(gf, nil, app)
	if code == 0 {
		t.Fatal("deploy should return non-zero when peer removal fails")
	}
	if !strings.Contains(stderr.String(), "operation failed") {
		t.Errorf("expected peer removal error, got: %s", stderr.String())
	}
}

func TestDeploy_ApplyError(t *testing.T) {
	sys := buildDriftFakeSystem()
	sys.ipsetAddErr = fmt.Errorf("ipset: resource temporarily unavailable")
	app, _, stderr := makeDeployApp(sys, "")
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)
	gf := &globalFlags{configDir: dir, yes: true}
	code := cmdDeploy(gf, nil, app)
	if code == 0 {
		t.Error("deploy should return non-zero when apply fails")
	}
	if !strings.Contains(stderr.String(), "resource temporarily unavailable") {
		t.Errorf("expected error message in stderr, got: %s", stderr.String())
	}
}
