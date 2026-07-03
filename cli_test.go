package main

import (
	"os"
	"testing"
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
