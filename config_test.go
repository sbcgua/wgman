package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ---- LoadConfig tests ----

func TestLoadConfig_Valid(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	h.writeFile(dir, "config.yaml", `
interface: wg0
sets:
  all: wg_allow_all
  matrix: wg_allow_matrix
`)
	cfg, err := LoadConfig(dir)
	h.assertNoError(err)
	if cfg.Interface != "wg0" {
		t.Errorf("interface = %q, want wg0", cfg.Interface)
	}
	if cfg.Sets.All != "wg_allow_all" {
		t.Errorf("sets.all = %q, want wg_allow_all", cfg.Sets.All)
	}
	if cfg.Sets.Matrix != "wg_allow_matrix" {
		t.Errorf("sets.matrix = %q, want wg_allow_matrix", cfg.Sets.Matrix)
	}
}

func TestLoadConfig_FromTestdata(t *testing.T) {
	cfg, err := LoadConfig("testdata/valid-offline")
	if err != nil {
		t.Fatalf("LoadConfig(testdata/valid-offline): %v", err)
	}
	if cfg.Interface != "wg0" {
		t.Errorf("interface = %q, want wg0", cfg.Interface)
	}
}

var configRequiredFieldTests = []struct {
	name    string
	yaml    string
	wantErr string
}{
	{
		name:    "missing interface",
		yaml:    "sets:\n  all: a\n  matrix: b\n",
		wantErr: "interface is required",
	},
	{
		name:    "missing sets.all",
		yaml:    "interface: wg0\nsets:\n  matrix: b\n",
		wantErr: "sets.all is required",
	},
	{
		name:    "missing sets.matrix",
		yaml:    "interface: wg0\nsets:\n  all: a\n",
		wantErr: "sets.matrix is required",
	},
	{
		name:    "unknown field",
		yaml:    "interface: wg0\nsets:\n  all: a\n  matrix: b\nextra: bad\n",
		wantErr: "not found in type",
	},
}

func TestLoadConfig_RequiredFields(t *testing.T) {
	for _, tc := range configRequiredFieldTests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHelper(t)
			dir := h.makeTempDir()
			h.writeFile(dir, "config.yaml", tc.yaml)
			_, err := LoadConfig(dir)
			h.assertError(err, tc.wantErr)
		})
	}
}

// ---- LoadDB tests ----

const validDBYAML = `
users:
  admin:
    ip: 10.8.0.5
    pub: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
  alice:
    ip: 10.8.0.10
    pub: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=
vms:
  sandbox: 192.168.122.100
access:
  admin:
    - "*"
  alice:
    - sandbox
`

func TestLoadDB_Valid(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	h.writeFile(dir, "db.yaml", validDBYAML)
	db, err := LoadDB(dir)
	h.assertNoError(err)
	if len(db.Users) != 2 {
		t.Errorf("users count = %d, want 2", len(db.Users))
	}
	if len(db.VMs) != 1 {
		t.Errorf("vms count = %d, want 1", len(db.VMs))
	}
}

func TestLoadDB_UserMetadata(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	h.writeFile(dir, "db.yaml", `
users:
  alice:
    ip: 10.8.0.10
    pub: ALICE_PUB=
    comment: offboarding pending
    inactive: true
  bob:
    ip: 10.8.0.11
    pub: BOB_PUB=
vms:
  sandbox: 192.168.122.100
`)
	db, err := LoadDB(dir)
	h.assertNoError(err)
	if db.Users["alice"].Comment != "offboarding pending" {
		t.Errorf("alice comment = %q, want offboarding pending", db.Users["alice"].Comment)
	}
	if !db.Users["alice"].Inactive {
		t.Error("alice inactive = false, want true")
	}
	if db.Users["bob"].Comment != "" {
		t.Errorf("bob comment = %q, want empty", db.Users["bob"].Comment)
	}
	if db.Users["bob"].Inactive {
		t.Error("bob inactive = true, want false")
	}
}

func TestLoadDB_FromTestdata(t *testing.T) {
	_, err := LoadDB("testdata/valid-offline")
	if err != nil {
		t.Fatalf("LoadDB(testdata/valid-offline): %v", err)
	}
}

