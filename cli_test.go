package main

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRun_Help(t *testing.T) {
	code := run([]string{"help"})
	if code != 0 {
		t.Errorf("help exit code = %d, want 0", code)
	}
}

func TestRun_HelpFlag(t *testing.T) {
	code := run([]string{"-h"})
	if code != 0 {
		t.Errorf("-h exit code = %d, want 0", code)
	}
}

func TestRun_NoArgs(t *testing.T) {
	code := run([]string{})
	if code != 0 {
		t.Errorf("no args exit code = %d, want 0", code)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	code := run([]string{"bogus"})
	if code == 0 {
		t.Errorf("unknown command should return non-zero exit code")
	}
}

func TestRunApp_HelpOutputSmoke(t *testing.T) {
	app := makeFakeApp(false)
	var stdout strings.Builder
	app.Stdout = &stdout
	code := runApp([]string{"help"}, app)
	if code != 0 {
		t.Fatalf("help exit code = %d, want 0", code)
	}
	out := stdout.String()
	for _, want := range []string{"Usage:", "Commands:", "deploy", "create <name>", "Alias for create", "--dry-run", "--no-color"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestAppIsStdoutTTYDelegatesConfiguredStdout(t *testing.T) {
	sys := newFakeSystem()
	sys.isTerminal = true
	var stdout strings.Builder
	app := &App{Sys: sys, Stdout: &stdout}

	if !app.IsStdoutTTY() {
		t.Fatal("IsStdoutTTY() = false, want true")
	}
	if sys.terminalWriter != app.Stdout {
		t.Error("IsStdoutTTY() did not inspect App.Stdout")
	}
}

func TestRunApp_ShowNoColorSuppressesTTYColor(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)

	sys := buildCleanFakeSystem()
	app, stdout, stderr := makeDeployApp(sys, "")
	app.Now = func() time.Time { return time.Unix(1748001000, 0) }
	sys.isTerminal = true
	code := runApp([]string{"show", "--no-color", "--config-dir", dir}, app)
	if code != 0 {
		t.Fatalf("show --no-color exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("expected no ANSI escapes with --no-color, got:\n%s", stdout.String())
	}
}

func TestRunApp_ShowColorsInteractiveTTY(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)

	sys := buildCleanFakeSystem()
	app, stdout, stderr := makeDeployApp(sys, "")
	app.Now = func() time.Time { return time.Unix(1748001000, 0) }
	sys.isTerminal = true
	code := runApp([]string{"show", "--config-dir", dir}, app)
	if code != 0 {
		t.Fatalf("show TTY exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), ansiGrey+"never"+ansiReset) {
		t.Errorf("expected colorized show output on TTY, got:\n%s", stdout.String())
	}
}

func TestRunApp_ShowNonTTYHasNoColor(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)

	sys := buildCleanFakeSystem()
	app, stdout, stderr := makeDeployApp(sys, "")
	app.Now = func() time.Time { return time.Unix(1748001000, 0) }
	sys.isTerminal = false
	code := runApp([]string{"show", "--config-dir", dir}, app)
	if code != 0 {
		t.Fatalf("show non-TTY exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("expected no ANSI escapes for non-TTY, got:\n%s", stdout.String())
	}
}

func TestRunApp_ListNoColorSuppressesTTYColor(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)

	sys := buildCleanFakeSystem()
	app, stdout, stderr := makeDeployApp(sys, "")
	sys.isTerminal = true
	code := runApp([]string{"list", "--no-color", "--config-dir", dir}, app)
	if code != 0 {
		t.Fatalf("list --no-color exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("expected no ANSI escapes with list --no-color, got:\n%s", stdout.String())
	}
}

func TestRunApp_ListColorsInteractiveTTY(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)

	sys := buildCleanFakeSystem()
	app, stdout, stderr := makeDeployApp(sys, "")
	sys.isTerminal = true
	code := runApp([]string{"list", "--config-dir", dir}, app)
	if code != 0 {
		t.Fatalf("list TTY exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), ansiRed+"*"+ansiReset) {
		t.Errorf("expected colorized list output on TTY, got:\n%s", stdout.String())
	}
}

func TestRunApp_ArgumentErrorsExitTwo(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown command", args: []string{"bogus"}},
		{name: "check positional", args: []string{"check", "alice"}},
		{name: "show positional", args: []string{"show", "alice"}},
		{name: "list too many", args: []string{"list", "alice", "bob"}},
		{name: "init positional", args: []string{"init-ipsets", "extra"}},
		{name: "deploy positional", args: []string{"deploy", "extra"}},
		{name: "create dry-run", args: []string{"create", "--dry-run", "alice"}},
		{name: "remove missing name", args: []string{"remove"}},
		{name: "mod missing expression", args: []string{"mod", "alice"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := makeFakeApp(true)
			code := runApp(tc.args, app)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
		})
	}
}

func TestRunApp_UnsupportedDryRunFlagsExitTwo(t *testing.T) {
	for _, cmd := range []string{"check", "list", "show", "init-ipsets", "create"} {
		t.Run(cmd, func(t *testing.T) {
			app := makeFakeApp(true)
			var stderr strings.Builder
			app.Stderr = &stderr
			args := []string{cmd, "--dry-run"}
			if cmd == "create" {
				args = append(args, "alice")
			}
			code := runApp(args, app)
			if code != 2 {
				t.Fatalf("%s --dry-run exit code = %d, want 2", cmd, code)
			}
			if !strings.Contains(stderr.String(), "does not support --dry-run") {
				t.Errorf("expected unsupported dry-run error, got: %s", stderr.String())
			}
		})
	}
}

// TestRun_CheckNotRoot verifies that wgman check rejects non-root callers.
func TestRun_CheckNotRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root execution")
	}
	code := run([]string{"check", "--config-dir", "testdata/valid-offline"})
	if code == 0 {
		t.Errorf("check as non-root should return non-zero")
	}
}

