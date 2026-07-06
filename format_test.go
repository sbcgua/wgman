package main

import (
	"testing"
	"time"
)

var formatBytesTests = []struct {
	n    int64
	want string
}{
	{0, "0B"},
	{512, "512B"},
	{1023, "1023B"},
	{1024, "1.00Kb"},
	{1536, "1.50Kb"},
	{1024 * 1024, "1.00Mb"},
	// 2.07 * 1024 * 1024 ≈ 2170552 bytes → "2.07Mb"
	{2170552, "2.07Mb"},
	{1024 * 1024 * 1024, "1.00Gb"},
}

func TestFormatBytes(t *testing.T) {
	for _, tc := range formatBytesTests {
		got := formatBytes(tc.n)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestFormatBytesColor(t *testing.T) {
	tests := []struct {
		name    string
		n       int64
		enabled bool
		want    string
	}{
		{name: "disabled", n: 1024, enabled: false, want: "1.00Kb"},
		{name: "zero", n: 0, enabled: true, want: ansiGrey + "0B" + ansiReset},
		{name: "bytes unit", n: 512, enabled: true, want: "512" + ansiDimCyan + "B" + ansiReset},
		{name: "kilobytes unit", n: 1024, enabled: true, want: "1.00" + ansiDimCyan + "Kb" + ansiReset},
		{name: "megabytes unit", n: 1024 * 1024, enabled: true, want: "1.00" + ansiDimCyan + "Mb" + ansiReset},
		{name: "gigabytes unit", n: 1024 * 1024 * 1024, enabled: true, want: "1.00" + ansiDimCyan + "Gb" + ansiReset},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatBytesColor(tc.n, tc.enabled)
			if got != tc.want {
				t.Errorf("formatBytesColor(%d, %v) = %q, want %q", tc.n, tc.enabled, got, tc.want)
			}
		})
	}
}

func TestFormatHandshake(t *testing.T) {
	base := int64(1748000000)
	now := time.Unix(base, 0)

	cases := []struct {
		ts   int64
		want string
	}{
		{0, "never"},
		{base - 30, "30s"},
		{base - 90, "1m30s"},           // 90s = 1m30s
		{base - 3661, "1h1m1s"},        // 3661s = 1h1m1s
		{base - 258520, "2d23h48m40s"}, // 2*86400 + 23*3600 + 48*60 + 40
	}

	for _, tc := range cases {
		got := formatHandshake(tc.ts, now)
		if got != tc.want {
			t.Errorf("formatHandshake(%d) = %q, want %q", tc.ts, got, tc.want)
		}
	}
}

func TestFormatHandshake_FutureTimestamp(t *testing.T) {
	now := time.Unix(1000, 0)
	// timestamp in the future should report 0s age
	got := formatHandshake(2000, now)
	if got != "0s" {
		t.Errorf("future timestamp: got %q, want 0s", got)
	}
}

var endpointHostTests = []struct {
	input, want string
}{
	{"(none)", "(none)"},
	{"192.168.1.100:50001", "192.168.1.100"},
	{"10.0.0.1:51820", "10.0.0.1"},
	{"myhost.example.com:12345", "myhost.example.com"},
}

func TestEndpointHost(t *testing.T) {
	for _, tc := range endpointHostTests {
		got := endpointHost(tc.input)
		if got != tc.want {
			t.Errorf("endpointHost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
