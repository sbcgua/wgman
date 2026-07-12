package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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

func TestLoadDB_UserGroups(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	h.writeFile(dir, "db.yaml", `
users:
  alice:
    ip: 10.8.0.10
    pub: ALICE_PUB=
  bob:
    ip: 10.8.0.11
    pub: BOB_PUB=
user-groups:
  devs:
    - alice
    - bob
  empty: []
vms:
  sandbox: 192.168.122.100
access:
  devs:
    - sandbox
`)
	db, err := LoadDB(dir)
	h.assertNoError(err)
	if got := strings.Join(db.UserGroups["devs"], ","); got != "alice,bob" {
		t.Errorf("devs members = %q, want alice,bob", got)
	}
	if members, ok := db.UserGroups["empty"]; !ok || len(members) != 0 {
		t.Errorf("empty group = %#v, want empty slice", members)
	}
}

func TestLoadDB_Resources(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	h.writeFile(dir, "db.yaml", `
users:
  alice:
    ip: 10.8.0.10
    pub: ALICE_PUB=
vms:
  sandbox: 192.168.122.100
resources:
  ssh@sandbox:
    vm: sandbox
    ports: 22
    comment: shell access
  dns@sandbox:
    vm: sandbox
    ports:
      - udp:53
      - tcp:53
access:
  alice:
    - ssh@sandbox
    - dns@sandbox
`)
	db, err := LoadDB(dir)
	h.assertNoError(err)
	ssh := db.Resources["ssh@sandbox"]
	if ssh.VM != "sandbox" || ssh.Comment != "shell access" {
		t.Errorf("ssh resource = %+v, want vm/comment", ssh)
	}
	if len(ssh.Ports) != 1 || ssh.Ports[0] != (ResourcePort{Protocol: "tcp", Port: 22}) {
		t.Errorf("ssh ports = %+v, want tcp:22", ssh.Ports)
	}
	dns := db.Resources["dns@sandbox"]
	wantDNS := ResourcePorts{{Protocol: "udp", Port: 53}, {Protocol: "tcp", Port: 53}}
	if !reflect.DeepEqual(dns.Ports, wantDNS) {
		t.Errorf("dns ports = %+v, want %+v", dns.Ports, wantDNS)
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
		name:    "access references unknown target",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\naccess:\n  alice:\n    - unknownvm\n",
		wantErr: "unknown access target",
	},
	{
		name:    "resource references unknown vm",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\nresources:\n  ssh@sandbox:\n    vm: missing\n    ports: 22\n",
		wantErr: "unknown vm",
	},
	{
		name:    "resource conflicts with vm",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\nresources:\n  sandbox:\n    vm: sandbox\n    ports: 22\n",
		wantErr: "conflicts",
	},
	{
		name:    "resource duplicate normalized port",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\nresources:\n  ssh@sandbox:\n    vm: sandbox\n    ports: [22, tcp:22]\n",
		wantErr: "duplicate port",
	},
	{
		name:    "vm name cannot contain at",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  ssh@sandbox: 192.168.122.100\n",
		wantErr: "invalid vm name",
	},
	{
		name:    "access references unknown user",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\naccess:\n  nobody:\n    - sandbox\n",
		wantErr: "unknown user",
	},
	{
		name:    "invalid user group name",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nuser-groups:\n  \"dev team\": [alice]\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "invalid user group name",
	},
	{
		name:    "user group unknown member",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nuser-groups:\n  devs: [bob]\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "unknown user",
	},
	{
		name:    "user group conflicts with user",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nuser-groups:\n  Alice: []\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "conflicts",
	},
	{
		name:    "user group duplicate member",
		yaml:    "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nuser-groups:\n  devs: [alice, alice]\nvms:\n  sandbox: 192.168.122.100\n",
		wantErr: "duplicate user",
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
	want := "users:\n  alice:\n    ip: 10.8.0.10\n    pub: ALICE_PUB=\n  bob:\n    ip: 10.8.0.15\n    pub: BOB_PUB=\n    comment: temporary contractor\n    inactive: true\n\nuser-groups: {}\n\nvms:\n  mailvm: 192.168.122.101\n  sandbox: 192.168.122.100\n\nresources: {}\n\naccess:\n  alice:\n    - sandbox\n  bob:\n    - sandbox\n    - mailvm\n"
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

func TestSaveDBAtomic_WritesUserGroups(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	db := &DB{
		Users: map[string]UserEntry{
			"alice": {IP: "10.8.0.10", Pub: "ALICE_PUB="},
			"bob":   {IP: "10.8.0.11", Pub: "BOB_PUB="},
		},
		UserGroups: map[string][]string{
			"empty": {},
			"devs":  {"bob", "alice"},
		},
		VMs:       map[string]string{"sandbox": "192.168.122.100"},
		Resources: map[string]ResourceEntry{},
		Access:    map[string][]string{"devs": {"sandbox"}},
	}

	if err := SaveDBAtomic(dir, db); err != nil {
		t.Fatalf("SaveDBAtomic: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "db.yaml"))
	if err != nil {
		t.Fatalf("read db.yaml: %v", err)
	}
	out := string(data)
	for _, want := range []string{"user-groups:\n", "  devs:\n    - alice\n    - bob\n", "  empty: []\n", "access:\n  devs:\n    - sandbox\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("saved db missing %q:\n%s", want, out)
		}
	}
}

func TestCloneDBIsIndependent(t *testing.T) {
	original := &DB{
		Users: map[string]UserEntry{"alice": {IP: "10.8.0.10", Pub: "ALICE"}},
		UserGroups: map[string][]string{
			"devs": {"alice"},
		},
		VMs: map[string]string{"sandbox": "192.168.122.100"},
		Resources: map[string]ResourceEntry{
			"ssh@sandbox": {VM: "sandbox", Ports: ResourcePorts{{Protocol: "tcp", Port: 22}}},
		},
		Access: map[string][]string{"alice": {"sandbox"}},
	}

	cloned := cloneDB(original)
	cloned.Users["alice"] = UserEntry{IP: "10.8.0.20", Pub: "CHANGED"}
	cloned.UserGroups["devs"][0] = "bob"
	cloned.VMs["sandbox"] = "192.168.122.200"
	cloned.Resources["ssh@sandbox"] = ResourceEntry{VM: "sandbox", Ports: ResourcePorts{{Protocol: "tcp", Port: 2222}}}
	cloned.Access["alice"][0] = "changed"

	if original.Users["alice"].IP != "10.8.0.10" {
		t.Error("cloneDB() aliased the users map")
	}
	if original.UserGroups["devs"][0] != "alice" {
		t.Error("cloneDB() aliased the user groups map")
	}
	if original.VMs["sandbox"] != "192.168.122.100" {
		t.Error("cloneDB() aliased the VMs map")
	}
	if original.Resources["ssh@sandbox"].Ports[0].Port != 22 {
		t.Error("cloneDB() aliased the resources map")
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
