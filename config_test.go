package main

import "testing"

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
