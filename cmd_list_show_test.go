package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

// ---- helpers ----

// makeCleanCheckResult builds an OK CheckResult with a WGDump matching makeTestDB().
func makeCleanCheckResult() *CheckResult {
	return &CheckResult{
		WGDump: &WGDumpResult{
			ServerPubKey: "SERVER_PUB=",
			Peers: []WGPeer{
				{PublicKey: "ADMIN_PUB=", Endpoint: "(none)", AllowedIP: "10.8.0.5", RxBytes: 0, TxBytes: 0, LatestHandshake: 0},
				{PublicKey: "ALICE_PUB=", Endpoint: "192.168.1.100:50001", AllowedIP: "10.8.0.10", RxBytes: 102400, TxBytes: 204800, LatestHandshake: 1748000000},
				{PublicKey: "BOB_PUB=", Endpoint: "(none)", AllowedIP: "10.8.0.15", RxBytes: 0, TxBytes: 0, LatestHandshake: 0},
			},
		},
	}
}

// ---- runList tests ----

func TestRunList_NoFilter(t *testing.T) {
	db := makeTestDB()
	var buf strings.Builder
	code := runList(db, makeCleanCheckResult(), "", &buf, io.Discard)
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
	runList(db, makeCleanCheckResult(), "", &buf, io.Discard)
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
	code := runList(db, makeCleanCheckResult(), "alice", &buf, io.Discard)
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
	code := runList(db, makeCleanCheckResult(), "admin", &buf, io.Discard)
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
	// User with no access entries.
	db.Users["newguy"] = UserEntry{IP: "10.8.0.20", Pub: "NEWGUY_PUB="}
	var buf strings.Builder
	code := runList(db, makeCleanCheckResult(), "newguy", &buf, io.Discard)
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
	var buf strings.Builder
	code := runList(db, makeCleanCheckResult(), "nobody", &buf, io.Discard)
	if code == 0 {
		t.Error("expected non-zero code for unknown user filter")
	}
	if !strings.Contains(buf.String(), "not found") {
		t.Errorf("expected 'not found' in output, got: %s", buf.String())
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

// ---- runShow tests ----

func TestRunShow_Basic(t *testing.T) {
	db := makeTestDB()
	result := makeCleanCheckResult()
	// alice's handshake is 1748000000; use now = 1748001000 → age = 1000s = 16m40s
	now := time.Unix(1748001000, 0)

	var buf strings.Builder
	code := runShow(db, result, now, &buf, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()

	for _, want := range []string{"NAME", "alice", "admin", "bob", "ENDPOINT", "LAST HANDSHAKE"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in show output, got:\n%s", want, out)
		}
	}
}

func TestRunShow_EndpointWithoutPort(t *testing.T) {
	db := makeTestDB()
	result := makeCleanCheckResult()
	var buf strings.Builder
	runShow(db, result, time.Now(), &buf, io.Discard)
	out := buf.String()

	// alice's endpoint is "192.168.1.100:50001"; port should be stripped
	if strings.Contains(out, "50001") {
		t.Errorf("port should not appear in show output, got:\n%s", out)
	}
	if !strings.Contains(out, "192.168.1.100") {
		t.Errorf("expected host part of endpoint, got:\n%s", out)
	}
}

func TestRunShow_ByteFormatting(t *testing.T) {
	db := makeTestDB()
	result := makeCleanCheckResult()
	var buf strings.Builder
	runShow(db, result, time.Now(), &buf, io.Discard)
	out := buf.String()

	// alice: RxBytes=102400 (100*1024) → "100.00Kb", TxBytes=204800 (200*1024) → "200.00Kb"
	if !strings.Contains(out, "100.00Kb") {
		t.Errorf("expected formatted rx bytes, got:\n%s", out)
	}
	if !strings.Contains(out, "200.00Kb") {
		t.Errorf("expected formatted tx bytes, got:\n%s", out)
	}
}

func TestRunShow_HandshakeFormatting(t *testing.T) {
	db := makeTestDB()
	result := makeCleanCheckResult()
	// alice LatestHandshake = 1748000000; now = 1748001000 → 1000s = 16m40s
	now := time.Unix(1748001000, 0)
	var buf strings.Builder
	runShow(db, result, now, &buf, io.Discard)
	out := buf.String()

	if !strings.Contains(out, "16m40s") {
		t.Errorf("expected '16m40s' handshake age, got:\n%s", out)
	}
	if !strings.Contains(out, "never") {
		t.Errorf("expected 'never' for zero handshake, got:\n%s", out)
	}
}

func TestRunShow_SortedByName(t *testing.T) {
	db := makeTestDB()
	result := makeCleanCheckResult()
	var buf strings.Builder
	runShow(db, result, time.Now(), &buf, io.Discard)
	out := buf.String()

	adminPos := strings.Index(out, "admin")
	alicePos := strings.Index(out, "alice")
	bobPos := strings.Index(out, "bob")
	if adminPos > alicePos || alicePos > bobPos {
		t.Errorf("expected users in alphabetical order, got:\n%s", out)
	}
}

func TestRunShow_RefusesOnHardErrors(t *testing.T) {
	db := makeTestDB()
	result := &CheckResult{HardErrors: []string{"something wrong"}}
	var buf strings.Builder
	code := runShow(db, result, time.Now(), &buf, io.Discard)
	if code == 0 {
		t.Error("expected non-zero code on check failure")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output to w on check failure, got: %s", buf.String())
	}
}

func TestRunShow_RefusesOnDrift(t *testing.T) {
	db := makeTestDB()
	result := &CheckResult{
		Drift:  []string{"ipset drift"},
		WGDump: makeCleanCheckResult().WGDump,
	}
	var buf strings.Builder
	code := runShow(db, result, time.Now(), &buf, io.Discard)
	if code == 0 {
		t.Error("expected non-zero code when drift present")
	}
}
