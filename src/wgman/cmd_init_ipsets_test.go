package main

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// makeInitIPSetsApp builds an App with a root fake system and captured I/O.
func makeInitIPSetsApp(sys *fakeSystem) (*App, *strings.Builder, *strings.Builder) {
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	return &App{
		Sys:    sys,
		Stdin:  strings.NewReader(""),
		Stdout: stdout,
		Stderr: stderr,
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, stdout, stderr
}

func writeInitIPSetsConfig(h *testHelper, dir string) {
	h.writeFile(dir, "config.yaml", `
interface: wg0
sets:
  all: wg_allow_all
  ip_matrix: wg_allow_matrix
  port_matrix: wg_allow_matrix_ports
`)
}

func TestInitIPSets_CreatesAllAccessSet(t *testing.T) {
	sys := newFakeSystem()
	app, _, _ := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	gf := &globalFlags{configDir: dir}
	code := cmdInitIPSets(gf, nil, app)
	if code != 0 {
		t.Fatalf("init-ipsets exit code = %d, want 0", code)
	}

	found := false
	for _, op := range sys.appliedOps {
		if op == "create:wg_allow_all:hash:ip:nocomment" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected all-access set creation, ops: %v", sys.appliedOps)
	}
}

func TestInitIPSets_CreatesMatrixSet(t *testing.T) {
	sys := newFakeSystem()
	app, _, _ := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	gf := &globalFlags{configDir: dir}
	code := cmdInitIPSets(gf, nil, app)
	if code != 0 {
		t.Fatalf("init-ipsets exit code = %d, want 0", code)
	}

	found := false
	for _, op := range sys.appliedOps {
		if op == "create:wg_allow_matrix:hash:net,net:comment" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected matrix set creation with comments, ops: %v", sys.appliedOps)
	}
}

func TestInitIPSets_CreatesPortMatrixSet(t *testing.T) {
	sys := newFakeSystem()
	app, _, _ := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	gf := &globalFlags{configDir: dir}
	code := cmdInitIPSets(gf, nil, app)
	if code != 0 {
		t.Fatalf("init-ipsets exit code = %d, want 0", code)
	}

	found := false
	for _, op := range sys.appliedOps {
		if op == "create:wg_allow_matrix_ports:hash:ip,port,ip:comment" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected port matrix set creation with comments, ops: %v", sys.appliedOps)
	}
}

func TestInitIPSets_LoadsSetNamesFromConfig(t *testing.T) {
	sys := newFakeSystem()
	app, stdout, _ := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	h.writeFile(dir, "config.yaml", `
interface: wg0
sets:
  all: custom_all_set
  ip_matrix: custom_matrix_set
  port_matrix: custom_port_matrix_set
`)

	gf := &globalFlags{configDir: dir}
	code := cmdInitIPSets(gf, nil, app)
	if code != 0 {
		t.Fatalf("init-ipsets exit code = %d, want 0", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "custom_all_set") {
		t.Errorf("expected custom_all_set in output, got: %s", out)
	}
	if !strings.Contains(out, "custom_matrix_set") {
		t.Errorf("expected custom_matrix_set in output, got: %s", out)
	}
	if !strings.Contains(out, "custom_port_matrix_set") {
		t.Errorf("expected custom_port_matrix_set in output, got: %s", out)
	}

	expected := []string{
		"create:custom_all_set:hash:ip:nocomment",
		"create:custom_matrix_set:hash:net,net:comment",
		"create:custom_port_matrix_set:hash:ip,port,ip:comment",
	}
	for _, want := range expected {
		found := false
		for _, op := range sys.appliedOps {
			if op == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected op %q, ops: %v", want, sys.appliedOps)
		}
	}
}

func TestInitIPSets_RequiresRoot(t *testing.T) {
	sys := newFakeSystem()
	sys.isRoot = false
	app, _, stderr := makeInitIPSetsApp(sys)

	gf := &globalFlags{configDir: "testdata/valid-offline"}
	code := cmdInitIPSets(gf, nil, app)
	if code == 0 {
		t.Error("init-ipsets should return non-zero when not root")
	}
	if !strings.Contains(stderr.String(), "root") {
		t.Errorf("expected root error, got: %s", stderr.String())
	}
}

func TestInitIPSets_RejectsMissingConfig(t *testing.T) {
	sys := newFakeSystem()
	app, _, stderr := makeInitIPSetsApp(sys)

	gf := &globalFlags{configDir: "testdata/nonexistent"}
	code := cmdInitIPSets(gf, nil, app)
	if code == 0 {
		t.Error("init-ipsets should return non-zero for missing config")
	}
	if stderr.String() == "" {
		t.Error("expected error message on stderr")
	}
}

func TestInitIPSets_RejectsPositionalArgs(t *testing.T) {
	sys := newFakeSystem()
	app, _, _ := makeInitIPSetsApp(sys)

	gf := &globalFlags{configDir: "testdata/valid-offline"}
	code := cmdInitIPSets(gf, []string{"extra"}, app)
	if code == 0 {
		t.Error("init-ipsets should reject positional arguments")
	}
}

func TestInitIPSets_IdempotentViaFakeAdapter(t *testing.T) {
	// Calling init-ipsets twice on a fake adapter should succeed both times:
	// the fake never fails, mirroring the real -exist flag behavior.
	sys := newFakeSystem()
	app, _, _ := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	gf := &globalFlags{configDir: dir}
	for i := 0; i < 2; i++ {
		code := cmdInitIPSets(gf, nil, app)
		if code != 0 {
			t.Fatalf("run %d: init-ipsets exit code = %d, want 0", i+1, code)
		}
	}
}

func TestInitIPSets_PropagatesCreateError(t *testing.T) {
	sys := newFakeSystem()
	sys.ipsetCreateErr = fmt.Errorf("ipset: kernel error")
	app, _, stderr := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	gf := &globalFlags{configDir: dir}
	code := cmdInitIPSets(gf, nil, app)
	if code == 0 {
		t.Error("init-ipsets should return non-zero when create fails")
	}
	if !strings.Contains(stderr.String(), "kernel error") {
		t.Errorf("expected create error in stderr, got: %s", stderr.String())
	}
}

func TestInitIPSets_FlushesManagedSets(t *testing.T) {
	sys := newFakeSystem()
	app, stdout, stderr := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	code := cmdInitIPSets(&globalFlags{configDir: dir, initIPSetsFlush: true}, nil, app)
	if code != 0 {
		t.Fatalf("init-ipsets --flush exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	wantOps := []string{
		"flush:wg_allow_all",
		"flush:wg_allow_matrix",
		"flush:wg_allow_matrix_ports",
	}
	if strings.Join(sys.appliedOps, "\n") != strings.Join(wantOps, "\n") {
		t.Errorf("flush ops = %v, want %v", sys.appliedOps, wantOps)
	}
	if !strings.Contains(stdout.String(), "flushed") {
		t.Errorf("expected flushed output, got: %s", stdout.String())
	}
}

func TestInitIPSets_DestroyManagedSets(t *testing.T) {
	sys := newFakeSystem()
	app, _, stderr := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	code := cmdInitIPSets(&globalFlags{configDir: dir, initIPSetsDestroy: true}, nil, app)
	if code != 0 {
		t.Fatalf("init-ipsets --destroy exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	wantOps := []string{
		"destroy:wg_allow_all",
		"destroy:wg_allow_matrix",
		"destroy:wg_allow_matrix_ports",
	}
	if strings.Join(sys.appliedOps, "\n") != strings.Join(wantOps, "\n") {
		t.Errorf("destroy ops = %v, want %v", sys.appliedOps, wantOps)
	}
}

func TestInitIPSets_FlushThenDestroyManagedSets(t *testing.T) {
	sys := newFakeSystem()
	app, _, stderr := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	code := cmdInitIPSets(&globalFlags{configDir: dir, initIPSetsFlush: true, initIPSetsDestroy: true}, nil, app)
	if code != 0 {
		t.Fatalf("init-ipsets --flush --destroy exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	wantOps := []string{
		"flush:wg_allow_all",
		"flush:wg_allow_matrix",
		"flush:wg_allow_matrix_ports",
		"destroy:wg_allow_all",
		"destroy:wg_allow_matrix",
		"destroy:wg_allow_matrix_ports",
	}
	if strings.Join(sys.appliedOps, "\n") != strings.Join(wantOps, "\n") {
		t.Errorf("flush/destroy ops = %v, want %v", sys.appliedOps, wantOps)
	}
}

func TestInitIPSets_FlushMissingSetsIsNonFatal(t *testing.T) {
	sys := newFakeSystem()
	sys.ipsetFlushErr = fmt.Errorf("Set cannot be found")
	app, stdout, stderr := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	code := cmdInitIPSets(&globalFlags{configDir: dir, initIPSetsFlush: true}, nil, app)
	if code != 0 {
		t.Fatalf("init-ipsets --flush missing sets exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "already absent") {
		t.Errorf("expected already absent output, got: %s", stdout.String())
	}
}

func TestInitIPSets_DestroyInUseSetFailsWithHint(t *testing.T) {
	sys := newFakeSystem()
	sys.ipsetDestroyErr = fmt.Errorf("Set cannot be destroyed: it is in use by a kernel component")
	app, _, stderr := makeInitIPSetsApp(sys)

	h := newHelper(t)
	dir := h.makeTempDir()
	writeInitIPSetsConfig(h, dir)

	code := cmdInitIPSets(&globalFlags{configDir: dir, initIPSetsDestroy: true}, nil, app)
	if code == 0 {
		t.Fatal("init-ipsets --destroy should fail when set is in use")
	}
	if !strings.Contains(stderr.String(), "wgman-firewall-hook down") {
		t.Errorf("expected firewall hook hint, got: %s", stderr.String())
	}
}

func TestCheck_MissingIPSetHintsInitIPSets(t *testing.T) {
	sys := buildCleanFakeSystem()
	sys.ipsetErrs["wg_allow_all"] = fmt.Errorf("set does not exist")

	result := Check(makeTestCfg(), makeTestDB(), sys)
	found := false
	for _, e := range result.HardErrors {
		if strings.Contains(e, "init-ipsets") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'init-ipsets' hint in check error, got: %v", result.HardErrors)
	}
}

// Ensure runApp correctly dispatches to init-ipsets via the command switch.
func TestRunApp_InitIPSetsNotRoot(t *testing.T) {
	sys := newFakeSystem()
	sys.isRoot = false
	var errBuf strings.Builder
	app := &App{
		Sys:    sys,
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: &errBuf,
		Now:    func() time.Time { return time.Unix(0, 0) },
	}
	code := runApp([]string{"init-ipsets", "--config-dir", "testdata/valid-offline"}, app)
	if code == 0 {
		t.Error("expected non-zero exit when not root")
	}
	if !strings.Contains(errBuf.String(), "root") {
		t.Errorf("expected root error, got: %s", errBuf.String())
	}
}
