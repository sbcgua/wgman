package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCreateTestData(h *testHelper, dir string) {
	writeValidTestData(h, dir)
	h.writeFile(dir, "user.conf.template", `[Interface]
PrivateKey = $CLIENT_PRIVATE_KEY
Address = $CLIENT_VPN_IP/32

[Peer]
PublicKey = $SERVER_PUBLIC_KEY
`)
}

func TestParseCreateArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		comment     string
		commentSet  bool
		wantName    string
		wantIP      string
		wantAccess  string
		wantComment string
	}{
		{name: "name only", args: []string{"carol"}, wantName: "carol"},
		{name: "supplied ip", args: []string{"carol", "10.8.0.20"}, wantName: "carol", wantIP: "10.8.0.20"},
		{name: "supplied cidr ip", args: []string{"carol", "10.8.0.20/32"}, wantName: "carol", wantIP: "10.8.0.20"},
		{name: "access without ip", args: []string{"carol", "sandbox,mailvm"}, wantName: "carol", wantAccess: "mailvm,sandbox"},
		{name: "resource access without ip", args: []string{"carol", "ssh@sandbox"}, wantName: "carol", wantAccess: "ssh@sandbox"},
		{name: "ip and access", args: []string{"carol", "10.8.0.20", "sandbox"}, wantName: "carol", wantIP: "10.8.0.20", wantAccess: "sandbox"},
		{name: "comment before name", args: []string{"-c", " laptop replacement ", "carol"}, wantName: "carol", wantComment: "laptop replacement"},
		{name: "comment after name", args: []string{"carol", "-c=temporary contractor"}, wantName: "carol", wantComment: "temporary contractor"},
		{name: "global comment", args: []string{"carol"}, comment: "manual approval", commentSet: true, wantName: "carol", wantComment: "manual approval"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCreateArgs(tc.args, tc.comment, tc.commentSet)
			if err != nil {
				t.Fatalf("parseCreateArgs: %v", err)
			}
			if got.Name != tc.wantName || got.IP != tc.wantIP || strings.Join(got.Access, ",") != tc.wantAccess || got.Comment != tc.wantComment {
				t.Errorf("parseCreateArgs = %+v, want name=%q ip=%q access=%q comment=%q", got, tc.wantName, tc.wantIP, tc.wantAccess, tc.wantComment)
			}
		})
	}
}