func TestRun_CheckMissingDir(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root; non-root is tested by TestRun_CheckNotRoot")
	}
	code := run([]string{"check", "--config-dir", "testdata/nonexistent"})
	if code == 0 {
		t.Errorf("check with missing dir should return non-zero")
	}
}

// ---- argument validation tests (use runApp with fake app) ----

// makeFakeApp returns an App with a non-root fake system and discarded I/O,
// suitable for testing argument validation without real system access.
func makeFakeApp(root bool) *App {
	sys := newFakeSystem()
	sys.isRoot = root
	return &App{
		Sys:    sys,
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: io.Discard,
		Now:    func() time.Time { return time.Unix(0, 0) },
	}
}

func TestRunApp_CheckRejectsPositionalArgs(t *testing.T) {
	app := makeFakeApp(false)
	code := runApp([]string{"check", "alice"}, app)
	if code == 0 {
		t.Error("check with positional arg should return non-zero")
	}
}

func TestRunApp_ShowRejectsPositionalArgs(t *testing.T) {
	app := makeFakeApp(false)
	code := runApp([]string{"show", "alice"}, app)
	if code == 0 {
		t.Error("show with positional arg should return non-zero")
	}
}

func TestRunApp_ListRejectsMultiplePositionalArgs(t *testing.T) {
	app := makeFakeApp(false)
	code := runApp([]string{"list", "alice", "bob"}, app)
	if code == 0 {
		t.Error("list with two positional args should return non-zero")
	}
}

func TestRunApp_CheckNotRoot(t *testing.T) {
	app := makeFakeApp(false) // non-root
	var errBuf strings.Builder
	app.Stderr = &errBuf
	code := runApp([]string{"check", "--config-dir", "testdata/valid-offline"}, app)
	if code == 0 {
		t.Error("check as non-root should return non-zero")
	}
	if !strings.Contains(errBuf.String(), "root") {
		t.Errorf("expected root error message, got: %s", errBuf.String())
	}
}

