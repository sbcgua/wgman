package main

import (
	"testing"
)

// ---- ipset parser tests ----

func TestParseIPSetEntries_AllAccessSet(t *testing.T) {
	input := "create wg_allow_all hash:net family inet hashsize 1024 maxelem 65536\n" +
		"add wg_allow_all 10.8.0.5\n"
	entries, err := ParseIPSetEntries(input)
	if err != nil {
		t.Fatalf("ParseIPSetEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries count = %d, want 1", len(entries))
	}
	if entries[0].Entry != "10.8.0.5" {
		t.Errorf("entry = %q, want 10.8.0.5", entries[0].Entry)
	}
	if entries[0].Comment != "" {
		t.Errorf("comment = %q, want empty", entries[0].Comment)
	}
}

func TestParseIPSetEntries_MatrixSet(t *testing.T) {
	input := "create wg_allow_matrix hash:net,net family inet hashsize 1024 maxelem 65536 comment\n" +
		"add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n" +
		"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n" +
		"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n"
	entries, err := ParseIPSetEntries(input)
	if err != nil {
		t.Fatalf("ParseIPSetEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries count = %d, want 3", len(entries))
	}

	if entries[0].Entry != "10.8.0.10,192.168.122.100" {
		t.Errorf("entry[0] = %q", entries[0].Entry)
	}
	if entries[0].Comment != "alice -> sandbox" {
		t.Errorf("comment[0] = %q, want alice -> sandbox", entries[0].Comment)
	}
	if entries[2].Entry != "10.8.0.15,192.168.122.101" {
		t.Errorf("entry[2] = %q", entries[2].Entry)
	}
	if entries[2].Comment != "bob -> mailvm" {
		t.Errorf("comment[2] = %q", entries[2].Comment)
	}
}

func TestParseIPSetEntries_Empty(t *testing.T) {
	input := "create wg_allow_all hash:net family inet hashsize 1024 maxelem 65536\n"
	entries, err := ParseIPSetEntries(input)
	if err != nil {
		t.Fatalf("ParseIPSetEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseIPSetEntries_EntryWithoutComment(t *testing.T) {
	input := "create wg_allow_matrix hash:net,net family inet\n" +
		"add wg_allow_matrix 10.8.0.10,192.168.122.100\n"
	entries, err := ParseIPSetEntries(input)
	if err != nil {
		t.Fatalf("ParseIPSetEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries count = %d, want 1", len(entries))
	}
	if entries[0].Comment != "" {
		t.Errorf("comment = %q, want empty", entries[0].Comment)
	}
}

func TestParseIPSetEntries_CommentWithSpaces(t *testing.T) {
	input := "create mySet hash:net,net family inet\n" +
		"add mySet 10.0.0.1,10.0.0.2 comment \"user name -> vm name\"\n"
	entries, err := ParseIPSetEntries(input)
	if err != nil {
		t.Fatalf("ParseIPSetEntries: %v", err)
	}
	if entries[0].Comment != "user name -> vm name" {
		t.Errorf("comment = %q, want %q", entries[0].Comment, "user name -> vm name")
	}
}

var ipsetParseErrorTests = []struct {
	name    string
	input   string
	wantErr string
}{
	{
		name:    "unexpected line",
		input:   "create mySet hash:net family inet\nfoo bar baz\n",
		wantErr: "unexpected line",
	},
	{
		name:    "add line too short",
		input:   "add mySet\n",
		wantErr: "add line too short",
	},
	{
		name:    "unexpected trailing content",
		input:   "create mySet hash:net family inet\nadd mySet 10.0.0.1 garbage here\n",
		wantErr: "unexpected trailing content",
	},
}

func TestParseIPSetEntries_Errors(t *testing.T) {
	for _, tc := range ipsetParseErrorTests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHelper(t)
			_, err := ParseIPSetEntries(tc.input)
			h.assertError(err, tc.wantErr)
		})
	}
}
