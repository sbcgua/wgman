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
