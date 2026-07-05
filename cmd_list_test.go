package main

import (
	"io"
	"strings"
	"testing"
)

func TestRunList_NoFilter(t *testing.T) {
	db := makeTestDB()
	var buf strings.Builder
	code := runList(db, &CheckResult{}, "", &buf, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	for _, want := range []string{"Users:", "alice", "10.8.0.10", "admin", "bob", "VMs:", "sandbox", "mailvm"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in list output, got:\n%s", want, out)
		}
	}
}

func TestRunList_UsersSortedAlphabetically(t *testing.T) {
	db := makeTestDB()
	var buf strings.Builder
	runList(db, &CheckResult{}, "", &buf, io.Discard)
	out := buf.String()
	adminPos := strings.Index(out, "admin")
	alicePos := strings.Index(out, "alice")
	bobPos := strings.Index(out, "bob")
	if adminPos > alicePos || alicePos > bobPos {
		t.Errorf("expected users in alphabetical order, got:\n%s", out)
	}
}

func TestRunList_WithFilter(t *testing.T) {
	db := makeTestDB()
	var buf strings.Builder
	code := runList(db, &CheckResult{}, "alice", &buf, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "sandbox") {
		t.Errorf("expected sandbox in alice's access, got:\n%s", out)
	}
	if strings.Contains(out, "mailvm") {
		t.Errorf("mailvm should not appear for alice, got:\n%s", out)
	}
	if !strings.Contains(out, "alice") {
		t.Errorf("expected alice's name in output, got:\n%s", out)
	}
}

func TestRunList_FilterAdminStar(t *testing.T) {
	db := makeTestDB()
	var buf strings.Builder
	code := runList(db, &CheckResult{}, "admin", &buf, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "*") {
		t.Errorf("expected * in admin's access, got:\n%s", out)
	}
}

func TestRunList_FilterNoAccess(t *testing.T) {
	db := makeTestDB()
	db.Users["newguy"] = UserEntry{IP: "10.8.0.20", Pub: "NEWGUY_PUB="}
	var buf strings.Builder
	code := runList(db, &CheckResult{}, "newguy", &buf, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected (none) for user with no access, got:\n%s", out)
	}
}

func TestRunList_FilterUnknownUser(t *testing.T) {
	db := makeTestDB()
	var stdout, stderr strings.Builder
	code := runList(db, &CheckResult{}, "nobody", &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero code for unknown user filter")
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no stdout for missing user, got: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "not found") || !strings.Contains(stderr.String(), "list: FAILED") {
		t.Errorf("expected not found failure in stderr, got: %s", stderr.String())
	}
}

func TestRunList_RefusesOnHardErrors(t *testing.T) {
	db := makeTestDB()
	result := &CheckResult{HardErrors: []string{"some hard error"}}
	var buf strings.Builder
	code := runList(db, result, "", &buf, io.Discard)
	if code == 0 {
		t.Error("expected non-zero code on check failure")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output to w on check failure, got: %s", buf.String())
	}
}

func TestRunList_RefusesOnDrift(t *testing.T) {
	db := makeTestDB()
	result := &CheckResult{Drift: []string{"ipset drift detected"}}
	var buf strings.Builder
	code := runList(db, result, "", &buf, io.Discard)
	if code == 0 {
		t.Error("expected non-zero code when drift present")
	}
}
