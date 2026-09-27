package main

import (
	"testing"
)

// ---- WG dump parser tests ----

const wgDumpValid = "" +
	"SERVER_PRIV_KEY=\tSERVER_PUB_KEY=\t51820\toff\n" +
	"ALICE_PUB=\t(none)\t192.168.1.100:50001\t10.8.0.10/32\t1748000000\t102400\t204800\toff\n" +
	"BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n"

func TestParseWGDump_Valid(t *testing.T) {
	result, err := ParseWGDump(wgDumpValid)
	if err != nil {
		t.Fatalf("ParseWGDump: %v", err)
	}
	if result.ServerPubKey != "SERVER_PUB_KEY=" {
		t.Errorf("ServerPubKey = %q, want SERVER_PUB_KEY=", result.ServerPubKey)
	}
	if len(result.Peers) != 2 {
		t.Fatalf("peers count = %d, want 2", len(result.Peers))
	}

	alice := result.Peers[0]
	if alice.PublicKey != "ALICE_PUB=" {
		t.Errorf("alice pubkey = %q", alice.PublicKey)
	}
	if alice.AllowedIP != "10.8.0.10" {
		t.Errorf("alice allowed-ip = %q, want 10.8.0.10 (normalized)", alice.AllowedIP)
	}
	if alice.Endpoint != "192.168.1.100:50001" {
		t.Errorf("alice endpoint = %q", alice.Endpoint)
	}
	if alice.LatestHandshake != 1748000000 {
		t.Errorf("alice handshake = %d", alice.LatestHandshake)
	}
	if alice.RxBytes != 102400 {
		t.Errorf("alice rx = %d", alice.RxBytes)
	}
	if alice.TxBytes != 204800 {
		t.Errorf("alice tx = %d", alice.TxBytes)
	}
	if alice.Keepalive != "off" {
		t.Errorf("alice keepalive = %q", alice.Keepalive)
	}

	bob := result.Peers[1]
	if bob.Endpoint != "(none)" {
		t.Errorf("bob endpoint = %q, want (none)", bob.Endpoint)
	}
	if bob.AllowedIP != "10.8.0.15" {
		t.Errorf("bob allowed-ip = %q, want 10.8.0.15", bob.AllowedIP)
	}
	if bob.LatestHandshake != 0 {
		t.Errorf("bob handshake = %d, want 0", bob.LatestHandshake)
	}
}

func TestParseWGDump_NoPeers(t *testing.T) {
	dump := "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n"
	result, err := ParseWGDump(dump)
	if err != nil {
		t.Fatalf("ParseWGDump: %v", err)
	}
	if result.ServerPubKey != "SERVER_PUB=" {
		t.Errorf("ServerPubKey = %q", result.ServerPubKey)
	}
	if len(result.Peers) != 0 {
		t.Errorf("expected no peers, got %d", len(result.Peers))
	}
}

var wgDumpErrorTests = []struct {
	name    string
	input   string
	wantErr string
}{
	{
		name:    "empty output",
		input:   "",
		wantErr: "empty output",
	},
	{
		name:    "server line too short",
		input:   "A\tB\t51820\n",
		wantErr: "server line has 3 fields",
	},
	{
		name:    "peer line too few fields",
		input:   "PRIV=\tPUB=\t51820\toff\n" + "PEER_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\n",
		wantErr: "peer line has 7 fields",
	},
	{
		name:    "peer non-numeric handshake",
		input:   "PRIV=\tPUB=\t51820\toff\n" + "PEER_PUB=\t(none)\t(none)\t10.8.0.5/32\tBAD\t0\t0\toff\n",
		wantErr: "parse latest-handshake",
	},
	{
		name:    "peer non-numeric rx",
		input:   "PRIV=\tPUB=\t51820\toff\n" + "PEER_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\tBAD\t0\toff\n",
		wantErr: "parse transfer-rx",
	},
	{
		name:    "peer non-numeric tx",
		input:   "PRIV=\tPUB=\t51820\toff\n" + "PEER_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\tBAD\toff\n",
		wantErr: "parse transfer-tx",
	},
	{
		name:    "peer multiple allowed-ips",
		input:   "PRIV=\tPUB=\t51820\toff\n" + "PEER_PUB=\t(none)\t(none)\t10.8.0.5/32,10.8.0.6/32\t0\t0\t0\toff\n",
		wantErr: "unexpected multiple allowed-ips",
	},
	{
		name:    "peer invalid allowed-ip",
		input:   "PRIV=\tPUB=\t51820\toff\n" + "PEER_PUB=\t(none)\t(none)\tnotanip\t0\t0\t0\toff\n",
		wantErr: "invalid allowed-ip",
	},
}

func TestParseWGDump_Errors(t *testing.T) {
	for _, tc := range wgDumpErrorTests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHelper(t)
			_, err := ParseWGDump(tc.input)
			h.assertError(err, tc.wantErr)
		})
	}
}

var normalizeWGAllowedIPTests = []struct {
	input   string
	want    string
	wantErr string
}{
	{"10.8.0.5/32", "10.8.0.5", ""},
	{"10.8.0.5", "10.8.0.5", ""},
	{"10.8.0.0/24", "10.8.0.0/24", ""},
	{"10.8.0.5/32,10.8.0.6/32", "", "unexpected multiple"},
	{"notanip", "", "invalid allowed-ip"},
}

func TestNormalizeWGAllowedIP(t *testing.T) {
	for _, tc := range normalizeWGAllowedIPTests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := normalizeWGAllowedIP(tc.input)
			if tc.wantErr != "" {
				if err == nil || !containsStr(err.Error(), tc.wantErr) {
					t.Errorf("normalizeWGAllowedIP(%q) error = %v, want containing %q", tc.input, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeWGAllowedIP(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("normalizeWGAllowedIP(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
