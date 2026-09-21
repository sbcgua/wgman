package main

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRunPing_AllVMs(t *testing.T) {
	db := makeTestDB()
	sys := newFakeSystem()
	sys.pingErrs["192.168.122.101"] = errors.New("unreachable")
	var stdout strings.Builder

	code := runPing(db, sys, "", &stdout, io.Discard)
	if code != 1 {
		t.Fatalf("code = %d, want 1 when a VM is down", code)
	}
	if got, want := stdout.String(), "mailvm: DOWN\nsandbox: UP\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if got, want := strings.Join(sys.pingedHosts, ","), "192.168.122.101,192.168.122.100"; got != want {
		t.Errorf("pinged hosts = %q, want %q", got, want)
	}
}

func TestRunPing_OneVMAndUnknownVM(t *testing.T) {
	db := makeTestDB()
	sys := newFakeSystem()
	var stdout, stderr strings.Builder

	if code := runPing(db, sys, "sandbox", &stdout, &stderr); code != 0 {
		t.Fatalf("sandbox code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if got, want := stdout.String(), "sandbox: UP\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}

	stdout.Reset()
	if code := runPing(db, sys, "missing", &stdout, &stderr); code != 1 {
		t.Fatalf("missing VM code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("unexpected stdout for unknown VM: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), `VM "missing" does not exist`) {
		t.Errorf("unknown VM error = %q", stderr.String())
	}
}

func TestCmdPing_ColorAndNoColor(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeValidTestData(h, dir)
	sys := newFakeSystem()
	sys.isTerminal = true
	sys.pingErrs["192.168.122.101"] = errors.New("unreachable")

	var stdout, stderr strings.Builder
	app := &App{Sys: sys, Stdout: &stdout, Stderr: &stderr}
	if code := runApp([]string{"ping", "--config-dir", dir}, app); code != 1 {
		t.Fatalf("colored ping code = %d, want 1; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), ansiGreen+"UP"+ansiReset) {
		t.Errorf("expected green UP status, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), ansiRed+"DOWN"+ansiReset) {
		t.Errorf("expected red DOWN status, got: %s", stdout.String())
	}

	stdout.Reset()
	if code := runApp([]string{"ping", "--no-color", "--config-dir", dir}, app); code != 1 {
		t.Fatalf("no-color ping code = %d, want 1; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("expected no ANSI escapes with --no-color, got: %s", stdout.String())
	}
}

func TestCmdPing_RejectsExtraArguments(t *testing.T) {
	app := makeFakeApp(true)
	var stderr strings.Builder
	app.Stderr = &stderr

	if code := cmdPing(&globalFlags{}, []string{"sandbox", "mailvm"}, app); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "at most one VM name") {
		t.Errorf("error = %q", stderr.String())
	}
}
