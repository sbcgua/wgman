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
