package main

import "testing"

func TestLoadConfig_Valid(t *testing.T) {
	h := newHelper(t)
	dir := h.makeTempDir()
	h.writeFile(dir, "config.yaml", `
interface: wg0
sets:
  all: wg_allow_all
  ip_matrix: wg_allow_matrix
  port_matrix: wg_allow_matrix_ports
`)
	cfg, err := LoadConfig(dir)
	h.assertNoError(err)
	if cfg.Interface != "wg0" {
		t.Errorf("interface = %q, want wg0", cfg.Interface)
	}
	if cfg.Sets.All != "wg_allow_all" {
		t.Errorf("sets.all = %q, want wg_allow_all", cfg.Sets.All)
	}
	if cfg.Sets.IPMatrix != "wg_allow_matrix" {
		t.Errorf("sets.ip_matrix = %q, want wg_allow_matrix", cfg.Sets.IPMatrix)
	}
	if cfg.Sets.PortMatrix != "wg_allow_matrix_ports" {
		t.Errorf("sets.port_matrix = %q, want wg_allow_matrix_ports", cfg.Sets.PortMatrix)
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
		yaml:    "sets:\n  all: a\n  ip_matrix: b\n  port_matrix: c\n",
		wantErr: "interface is required",
	},
	{
		name:    "missing sets.all",
		yaml:    "interface: wg0\nsets:\n  ip_matrix: b\n  port_matrix: c\n",
		wantErr: "sets.all is required",
	},
	{
		name:    "missing sets.ip_matrix",
		yaml:    "interface: wg0\nsets:\n  all: a\n  port_matrix: c\n",
		wantErr: "sets.ip_matrix is required",
	},
	{
		name:    "missing sets.port_matrix",
		yaml:    "interface: wg0\nsets:\n  all: a\n  ip_matrix: b\n",
		wantErr: "sets.port_matrix is required",
	},
	{
		name:    "unknown field",
		yaml:    "interface: wg0\nsets:\n  all: a\n  ip_matrix: b\n  port_matrix: c\nextra: bad\n",
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
