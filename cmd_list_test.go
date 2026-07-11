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
	for _, want := range []string{"admin", "(*)", "alice", "(sandbox)", "bob", "(mailvm,sandbox)"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected access summary %q in list output, got:\n%s", want, out)
		}
	}
}

func TestRunList_NoFilterUserWithNoAccess(t *testing.T) {
	db := makeTestDB()
	db.Users["newguy"] = UserEntry{IP: "10.8.0.20", Pub: "NEWGUY_PUB="}
	var buf strings.Builder
	code := runList(db, &CheckResult{}, "", &buf, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "newguy") || !strings.Contains(out, "(none)") {
		t.Errorf("expected no-access summary for newguy, got:\n%s", out)
	}
}

func TestRunList_ShowsResources(t *testing.T) {
	db := makeResourceDB()
	var buf strings.Builder
	code := runList(db, &CheckResult{}, "", &buf, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	for _, want := range []string{"Resources:", "dns@mailvm", "mailvm", "udp:53", "ssh@sandbox", "sandbox", "tcp:22"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in list output, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "(dns@mailvm,ssh@sandbox)") {
		t.Errorf("expected mixed resource access summary, got:\n%s", out)
	}
}

func TestRunList_ColorizesAccessSummaryMarkers(t *testing.T) {
	db := makeTestDB()
	db.Users["newguy"] = UserEntry{IP: "10.8.0.20", Pub: "NEWGUY_PUB="}
	var buf strings.Builder
	code := runListWithColor(db, &CheckResult{}, "", &buf, io.Discard, true)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "("+ansiRed+"*"+ansiReset+")") {
		t.Errorf("expected red admin star in access summary, got:\n%s", out)
	}
	if !strings.Contains(out, "("+ansiGrey+"none"+ansiReset+")") {
		t.Errorf("expected grey none in access summary, got:\n%s", out)
	}
	if !strings.Contains(stripANSI(out), "(*)") || !strings.Contains(stripANSI(out), "(none)") {
		t.Errorf("plain access summaries changed after stripping ANSI, got:\n%s", stripANSI(out))
	}
}

func TestRunList_InactiveUserNameSuffixNoColor(t *testing.T) {
	db := makeTestDB()
	bob := db.Users["bob"]
	bob.Inactive = true
	db.Users["bob"] = bob

	var buf strings.Builder
	code := runListWithColor(db, &CheckResult{}, "", &buf, io.Discard, false)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "bob~") {
		t.Errorf("expected inactive user suffix, got:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI escapes in no-color output, got:\n%s", out)
	}
}

func TestRunList_InactiveUserNameGreyWithColor(t *testing.T) {
	db := makeTestDB()
	bob := db.Users["bob"]
	bob.Inactive = true
	db.Users["bob"] = bob

	var buf strings.Builder
	code := runListWithColor(db, &CheckResult{}, "", &buf, io.Discard, true)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, ansiGrey+"bob~"+ansiReset) {
		t.Errorf("expected grey inactive user name, got:\n%s", out)
	}
	if !strings.Contains(stripANSI(out), "bob~") {
		t.Errorf("expected inactive suffix after stripping ANSI, got:\n%s", stripANSI(out))
	}
}

func TestRunList_FilterColorizesAccessMarkers(t *testing.T) {
	db := makeTestDB()
	var admin strings.Builder
	code := runListWithColor(db, &CheckResult{}, "admin", &admin, io.Discard, true)
	if code != 0 {
		t.Fatalf("admin list code = %d, want 0", code)
	}
	if !strings.Contains(admin.String(), ansiRed+"*"+ansiReset) {
		t.Errorf("expected red admin star, got:\n%s", admin.String())
	}

	db.Users["newguy"] = UserEntry{IP: "10.8.0.20", Pub: "NEWGUY_PUB="}
	var none strings.Builder
	code = runListWithColor(db, &CheckResult{}, "newguy", &none, io.Discard, true)
	if code != 0 {
		t.Fatalf("newguy list code = %d, want 0", code)
	}
	if !strings.Contains(none.String(), ansiGrey+"none"+ansiReset) {
		t.Errorf("expected grey none, got:\n%s", none.String())
	}
}

func TestRunList_FilterInactiveUserNameGreyWithColor(t *testing.T) {
	db := makeTestDB()
	bob := db.Users["bob"]
	bob.Inactive = true
	db.Users["bob"] = bob

	var buf strings.Builder
	code := runListWithColor(db, &CheckResult{}, "bob", &buf, io.Discard, true)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, ansiGrey+"bob~"+ansiReset+":") {
		t.Errorf("expected grey inactive filtered user name, got:\n%s", out)
	}
	if !strings.Contains(stripANSI(out), "bob~:") {
		t.Errorf("expected inactive suffix after stripping ANSI, got:\n%s", stripANSI(out))
	}
}

func TestRunList_ColorizesResourcePortPrefixes(t *testing.T) {
	db := makeResourceDB()
	db.Resources["dns@mailvm"] = ResourceEntry{
		VM:    "mailvm",
		Ports: ResourcePorts{{Protocol: "udp", Port: 53}, {Protocol: "tcp", Port: 53}},
	}
	var buf strings.Builder
	code := runListWithColor(db, &CheckResult{}, "", &buf, io.Discard, true)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, ansiPink+"tcp:"+ansiReset+"53") {
		t.Errorf("expected pink tcp prefix, got:\n%s", out)
	}
	if !strings.Contains(out, ansiCyan+"udp:"+ansiReset+"53") {
		t.Errorf("expected cyan udp prefix, got:\n%s", out)
	}
	if !strings.Contains(stripANSI(out), "tcp:53,udp:53") {
		t.Errorf("expected plain sorted ports after stripping ANSI, got:\n%s", stripANSI(out))
	}
}

func TestRunList_ColorDoesNotChangeVisibleLayout(t *testing.T) {
	db := makeResourceDB()
	bob := db.Users["bob"]
	bob.Inactive = true
	db.Users["bob"] = bob
	var plain strings.Builder
	var colored strings.Builder

	runListWithColor(db, &CheckResult{}, "", &plain, io.Discard, false)
	runListWithColor(db, &CheckResult{}, "", &colored, io.Discard, true)

	if got, want := stripANSI(colored.String()), plain.String(); got != want {
		t.Errorf("colored output changed visible layout after stripping ANSI\ngot:\n%s\nwant:\n%s", got, want)
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
