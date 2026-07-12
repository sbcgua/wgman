package main

import (
	"reflect"
	"testing"
)

func TestEffectiveAccessEntriesForUserAnnotatesGroupOnlyAccess(t *testing.T) {
	db := makeTestDB()
	db.UserGroups = map[string][]string{
		"devs": {"alice", "bob"},
		"ops":  {"alice"},
	}
	db.Access["devs"] = []string{"mailvm"}
	db.Access["ops"] = []string{"sandbox"}

	got := effectiveAccessEntriesForUser(db, "alice")
	want := []EffectiveAccessEntry{
		{Target: "mailvm", Groups: []string{"devs"}},
		{Target: "sandbox", Direct: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("effective access = %#v, want %#v", got, want)
	}
}

func TestEffectiveAccessEntriesForUserMergesMultipleGroups(t *testing.T) {
	db := makeTestDB()
	db.Access["alice"] = nil
	db.UserGroups = map[string][]string{
		"devs": {"alice"},
		"ops":  {"alice"},
	}
	db.Access["devs"] = []string{"mailvm"}
	db.Access["ops"] = []string{"mailvm"}

	got := effectiveAccessEntriesForUser(db, "alice")
	want := []EffectiveAccessEntry{{Target: "mailvm", Groups: []string{"devs", "ops"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("effective access = %#v, want %#v", got, want)
	}
}

func TestEffectiveAccessEntriesForUserStarDominates(t *testing.T) {
	db := makeTestDB()
	db.UserGroups = map[string][]string{"admins": {"alice"}}
	db.Access["admins"] = []string{"*"}

	got := effectiveAccessEntriesForUser(db, "alice")
	want := []EffectiveAccessEntry{{Target: "*", Groups: []string{"admins"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("effective access = %#v, want %#v", got, want)
	}
}
