package main

// EffectiveAccessEntry is one merged access target for a user.
// Groups contains sorted user group names that grant Target when the target is
// not granted directly to the user.
type EffectiveAccessEntry struct {
	Target string
	Direct bool
	Groups []string
}

// EffectiveTargetUserEntry is one user whose effective access grants a target.
// Groups contains sorted user group names when the target is granted only by
// user group membership.
type EffectiveTargetUserEntry struct {
	User      string
	Direct    bool
	AllAccess bool
	Groups    []string
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

func effectiveUsersForTarget(db *DB, target string) []EffectiveTargetUserEntry {
	users := sortedKeys(db.Users)
	entries := make([]EffectiveTargetUserEntry, 0, len(users))
	for _, user := range users {
		direct := false
		allAccess := false
		for _, accessTarget := range db.Access[user] {
			if accessTargetGrantsTarget(accessTarget, target) {
				direct = true
				if target != "*" && accessTarget == "*" {
					allAccess = true
				}
				break
			}
		}

		inherited := map[string]bool{}
		for _, group := range userGroupsForUser(db, user) {
			for _, accessTarget := range db.Access[group] {
				if accessTargetGrantsTarget(accessTarget, target) {
					inherited[group] = true
					if target != "*" && accessTarget == "*" {
						allAccess = true
					}
					break
				}
			}
		}

		if direct || len(inherited) > 0 {
			entry := EffectiveTargetUserEntry{User: user, Direct: direct, AllAccess: allAccess}
			if !entry.Direct {
				entry.Groups = sortedBoolKeys(inherited)
			}
			entries = append(entries, entry)
		}
	}
	return entries
}

func accessTargetGrantsTarget(accessTarget, target string) bool {
	if target == "*" {
		return accessTarget == "*"
	}
	return accessTarget == "*" || accessTarget == target
}
