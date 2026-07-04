package main

import (
	"testing"
)

// ---- ipset parser tests ----

func TestParseIPSet_AllAccessSet(t *testing.T) {
	input := "create wg_allow_all hash:ip family inet hashsize 1024 maxelem 65536\n" +
		"add wg_allow_all 10.8.0.5\n"
	parsed, err := ParseIPSet(input)
	if err != nil {
		t.Fatalf("ParseIPSet: %v", err)
	}
	if parsed.SetName != "wg_allow_all" {
		t.Errorf("SetName = %q, want wg_allow_all", parsed.SetName)
	}
	if parsed.SetType != "hash:ip" {
		t.Errorf("SetType = %q, want hash:ip", parsed.SetType)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("entries count = %d, want 1", len(parsed.Entries))
	}
	if parsed.Entries[0].Entry != "10.8.0.5" {
		t.Errorf("entry = %q, want 10.8.0.5", parsed.Entries[0].Entry)
	}
	if parsed.Entries[0].Comment != "" {
		t.Errorf("comment = %q, want empty", parsed.Entries[0].Comment)
	}
}

func TestParseIPSet_MatrixSet(t *testing.T) {
	input := "create wg_allow_matrix hash:net,net family inet hashsize 1024 maxelem 65536 comment\n" +
		"add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n" +
		"add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n" +
		"add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n"
	parsed, err := ParseIPSet(input)
	if err != nil {
		t.Fatalf("ParseIPSet: %v", err)
	}
	if parsed.SetName != "wg_allow_matrix" {
		t.Errorf("SetName = %q, want wg_allow_matrix", parsed.SetName)
	}
	if parsed.SetType != "hash:net,net" {
		t.Errorf("SetType = %q, want hash:net,net", parsed.SetType)
	}
	if len(parsed.Entries) != 3 {
		t.Fatalf("entries count = %d, want 3", len(parsed.Entries))
	}
	if parsed.Entries[0].Entry != "10.8.0.10,192.168.122.100" {
		t.Errorf("entry[0] = %q", parsed.Entries[0].Entry)
	}
	if parsed.Entries[0].Comment != "alice -> sandbox" {
		t.Errorf("comment[0] = %q, want alice -> sandbox", parsed.Entries[0].Comment)
	}
	if parsed.Entries[2].Entry != "10.8.0.15,192.168.122.101" {
		t.Errorf("entry[2] = %q", parsed.Entries[2].Entry)
	}
	if parsed.Entries[2].Comment != "bob -> mailvm" {
		t.Errorf("comment[2] = %q", parsed.Entries[2].Comment)
	}
}

func TestParseIPSet_Empty(t *testing.T) {
	input := "create wg_allow_all hash:ip family inet hashsize 1024 maxelem 65536\n"
	parsed, err := ParseIPSet(input)
	if err != nil {
		t.Fatalf("ParseIPSet: %v", err)
	}
	if len(parsed.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(parsed.Entries))
	}
}

func TestParseIPSet_EntryWithoutComment(t *testing.T) {
	input := "create wg_allow_matrix hash:net,net family inet\n" +
		"add wg_allow_matrix 10.8.0.10,192.168.122.100\n"
	parsed, err := ParseIPSet(input)
	if err != nil {
		t.Fatalf("ParseIPSet: %v", err)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("entries count = %d, want 1", len(parsed.Entries))
	}
	if parsed.Entries[0].Comment != "" {
		t.Errorf("comment = %q, want empty", parsed.Entries[0].Comment)
	}
}

func TestParseIPSet_CommentWithSpaces(t *testing.T) {
	input := "create mySet hash:net,net family inet\n" +
		"add mySet 10.0.0.1,10.0.0.2 comment \"user name -> vm name\"\n"
	parsed, err := ParseIPSet(input)
	if err != nil {
		t.Fatalf("ParseIPSet: %v", err)
	}
	if parsed.Entries[0].Comment != "user name -> vm name" {
		t.Errorf("comment = %q, want %q", parsed.Entries[0].Comment, "user name -> vm name")
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
	{
		name:    "add before create",
		input:   "add mySet 10.0.0.1\n",
		wantErr: "add line before create line",
	},
	{
		name:    "duplicate create line",
		input:   "create mySet hash:ip family inet\ncreate mySet hash:ip family inet\n",
		wantErr: "duplicate create line",
	},
	{
		name:    "add set name mismatch",
		input:   "create mySet hash:ip family inet\nadd otherSet 10.0.0.1\n",
		wantErr: "does not match create set name",
	},
}

func TestParseIPSet_Errors(t *testing.T) {
	for _, tc := range ipsetParseErrorTests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHelper(t)
			_, err := ParseIPSet(tc.input)
			h.assertError(err, tc.wantErr)
		})
	}
}