var dbValidationTests = []struct {
	name    string
	yaml    string
	wantErr string
}{
	{
		name:    "missing users section",
		yaml:    "vms:\n  sandbox: 192.168.122.100\n",
		wantErr: "users section is required",
	},
	{
		name:    "missing vms section",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: ABC=\n",
		wantErr: "vms section is required",
	},
	{
		name:    "invalid user name",
		yaml:    "users:\n  \"alice smith\":\n    ip: 10.8.0.10\n    pub: ABC=\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "invalid user name",
	},
	{
		name:    "user missing ip",
		yaml:    "users:\n  alice:\n    pub: ABC=\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "ip is required",
	},
	{
		name:    "user missing pub",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "pub is required",
	},
	{
		name:    "duplicate ips",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\n  bob:\n    ip: 10.8.0.10\n    pub: BBBB=\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "duplicate ip",
	},
	{
		name:    "duplicate pub keys",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\n  bob:\n    ip: 10.8.0.11\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "duplicate pub key",
	},
	{
		name:    "case-conflicting user names",
		yaml:    "users:\n  Alice:\n    ip: 10.8.0.10\n    pub: AAAA=\n  alice:\n    ip: 10.8.0.11\n    pub: BBBB=\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "conflicts with",
	},
	{
		name:    "invalid vm name",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  \"sand box\": 192.168.122.100\n",
		wantErr: "invalid vm name",
	},
	{
		name:    "access references unknown vm",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\naccess:\n  alice:\n    - unknownvm\n",
		wantErr: "unknown vm",
	},
	{
		name:    "access references unknown user",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\naccess:\n  nobody:\n    - sandbox\n",
		wantErr: "unknown user",
	},
	{
		name:    "duplicate access entry",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\naccess:\n  alice:\n    - sandbox\n    - sandbox\n",
		wantErr: "duplicate entry",
	},
	{
		name:    "unknown field in db",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\nextrafield: bad\n",
		wantErr: "not found in type",
	},
	{
		name:    "unknown field in user",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\n    extra: bad\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "not found in type",
	},
}

func TestLoadDB_Validation(t *testing.T) {
	for _, tc := range dbValidationTests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHelper(t)
			dir := h.makeTempDir()
			h.writeFile(dir, "db.yaml", tc.yaml)
			_, err := LoadDB(dir)
			h.assertError(err, tc.wantErr)
		})
	}
}

// ---- validateDB tests ----

func mustLoadTestdata(t *testing.T) (*Config, *DB) {
	t.Helper()
	cfg, err := LoadConfig("testdata/valid-offline")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	db, err := LoadDB("testdata/valid-offline")
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}
	return cfg, db
}

