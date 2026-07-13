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

func TestEffectiveUsersForTargetIncludesStarAndExactAccess(t *testing.T) {
	db := makeTestDB()
	db.UserGroups = map[string][]string{
		"admins": {"alice"},
		"devs":   {"bob"},
	}
	db.Access["admins"] = []string{"*"}
	db.Access["devs"] = []string{"sandbox"}

	got := effectiveUsersForTarget(db, "sandbox")
	want := []EffectiveTargetUserEntry{
		{User: "admin", Direct: true},
		{User: "alice", Direct: true},
		{User: "bob", Direct: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("effective target users = %#v, want %#v", got, want)
	}
}

func TestEffectiveUsersForTargetAnnotatesGroupOnlyAccess(t *testing.T) {
	db := makeTestDB()
	db.Access["alice"] = nil
	db.UserGroups = map[string][]string{
		"admins": {"alice"},
		"devs":   {"alice"},
	}
	db.Access["admins"] = []string{"*"}
	db.Access["devs"] = []string{"sandbox"}

	got := effectiveUsersForTarget(db, "sandbox")
	want := []EffectiveTargetUserEntry{
		{User: "admin", Direct: true},
		{User: "alice", Groups: []string{"admins", "devs"}},
		{User: "bob", Direct: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("effective target users = %#v, want %#v", got, want)
	}
}

func TestEffectiveUsersForStarMatchesOnlyAllAccess(t *testing.T) {
	db := makeTestDB()
	db.Access["admin"] = nil
	db.UserGroups = map[string][]string{"admins": {"alice"}}
	db.Access["admins"] = []string{"*"}

	got := effectiveUsersForTarget(db, "*")
	want := []EffectiveTargetUserEntry{{User: "alice", Groups: []string{"admins"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("effective target users = %#v, want %#v", got, want)
	}
}
