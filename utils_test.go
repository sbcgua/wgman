package main

import (
	"reflect"
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
