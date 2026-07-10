package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestSortedKeys(t *testing.T) {
	input := map[string]int{
		"charlie": 3,
		"alice":   1,
		"bob":     2,
	}

	got := sortedKeys(input)
	want := []string{"alice", "bob", "charlie"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortedKeys() = %#v, want %#v", got, want)
	}
}

func TestIsValidIPv4OrCIDR(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "10.8.0.5", want: true},
		{input: "10.8.0.0/24", want: true},
		{input: "not-an-ip", want: false},
		{input: "2001:db8::1", want: false},
		{input: "2001:db8::/32", want: false},
	}

	for _, tc := range tests {
		if got := isValidIPv4OrCIDR(tc.input); got != tc.want {
			t.Errorf("isValidIPv4OrCIDR(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestCaseFold(t *testing.T) {
	if got, want := caseFold("Alice_BOB-42"), "alice_bob-42"; got != want {
		t.Errorf("caseFold() = %q, want %q", got, want)
	}
}

func TestEndpointHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "(none)", want: "(none)"},
		{input: "192.168.1.100:50001", want: "192.168.1.100"},
		{input: "10.0.0.1:51820", want: "10.0.0.1"},
		{input: "myhost.example.com:12345", want: "myhost.example.com"},
	}

	for _, tc := range tests {
		got := endpointHost(tc.input)
		if got != tc.want {
			t.Errorf("endpointHost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestConfirmAction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "lower yes", input: "y\n", want: true},
		{name: "upper yes trimmed", input: " Y \n", want: true},
		{name: "word yes rejected", input: "yes\n", want: false},
		{name: "default no", input: "\n", want: false},
		{name: "eof", input: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got := confirmAction(strings.NewReader(tc.input), &out, "Continue? [y/N] ")
			if got != tc.want {
				t.Errorf("confirmAction() = %v, want %v", got, tc.want)
			}
			if out.String() != "Continue? [y/N] " {
				t.Errorf("prompt output = %q", out.String())
			}
		})
	}
}
