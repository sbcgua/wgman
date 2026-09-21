package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPlanRecreateUserPreservesUserMetadata(t *testing.T) {
	db := makeTestDB()
	entry := db.Users["alice"]
	entry.Comment = "rotated laptop"
	db.Users["alice"] = entry
	sys := newFakeSystem()
	sys.genKeyResult = "ALICE_PRIVATE_NEW"
	sys.pubKeyResult = "ALICE_PUBLIC_NEW="

	plan, err := planRecreateUser(db, "alice", sys)
	if err != nil {
		t.Fatalf("planRecreateUser: %v", err)
	}
	got := plan.UpdatedDB.Users["alice"]
	if got.IP != "10.8.0.10" || got.Pub != "ALICE_PUBLIC_NEW=" || got.Comment != "rotated laptop" || got.Inactive {
		t.Errorf("updated alice = %+v, metadata was not preserved", got)
	}
	if strings.Join(plan.UpdatedDB.Access["alice"], ",") != "sandbox" {
		t.Errorf("alice access = %v, want sandbox", plan.UpdatedDB.Access["alice"])
	}
	wantPeerDeltas := []WGPeerDeltaOp{
		{User: "alice", PubKey: "ALICE_PUB=", AllowedIP: "10.8.0.10", Action: WGPeerRemove},
		{User: "alice", PubKey: "ALICE_PUBLIC_NEW=", AllowedIP: "10.8.0.10", Action: WGPeerAdd},
	}
	if !reflect.DeepEqual(plan.PeerDeltas, wantPeerDeltas) {
		t.Errorf("peer deltas = %+v, want %+v", plan.PeerDeltas, wantPeerDeltas)
	}
}

func TestRecreate_RejectsDryRun(t *testing.T) {
	sys := newFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")

	if code := cmdRecreate(&globalFlags{dryRun: true}, []string{"alice"}, app); code != 2 {
		t.Fatalf("recreate --dry-run exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "does not support --dry-run") {
		t.Errorf("error output = %q, want dry-run rejection", stderr.String())
	}
}

func TestRecreate_ReplacesConfigAndActivePeer(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)
	h.writeFile(outDir, "alice.vpn.conf", "old config")
	sys := buildCleanFakeSystem()
	sys.genKeyResult = "ALICE_PRIVATE_NEW"
	sys.pubKeyResult = "ALICE_PUBLIC_NEW="
	app, stdout, _ := makeDeployApp(sys, "")

	if code := cmdRecreate(&globalFlags{configDir: dir}, []string{"alice"}, app); code != 0 {
		t.Fatalf("recreate exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "recreated alice") {
		t.Errorf("success output = %q", stdout.String())
	}
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}
	if got := db.Users["alice"].Pub; got != "ALICE_PUBLIC_NEW=" {
		t.Errorf("alice public key = %q, want replacement", got)
	}
	config, err := os.ReadFile(filepath.Join(outDir, "alice.vpn.conf"))
	if err != nil {
		t.Fatalf("read recreated config: %v", err)
	}
	if !strings.Contains(string(config), "PrivateKey = ALICE_PRIVATE_NEW") {
		t.Errorf("recreated config = %q, missing replacement key", config)
	}
	wantOps := []string{
		"wgdel:wg0:ALICE_PUB=",
		"wgset:wg0:ALICE_PUBLIC_NEW=:10.8.0.10",
	}
	if strings.Join(sys.appliedOps, ",") != strings.Join(wantOps, ",") {
		t.Errorf("live operations = %v, want %v", sys.appliedOps, wantOps)
	}
}

func TestRecreate_SaveFailureRestoresOriginalPeer(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}
	oldSaveDBAtomic := saveDBAtomic
	saveDBAtomic = func(_ string, _ *DB) error { return os.ErrPermission }
	t.Cleanup(func() { saveDBAtomic = oldSaveDBAtomic })

	sys := buildCleanFakeSystem()
	sys.genKeyResult = "ALICE_PRIVATE_NEW"
	sys.pubKeyResult = "ALICE_PUBLIC_NEW="
	app, _, stderr := makeDeployApp(sys, "")
	if code := cmdRecreate(&globalFlags{configDir: dir}, []string{"alice"}, app); code == 0 {
		t.Fatal("recreate should fail when db save fails")
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("db.yaml changed after recreate save failure")
	}
	wantOps := []string{
		"wgdel:wg0:ALICE_PUB=",
		"wgset:wg0:ALICE_PUBLIC_NEW=:10.8.0.10",
		"wgdel:wg0:ALICE_PUBLIC_NEW=",
		"wgset:wg0:ALICE_PUB=:10.8.0.10",
	}
	if strings.Join(sys.appliedOps, ",") != strings.Join(wantOps, ",") {
		t.Errorf("live operations = %v, want %v", sys.appliedOps, wantOps)
	}
	if _, err := os.Stat(filepath.Join(outDir, "alice.vpn.conf")); !os.IsNotExist(err) {
		t.Fatalf("replacement config should be removed after failure, stat err: %v", err)
	}
	if !strings.Contains(stderr.String(), "permission") {
		t.Errorf("failure output = %q, want permission error", stderr.String())
	}
}

func TestRecreate_InactiveUserDoesNotChangeLiveState(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)
	dbPath := filepath.Join(dir, "db.yaml")
	dbData, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db fixture: %v", err)
	}
	dbData = []byte(strings.Replace(string(dbData), "    pub: BOB_PUB=\n", "    pub: BOB_PUB=\n    inactive: true\n", 1))
	if err := os.WriteFile(dbPath, dbData, 0600); err != nil {
		t.Fatalf("write inactive db fixture: %v", err)
	}
	sys := buildCleanFakeSystem()
	sys.wgDumpResult = strings.Replace(sys.wgDumpResult,
		"BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n", "", 1)
	sys.ipsetResults["wg_allow_matrix"] = strings.ReplaceAll(
		sys.ipsetResults["wg_allow_matrix"],
		"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n"+
			"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n", "",
	)
	sys.genKeyResult = "BOB_PRIVATE_NEW"
	sys.pubKeyResult = "BOB_PUBLIC_NEW="
	app, _, _ := makeDeployApp(sys, "")

	if code := cmdRecreate(&globalFlags{configDir: dir}, []string{"bob"}, app); code != 0 {
		t.Fatalf("recreate inactive user exit code = %d, want 0", code)
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("inactive recreation changed live state: %v", sys.appliedOps)
	}
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}
	if entry := db.Users["bob"]; entry.Pub != "BOB_PUBLIC_NEW=" || !entry.Inactive {
		t.Errorf("bob = %+v, want inactive user with replacement key", entry)
	}
}