func TestRunApp_ModExpressionMayStartWithDash(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeDeployTestData(h, dir)

	sys := buildCleanFakeSystem()
	app, stdout, stderr := makeDeployApp(sys, "")
	code := runApp([]string{"mod", "--config-dir", dir, "alice", "-sandbox"}, app)
	if code != 0 {
		t.Fatalf("mod with -sandbox exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("-sandbox was parsed as a flag: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "applied") {
		t.Errorf("expected applied output, got: %s", stdout.String())
	}

	found := false
	for _, op := range sys.appliedOps {
		if op == "del:wg_allow_matrix:10.8.0.10,192.168.122.100" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected delete op for alice sandbox, got: %v", sys.appliedOps)
	}
}

func TestRunApp_CreateRejectsDryRun(t *testing.T) {
	app := makeFakeApp(true)
	var errBuf strings.Builder
	app.Stderr = &errBuf
	code := runApp([]string{"create", "--dry-run", "alice"}, app)
	if code != 2 {
		t.Fatalf("create --dry-run exit code = %d, want 2", code)
	}
	if !strings.Contains(errBuf.String(), "does not support --dry-run") {
		t.Errorf("expected unsupported dry-run error, got: %s", errBuf.String())
	}
}

func TestRunApp_CreateStoresComment(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)

	sys := buildCleanFakeSystem()
	sys.genKeyResult = "CAROL_PRIVATE"
	sys.pubKeyResult = "CAROL_PUBLIC="
	app, _, stderr := makeDeployApp(sys, "")
	code := runApp([]string{"create", "-c", "laptop replacement", "--config-dir", dir, "carol"}, app)
	if code != 0 {
		t.Fatalf("create with comment exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}
	if db.Users["carol"].Comment != "laptop replacement" {
		t.Errorf("carol comment = %q, want laptop replacement", db.Users["carol"].Comment)
	}
}

func TestRunApp_CreateRejectsEmptyComment(t *testing.T) {
	app := makeFakeApp(true)
	var stderr strings.Builder
	app.Stderr = &stderr
	code := runApp([]string{"create", "-c", "  ", "alice"}, app)
	if code != 2 {
		t.Fatalf("create empty comment exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "comment must not be empty") {
		t.Errorf("expected empty comment error, got: %s", stderr.String())
	}
}

func TestRunApp_CreateRejectsDuplicateCommentFlag(t *testing.T) {
	app := makeFakeApp(true)
	var stderr strings.Builder
	app.Stderr = &stderr
	code := runApp([]string{"-c", "one", "create", "-c", "two", "alice"}, app)
	if code != 2 {
		t.Fatalf("duplicate create comment exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "specified more than once") {
		t.Errorf("expected duplicate comment error, got: %s", stderr.String())
	}
}

func TestRunApp_CommentFlagRejectedForOtherCommands(t *testing.T) {
	app := makeFakeApp(true)
	var stderr strings.Builder
	app.Stderr = &stderr
	code := runApp([]string{"-c", "note", "check"}, app)
	if code != 2 {
		t.Fatalf("check -c exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "only supported by create/add") {
		t.Errorf("expected unsupported comment flag error, got: %s", stderr.String())
	}
}

func TestRunApp_AddAliasCreatesUser(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	outDir := h.makeTempDir()
	t.Chdir(outDir)
	writeCreateTestData(h, dir)

	sys := buildCleanFakeSystem()
	sys.genKeyResult = "CAROL_PRIVATE"
	sys.pubKeyResult = "CAROL_PUBLIC="
	app, stdout, stderr := makeDeployApp(sys, "")
	code := runApp([]string{"add", "--config-dir", dir, "carol", "-c", "temporary contractor"}, app)
	if code != 0 {
		t.Fatalf("add alias exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "created carol") {
		t.Errorf("expected create success output, got: %s", stdout.String())
	}
	if len(sys.appliedOps) == 0 {
		t.Fatalf("expected live operations for add alias")
	}
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}
	if db.Users["carol"].Comment != "temporary contractor" {
		t.Errorf("carol comment = %q, want temporary contractor", db.Users["carol"].Comment)
	}
}
