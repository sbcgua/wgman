package main

import (
	"strings"
	"testing"
)

func TestCmdCheckSmoke(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeValidTestData(h, dir)

	var stdout, stderr strings.Builder
	app := &App{
		Sys:    buildCleanFakeSystem(),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	if code := cmdCheck(&globalFlags{configDir: dir}, nil, app); code != 0 {
		t.Fatalf("cmdCheck() exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.String() != "check: OK\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "check: OK\n")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestCmdCheck_ColorizesOKWhenTerminal(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeValidTestData(h, dir)

	sys := buildCleanFakeSystem()
	sys.isTerminal = true
	var stdout, stderr strings.Builder
	app := &App{
		Sys:    sys,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	if code := cmdCheck(&globalFlags{configDir: dir}, nil, app); code != 0 {
		t.Fatalf("cmdCheck() exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.String() != "check: "+ansiGreen+"OK"+ansiReset+"\n" {
		t.Errorf("stdout = %q, want green OK", stdout.String())
	}
}

func TestCmdCheck_ColorizesFailedWhenTerminal(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeValidTestData(h, dir)

	sys := buildCleanFakeSystem()
	sys.isTerminal = true
	sys.wgDumpResult += "UNKNOWN_PUB=\t(none)\t(none)\t10.8.0.99/32\t0\t0\t0\toff\n"
	var stdout, stderr strings.Builder
	app := &App{
		Sys:    sys,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	if code := cmdCheck(&globalFlags{configDir: dir}, nil, app); code == 0 {
		t.Fatal("cmdCheck() exit code = 0, want failure")
	}
	if !strings.Contains(stderr.String(), "check: "+ansiRed+"FAILED"+ansiReset+"\n") {
		t.Errorf("stderr = %q, want red FAILED", stderr.String())
	}
}

func TestCmdCheck_NoColorSuppressesStatusColor(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	writeValidTestData(h, dir)

	sys := buildCleanFakeSystem()
	sys.isTerminal = true
	var stdout, stderr strings.Builder
	app := &App{
		Sys:    sys,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	if code := cmdCheck(&globalFlags{configDir: dir, noColor: true}, nil, app); code != 0 {
		t.Fatalf("cmdCheck() exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.String() != "check: OK\n" {
		t.Errorf("stdout = %q, want plain OK", stdout.String())
	}
}
