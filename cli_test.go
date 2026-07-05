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
	for _, want := range []string{"Usage:", "Commands:", "deploy", "create <name>", "Alias for create", "--dry-run"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q:\n%s", want, out)
		}
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
	code := runApp([]string{"add", "--config-dir", dir, "carol"}, app)
	if code != 0 {
		t.Fatalf("add alias exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "created carol") {
		t.Errorf("expected create success output, got: %s", stdout.String())
	}
	if len(sys.appliedOps) == 0 {
		t.Fatalf("expected live operations for add alias")
	}
}
