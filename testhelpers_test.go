package main

import (
	"os"
	"path/filepath"
	"testing"
)

// testHelper provides test utilities for table-driven tests.
type testHelper struct {
	t *testing.T
}

func newHelper(t *testing.T) *testHelper {
	t.Helper()
	return &testHelper{t: t}
}

// makeTempDir creates a temporary directory for a test and returns its path.
// The directory is automatically removed when the test finishes.
func (h *testHelper) makeTempDir() string {
	h.t.Helper()
	dir, err := os.MkdirTemp("", "wgman-test-*")
	if err != nil {
		h.t.Fatalf("makeTempDir: %v", err)
	}
	h.t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// writeFile writes content to path inside dir.
func (h *testHelper) writeFile(dir, name, content string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		h.t.Fatalf("writeFile %s: %v", name, err)
	}
}

// assertNoError fails the test if err is non-nil.
func (h *testHelper) assertNoError(err error) {
	h.t.Helper()
	if err != nil {
		h.t.Fatalf("unexpected error: %v", err)
	}
}

// assertError fails the test if err is nil, or if want is non-empty and the
// error message does not contain want.
func (h *testHelper) assertError(err error, want string) {
	h.t.Helper()
	if err == nil {
		h.t.Fatal("expected an error but got nil")
	}
	if want != "" && !containsStr(err.Error(), want) {
		h.t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

// fakeSystem is a minimal fake SystemAdapter for tests.
// Fields can be populated per test case as needed.
type fakeSystem struct {
	isRoot         bool
	subnetResult   string
	subnetErr      error
	wgDumpResult   string
	wgDumpErr      error
	ipsetResults   map[string]string
	ipsetErrs      map[string]error
	ipsetCreateErr error
	ipsetAddErr    error
	ipsetDelErr    error
	wgSetErr       error
	wgDelErr       error
	appliedOps     []string // records IPSetCreate/Add/Del and WGSet/Del calls
	genKeyResult   string
	genKeyErr      error
	pubKeyResult   string
	pubKeyErr      error
}

func newFakeSystem() *fakeSystem {
	return &fakeSystem{
		isRoot:       true,
		ipsetResults: make(map[string]string),
		ipsetErrs:    make(map[string]error),
	}
}

func (f *fakeSystem) IsRoot() bool { return f.isRoot }

func (f *fakeSystem) InterfaceSubnet(_ string) (string, error) {
	return f.subnetResult, f.subnetErr
}

func (f *fakeSystem) WGDump(_ string) (string, error) {
	return f.wgDumpResult, f.wgDumpErr
}

func (f *fakeSystem) IPSetList(setname string) (string, error) {
	if err, ok := f.ipsetErrs[setname]; ok {
		return "", err
	}
	return f.ipsetResults[setname], nil
}

func (f *fakeSystem) IPSetCreate(setname, setType string, withComment bool) error {
	if f.ipsetCreateErr != nil {
		return f.ipsetCreateErr
	}
	commentFlag := "nocomment"
	if withComment {
		commentFlag = "comment"
	}
	f.appliedOps = append(f.appliedOps, "create:"+setname+":"+setType+":"+commentFlag)
	return nil
}

func (f *fakeSystem) IPSetAdd(setname, entry, comment string) error {
	if f.ipsetAddErr != nil {
		return f.ipsetAddErr
	}
	f.appliedOps = append(f.appliedOps, "add:"+setname+":"+entry+":"+comment)
	return nil
}

func (f *fakeSystem) IPSetDel(setname, entry string) error {
	if f.ipsetDelErr != nil {
		return f.ipsetDelErr
	}
	f.appliedOps = append(f.appliedOps, "del:"+setname+":"+entry)
	return nil
}

func (f *fakeSystem) WGSetPeer(iface, pubkey, allowedIP string) error {
	if f.wgSetErr != nil {
		return f.wgSetErr
	}
	f.appliedOps = append(f.appliedOps, "wgset:"+iface+":"+pubkey+":"+allowedIP)
	return nil
}

func (f *fakeSystem) WGDelPeer(iface, pubkey string) error {
	if f.wgDelErr != nil {
		return f.wgDelErr
	}
	f.appliedOps = append(f.appliedOps, "wgdel:"+iface+":"+pubkey)
	return nil
}

func (f *fakeSystem) WGGenKey() (string, error) {
	if f.genKeyErr != nil {
		return "", f.genKeyErr
	}
	if f.genKeyResult != "" {
		return f.genKeyResult, nil
	}
	return "FAKE_PRIVATE_KEY", nil
}

func (f *fakeSystem) WGPubKey(_ string) (string, error) {
	if f.pubKeyErr != nil {
		return "", f.pubKeyErr
	}
	if f.pubKeyResult != "" {
		return f.pubKeyResult, nil
	}
	return "FAKE_PUBLIC_KEY", nil
}

// containsStr reports whether s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		indexStr(s, substr) >= 0)
}

func indexStr(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}