func TestParseCreateArgs_Invalid(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no args", args: nil},
		{name: "too many", args: []string{"carol", "10.8.0.20", "sandbox", "extra"}},
		{name: "invalid username", args: []string{"bad name"}},
		{name: "invalid access", args: []string{"carol", "sand box"}},
		{name: "duplicate access", args: []string{"carol", "sandbox,sandbox"}},
		{name: "third arg without ip", args: []string{"carol", "sandbox", "mailvm"}},
		{name: "non slash32 cidr", args: []string{"carol", "10.8.0.0/24", "sandbox"}},
		{name: "empty comment", args: []string{"-c", "", "carol"}},
		{name: "whitespace comment", args: []string{"carol", "-c", "  "}},
		{name: "missing comment value", args: []string{"carol", "-c"}},
		{name: "duplicate comment", args: []string{"-c", "one", "carol", "-c", "two"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseCreateArgs(tc.args, "", false); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestAllocateNextUserIP(t *testing.T) {
	got, err := allocateNextUserIP("10.8.0.1/24", makeTestDB())
	if err != nil {
		t.Fatalf("allocateNextUserIP: %v", err)
	}
	if got != "10.8.0.16" {
		t.Errorf("allocated IP = %q, want 10.8.0.16", got)
	}
}

func TestAllocateNextUserIP_NoUsableAddresses(t *testing.T) {
	_, err := allocateNextUserIP("10.8.0.1/32", makeTestDB())
	if err == nil || !strings.Contains(err.Error(), "no usable") {
		t.Fatalf("error = %v, want no usable addresses", err)
	}
}

func TestPlanCreateUser_SuppliedIPAndAccess(t *testing.T) {
	args := &createArgs{Name: "carol", IP: "10.8.0.20", Access: []string{"mailvm"}, Comment: "temporary contractor"}
	sys := newFakeSystem()
	sys.subnetResult = "10.8.0.1/24"
	sys.genKeyResult = "CAROL_PRIV"
	sys.pubKeyResult = "CAROL_PUB="

	plan, err := planCreateUser(makeTestCfg(), makeTestDB(), args, sys)
	if err != nil {
		t.Fatalf("planCreateUser: %v", err)
	}
	if plan.ClientIP != "10.8.0.20" {
		t.Errorf("clientIP = %q, want 10.8.0.20", plan.ClientIP)
	}
	if plan.PrivateKey != "CAROL_PRIV" || plan.PubKey != "CAROL_PUB=" {
		t.Errorf("generated plan values = priv %q pub %q", plan.PrivateKey, plan.PubKey)
	}
	if plan.UpdatedDB.Users["carol"].Pub != "CAROL_PUB=" {
		t.Errorf("carol pub not saved")
	}
	if plan.UpdatedDB.Users["carol"].Comment != "temporary contractor" {
		t.Errorf("carol comment = %q, want temporary contractor", plan.UpdatedDB.Users["carol"].Comment)
	}
	if strings.Join(plan.UpdatedDB.Access["carol"], ",") != "mailvm" {
		t.Errorf("carol access = %v, want mailvm", plan.UpdatedDB.Access["carol"])
	}
	if len(plan.IPSetDeltas) != 1 || !plan.IPSetDeltas[0].Add || plan.IPSetDeltas[0].Entry != "10.8.0.20,192.168.122.101" {
		t.Errorf("ipset deltas = %+v, want one mailvm add", plan.IPSetDeltas)
	}
}

func TestPlanCreateUser_ResourceAccessCreatesPortDelta(t *testing.T) {
	db := makeTestDB()
	db.Resources = map[string]ResourceEntry{
		"ssh@sandbox": {VM: "sandbox", Ports: ResourcePorts{{Protocol: "tcp", Port: 22}}},
	}
	args := &createArgs{Name: "carol", IP: "10.8.0.20", Access: []string{"ssh@sandbox"}}
	sys := newFakeSystem()
	sys.subnetResult = "10.8.0.1/24"
	sys.pubKeyResult = "CAROL_PUB="

	plan, err := planCreateUser(makeTestCfg(), db, args, sys)
	if err != nil {
		t.Fatalf("planCreateUser: %v", err)
	}
	if len(plan.IPSetDeltas) != 1 {
		t.Fatalf("len(ipset deltas) = %d, want 1: %+v", len(plan.IPSetDeltas), plan.IPSetDeltas)
	}
	delta := plan.IPSetDeltas[0]
	if !delta.Add || delta.Set != "wg_allow_matrix_ports" || delta.Entry != "10.8.0.20,tcp:22,192.168.122.100" {
		t.Errorf("delta = %+v, want port matrix add", delta)
	}
}

func TestPlanCreateUser_RejectsDuplicateAndCaseConflict(t *testing.T) {
	tests := []struct {
		name string
		user string
		want string
	}{
		{name: "duplicate", user: "alice", want: "already exists"},
		{name: "case conflict", user: "Alice", want: "conflicts"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := planCreateUser(makeTestCfg(), makeTestDB(), &createArgs{Name: tc.user}, newFakeSystem())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestPlanCreateUser_RejectsUnknownAccessTarget(t *testing.T) {
	sys := newFakeSystem()
	sys.subnetResult = "10.8.0.1/24"
	sys.pubKeyResult = "PUB="
	_, err := planCreateUser(makeTestCfg(), makeTestDB(), &createArgs{Name: "carol", Access: []string{"unknown"}}, sys)
	if err == nil || !strings.Contains(err.Error(), "unknown access target") {
		t.Fatalf("error = %v, want unknown access target", err)
	}
}

func TestPlanCreateUser_RejectsIPAndPubConflicts(t *testing.T) {
	tests := []struct {
		name   string
		args   *createArgs
		pubKey string
		want   string
	}{
		{name: "duplicate ip", args: &createArgs{Name: "carol", IP: "10.8.0.10"}, pubKey: "CAROL_PUB=", want: "conflicts"},
		{name: "duplicate pub", args: &createArgs{Name: "carol", IP: "10.8.0.20"}, pubKey: "ALICE_PUB=", want: "public key conflicts"},
		{name: "outside subnet", args: &createArgs{Name: "carol", IP: "10.9.0.20"}, pubKey: "CAROL_PUB=", want: "outside interface subnet"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sys := newFakeSystem()
			sys.subnetResult = "10.8.0.1/24"
			sys.pubKeyResult = tc.pubKey
			_, err := planCreateUser(makeTestCfg(), makeTestDB(), tc.args, sys)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestCreate_RefusesPreExistingDrift(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeCreateTestData(h, dir)
	sys := buildDriftFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdCreate(&globalFlags{configDir: dir}, []string{"carol"}, app)
	if code == 0 {
		t.Fatal("create should fail on pre-existing drift")
	}
	if !strings.Contains(stderr.String(), "ipset drift") {
		t.Errorf("expected drift error, got: %s", stderr.String())
	}
}

func TestCreate_RejectsDryRun(t *testing.T) {
	sys := newFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdCreate(&globalFlags{configDir: "testdata/valid-offline", dryRun: true}, []string{"carol"}, app)
	if code != 2 {
		t.Fatalf("create --dry-run exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "does not support --dry-run") {
		t.Errorf("expected unsupported dry-run error, got: %s", stderr.String())
	}
}

func TestCreate_RefusesExistingClientConfig(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)
	h.writeFile(outDir, "carol.vpn.conf", "existing")
	sys := buildCleanFakeSystem()
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdCreate(&globalFlags{configDir: dir}, []string{"carol"}, app)
	if code == 0 {
		t.Fatal("create should fail when client config exists")
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("expected already exists error, got: %s", stderr.String())
	}
}

func TestCreate_ClientConfigWriteFailureDoesNotWriteDBOrApply(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}

	oldWriteClientConfig := writeClientConfig
	writeClientConfig = func(_, _ string) error {
		return os.ErrPermission
	}
	t.Cleanup(func() { writeClientConfig = oldWriteClientConfig })

	sys := buildCleanFakeSystem()
	sys.genKeyResult = "CAROL_PRIVATE"
	sys.pubKeyResult = "CAROL_PUBLIC="
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdCreate(&globalFlags{configDir: dir}, []string{"carol", "mailvm"}, app)
	if code == 0 {
		t.Fatal("create should fail when client config write fails")
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("db.yaml changed after client config write failure")
	}
	if len(sys.appliedOps) != 0 {
		t.Errorf("expected no live ops after client config write failure, got: %v", sys.appliedOps)
	}
	if !strings.Contains(stderr.String(), "permission") {
		t.Errorf("expected permission error, got: %s", stderr.String())
	}
}

func TestCreate_WGSetFailureRemovesConfigAndDoesNotWriteDB(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}

	sys := buildCleanFakeSystem()
	sys.genKeyResult = "CAROL_PRIVATE"
	sys.pubKeyResult = "CAROL_PUBLIC="
	sys.wgSetErr = os.ErrPermission
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdCreate(&globalFlags{configDir: dir}, []string{"carol", "mailvm"}, app)
	if code == 0 {
		t.Fatal("create should fail when WGSetPeer fails")
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("db.yaml changed after WGSetPeer failure")
	}
	if _, err := os.Stat(filepath.Join(outDir, "carol.vpn.conf")); !os.IsNotExist(err) {
		t.Fatalf("generated config should be removed after WGSetPeer failure, stat err: %v", err)
	}
	for _, op := range sys.appliedOps {
		if strings.HasPrefix(op, "wgdel:") {
			t.Errorf("failed peer add should not be rolled back as completed state, got: %v", sys.appliedOps)
		}
	}
	if !strings.Contains(stderr.String(), "permission") {
		t.Errorf("expected permission error, got: %s", stderr.String())
	}
}

func TestCreate_IPSetFailureRollsBackWireGuardAndConfig(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)
	before, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}

	sys := buildCleanFakeSystem()
	sys.genKeyResult = "CAROL_PRIVATE"
	sys.pubKeyResult = "CAROL_PUBLIC="
	sys.ipsetAddErr = os.ErrPermission
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdCreate(&globalFlags{configDir: dir}, []string{"carol", "mailvm"}, app)
	if code == 0 {
		t.Fatal("create should fail when ipset add fails")
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("db.yaml changed after ipset add failure")
	}
	if _, err := os.Stat(filepath.Join(outDir, "carol.vpn.conf")); !os.IsNotExist(err) {
		t.Fatalf("generated config should be removed after ipset add failure, stat err: %v", err)
	}
	wantOps := []string{
		"wgset:wg0:CAROL_PUBLIC=:10.8.0.16",
		"wgdel:wg0:CAROL_PUBLIC=",
	}
	for _, want := range wantOps {
		found := false
		for _, op := range sys.appliedOps {
			if op == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing rollback op %q from %v", want, sys.appliedOps)
		}
	}
	if !strings.Contains(stderr.String(), "permission") {
		t.Errorf("expected permission error, got: %s", stderr.String())
	}
}

func TestCreate_SaveDBFailureRollsBackLiveStateAndConfig(t *testing.T) {
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
	saveDBAtomic = func(_ string, _ *DB) error {
		return os.ErrPermission
	}
	t.Cleanup(func() { saveDBAtomic = oldSaveDBAtomic })

	sys := buildCleanFakeSystem()
	sys.genKeyResult = "CAROL_PRIVATE"
	sys.pubKeyResult = "CAROL_PUBLIC="
	app, _, stderr := makeDeployApp(sys, "")
	code := cmdCreate(&globalFlags{configDir: dir}, []string{"carol", "mailvm"}, app)
	if code == 0 {
		t.Fatal("create should fail when db save fails")
	}
	after, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("db.yaml changed after db save failure")
	}
	if _, err := os.Stat(filepath.Join(outDir, "carol.vpn.conf")); !os.IsNotExist(err) {
		t.Fatalf("generated config should be removed after db save failure, stat err: %v", err)
	}
	wantOps := []string{
		"wgset:wg0:CAROL_PUBLIC=:10.8.0.16",
		"add:wg_allow_matrix:10.8.0.16,192.168.122.101:carol -> mailvm",
		"del:wg_allow_matrix:10.8.0.16,192.168.122.101",
		"wgdel:wg0:CAROL_PUBLIC=",
	}
	for _, want := range wantOps {
		found := false
		for _, op := range sys.appliedOps {
			if op == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing rollback op %q from %v", want, sys.appliedOps)
		}
	}
	if !strings.Contains(stderr.String(), "permission") {
		t.Errorf("expected permission error, got: %s", stderr.String())
	}
}

func TestCreate_SuccessWritesDBConfigAndAppliesOps(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)

	sys := buildCleanFakeSystem()
	sys.genKeyResult = "CAROL_PRIVATE"
	sys.pubKeyResult = "CAROL_PUBLIC="
	app, stdout, _ := makeDeployApp(sys, "")
	code := cmdCreate(&globalFlags{configDir: dir, createComment: "laptop replacement", createCommentSet: true}, []string{"carol", "mailvm"}, app)
	if code != 0 {
		t.Fatalf("create exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "created carol") {
		t.Errorf("expected created output, got: %s", stdout.String())
	}

	dbData, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db.yaml: %v", err)
	}
	dbText := string(dbData)
	if !strings.Contains(dbText, "carol:\n    ip: 10.8.0.16\n    pub: CAROL_PUBLIC=\n") {
		t.Errorf("expected carol in db.yaml, got:\n%s", dbText)
	}
	if !strings.Contains(dbText, "comment: laptop replacement") {
		t.Errorf("expected carol comment in db.yaml, got:\n%s", dbText)
	}
	if strings.Contains(dbText, "CAROL_PRIVATE") {
		t.Errorf("private key leaked into db.yaml:\n%s", dbText)
	}

	configData, err := os.ReadFile(filepath.Join(outDir, "carol.vpn.conf"))
	if err != nil {
		t.Fatalf("read generated client config: %v", err)
	}
	configText := string(configData)
	for _, want := range []string{"PrivateKey = CAROL_PRIVATE", "Address = 10.8.0.16/32", "PublicKey = SERVER_PUB="} {
		if !strings.Contains(configText, want) {
			t.Errorf("generated config missing %q:\n%s", want, configText)
		}
	}

	wantOps := []string{
		"wgset:wg0:CAROL_PUBLIC=:10.8.0.16",
		"add:wg_allow_matrix:10.8.0.16,192.168.122.101:carol -> mailvm",
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