func TestValidateDB_Valid(t *testing.T) {
	_, db := mustLoadTestdata(t)
	errs := validateDB(db)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateDB_InvalidUserIP(t *testing.T) {
	_, db := mustLoadTestdata(t)
	db.Users["alice"] = UserEntry{IP: "not-an-ip", Pub: db.Users["alice"].Pub}
	errs := validateDB(db)
	if !anyContains(errs, "invalid ip") {
		t.Errorf("expected error about invalid ip, got: %v", errs)
	}
}

func TestValidateDB_IPv6UserIP(t *testing.T) {
	_, db := mustLoadTestdata(t)
	db.Users["alice"] = UserEntry{IP: "::1", Pub: db.Users["alice"].Pub}
	errs := validateDB(db)
	if !anyContains(errs, "invalid ip") {
		t.Errorf("expected error for IPv6 user ip, got: %v", errs)
	}
}

func TestValidateDB_IPv6VMIP(t *testing.T) {
	_, db := mustLoadTestdata(t)
	db.VMs["sandbox"] = "2001:db8::1"
	errs := validateDB(db)
	if !anyContains(errs, "invalid ip") {
		t.Errorf("expected error for IPv6 vm ip, got: %v", errs)
	}
}

func TestValidateDB_InvalidVMIP(t *testing.T) {
	_, db := mustLoadTestdata(t)
	db.VMs["sandbox"] = "not-an-ip"
	errs := validateDB(db)
	if !anyContains(errs, "invalid ip") {
		t.Errorf("expected error about invalid vm ip, got: %v", errs)
	}
}

func TestValidateDB_StarMixedWithVMs(t *testing.T) {
	_, db := mustLoadTestdata(t)
	db.Access["alice"] = []string{"*", "sandbox"}
	errs := validateDB(db)
	if !anyContains(errs, `"*" must be the sole entry`) {
		t.Errorf("expected error about mixed star access, got: %v", errs)
	}
}

func TestSaveDBAtomic_DeterministicOutput(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	db := &DB{
		Users: map[string]UserEntry{
			"bob":   {IP: "10.8.0.15", Pub: "BOB_PUB=", Comment: "temporary contractor", Inactive: true},
			"alice": {IP: "10.8.0.10", Pub: "ALICE_PUB="},
		},
		VMs: map[string]string{
			"sandbox": "192.168.122.100",
			"mailvm":  "192.168.122.101",
		},
		Access: map[string][]string{
			"bob":   {"sandbox", "mailvm"},
			"alice": {"sandbox"},
		},
	}

	if err := SaveDBAtomic(dir, db); err != nil {
		t.Fatalf("SaveDBAtomic: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db.yaml: %v", err)
	}
	want := "users:\n  alice:\n    ip: 10.8.0.10\n    pub: ALICE_PUB=\n  bob:\n    ip: 10.8.0.15\n    pub: BOB_PUB=\n    comment: temporary contractor\n    inactive: true\n\nvms:\n  mailvm: 192.168.122.101\n  sandbox: 192.168.122.100\n\naccess:\n  alice:\n    - sandbox\n  bob:\n    - sandbox\n    - mailvm\n"
	if string(data) != want {
		t.Errorf("db.yaml =\n%s\nwant:\n%s", string(data), want)
	}
	loaded, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("saved db should load: %v", err)
	}
	if loaded.Users["alice"].Comment != "" || loaded.Users["alice"].Inactive {
		t.Errorf("alice metadata = %+v, want empty comment and active", loaded.Users["alice"])
	}
	if loaded.Users["bob"].Comment != "temporary contractor" || !loaded.Users["bob"].Inactive {
		t.Errorf("bob metadata = %+v, want comment and inactive true", loaded.Users["bob"])
	}
}

func TestCloneDBIsIndependent(t *testing.T) {
	original := &DB{
		Users:  map[string]UserEntry{"alice": {IP: "10.8.0.10", Pub: "ALICE"}},
		VMs:    map[string]string{"sandbox": "192.168.122.100"},
		Access: map[string][]string{"alice": {"sandbox"}},
	}

	cloned := cloneDB(original)
	cloned.Users["alice"] = UserEntry{IP: "10.8.0.20", Pub: "CHANGED"}
	cloned.VMs["sandbox"] = "192.168.122.200"
	cloned.Access["alice"][0] = "changed"

	if original.Users["alice"].IP != "10.8.0.10" {
		t.Error("cloneDB() aliased the users map")
	}
	if original.VMs["sandbox"] != "192.168.122.100" {
		t.Error("cloneDB() aliased the VMs map")
	}
	if original.Access["alice"][0] != "sandbox" {
		t.Error("cloneDB() aliased an access slice")
	}
}

func TestNormalizeDBAccess(t *testing.T) {
	db := &DB{Access: map[string][]string{
		"alice": {"sandbox", "mail", "sandbox"},
		"bob":   {},
	}}

	normalizeDBAccess(db)

	want := map[string][]string{"alice": {"mail", "sandbox"}}
	if !reflect.DeepEqual(db.Access, want) {
		t.Errorf("normalized access = %#v, want %#v", db.Access, want)
	}
}

func TestNormalizeAccessSet(t *testing.T) {
	if got := normalizeAccessSet(nil); got != nil {
		t.Errorf("normalizeAccessSet(nil) = %#v, want nil", got)
	}
	got := normalizeAccessSet(map[string]bool{"sandbox": true, "mail": true})
	want := []string{"mail", "sandbox"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizeAccessSet() = %#v, want %#v", got, want)
	}
}

func anyContains(ss []string, substr string) bool {
	for _, s := range ss {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
