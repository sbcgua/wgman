package main

import "sort"

// EffectiveAccessEntry is one merged access target for a user.
// Groups contains sorted user group names that grant Target when the target is
// not granted directly to the user.
type EffectiveAccessEntry struct {
	Target string
	Direct bool
	Groups []string
}

func effectiveAccessForUser(db *DB, user string) []string {
	entries := effectiveAccessEntriesForUser(db, user)
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Target)
	}
	return out
}

func effectiveAccessEntriesForUser(db *DB, user string) []EffectiveAccessEntry {
	direct := map[string]bool{}
	for _, target := range db.Access[user] {
		direct[target] = true
	}

	inherited := map[string]map[string]bool{} // target -> user group set
	for _, group := range userGroupsForUser(db, user) {
		for _, target := range db.Access[group] {
			if inherited[target] == nil {
				inherited[target] = map[string]bool{}
			}
			inherited[target][group] = true
		}
	}

	if direct["*"] || len(inherited["*"]) > 0 {
		entry := EffectiveAccessEntry{Target: "*", Direct: direct["*"]}
		if !entry.Direct {
			entry.Groups = sortedBoolKeys(inherited["*"])
		}
		return []EffectiveAccessEntry{entry}
	}

	targets := map[string]bool{}
	for target := range direct {
		targets[target] = true
	}
	for target := range inherited {
		targets[target] = true
	}

	names := sortedBoolKeys(targets)
	entries := make([]EffectiveAccessEntry, 0, len(names))
	for _, target := range names {
		entry := EffectiveAccessEntry{Target: target, Direct: direct[target]}
		if !entry.Direct {
			entry.Groups = sortedBoolKeys(inherited[target])
		}
		entries = append(entries, entry)
	}
	return entries
}

func userGroupsForUser(db *DB, user string) []string {
	var groups []string
	for _, group := range sortedKeys(db.UserGroups) {
		for _, member := range db.UserGroups[group] {
			if member == user {
				groups = append(groups, group)
				break
			}
		}
	}
	return groups
}

func sortedBoolKeys(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
