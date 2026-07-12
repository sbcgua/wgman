package main

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

type userGroupOp struct {
	Add  bool
	User string
}

type userGroupPlan struct {
	UpdatedDB         *DB
	IPSetDeltas       []IpsetDeltaOp
	Group             string
	MembershipChanged bool
}

func cmdUserGroup(gf *globalFlags, args []string, app *App) int {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(app.Stderr, "error: usergroup requires <group> [+user,-user...]")
		return 2
	}
	if !app.Sys.IsRoot() {
		fmt.Fprintln(app.Stderr, "error: wgman must be run as root")
		return 1
	}

	cfg, err := LoadConfig(gf.configDir)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	db, err := LoadDB(gf.configDir)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	result := Check(cfg, db, app.Sys)
	if !result.OK() {
		printCheckErrors(result, app.Stderr)
		fmt.Fprintln(app.Stderr, "usergroup: FAILED (resolve hard errors or drift before changing user groups)")
		return 1
	}

	if len(args) == 1 {
		return runUserGroupList(db, args[0], app.Stdout, app.Stderr)
	}

	ops, err := parseUserGroupExpression(args[1])
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 2
	}
	plan, err := planUserGroup(cfg, db, args[0], ops)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	if len(plan.IPSetDeltas) == 0 && !plan.MembershipChanged {
		fmt.Fprintln(app.Stdout, "usergroup: no changes needed")
		return 0
	}

	changeCount := len(plan.IPSetDeltas)
	if plan.MembershipChanged && changeCount == 0 {
		changeCount = 1
	}
	fmt.Fprintf(app.Stdout, "usergroup: planned changes (%d):\n", changeCount)
	if plan.MembershipChanged {
		fmt.Fprintf(app.Stdout, "  update user group %s membership\n", plan.Group)
	}
	printIPSetDeltas(plan.IPSetDeltas, app.Stdout)

	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "usergroup: dry-run, no changes applied")
		return 0
	}

	if err := applyUserGroupPlan(gf.configDir, plan, app.Sys); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintf(app.Stdout, "usergroup: applied %d change(s)\n", changeCount)
	return 0
}

func applyUserGroupPlan(configDir string, plan *userGroupPlan, sys SystemAdapter) error {
	if err := saveDBAtomic(configDir, plan.UpdatedDB); err != nil {
		return err
	}
	return ApplyIPSetDeltas(plan.IPSetDeltas, sys)
}

func runUserGroupList(db *DB, group string, stdout, stderr io.Writer) int {
	if !nameRe.MatchString(group) {
		fmt.Fprintf(stderr, "error: invalid user group name %q\n", group)
		return 2
	}
	members, ok := db.UserGroups[group]
	if !ok {
		fmt.Fprintf(stderr, "error: user group %q not found\n", group)
		fmt.Fprintln(stderr, "usergroup: FAILED")
		return 1
	}
	sorted := append([]string(nil), members...)
	sort.Strings(sorted)
	memberText := strings.Join(sorted, ",")
	if memberText == "" {
		memberText = "(none)"
	}
	fmt.Fprintf(stdout, "%s: %s\n", group, memberText)
	return 0
}

func parseUserGroupExpression(expr string) ([]userGroupOp, error) {
	if expr == "" {
		return nil, fmt.Errorf("user group expression is required")
	}
	parts := strings.Split(expr, ",")
	ops := make([]userGroupOp, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("empty user group operation")
		}
		if len(part) < 2 || (part[0] != '+' && part[0] != '-') {
			return nil, fmt.Errorf("operation %q must start with + or -", part)
		}
		user := part[1:]
		if !nameRe.MatchString(user) {
			return nil, fmt.Errorf("invalid user name %q", user)
		}
		if seen[user] {
			return nil, fmt.Errorf("duplicate user %q in user group expression", user)
		}
		seen[user] = true
		ops = append(ops, userGroupOp{Add: part[0] == '+', User: user})
	}
	return ops, nil
}

func planUserGroup(cfg *Config, db *DB, group string, ops []userGroupOp) (*userGroupPlan, error) {
	if !nameRe.MatchString(group) {
		return nil, fmt.Errorf("invalid user group name %q", group)
	}
	if _, ok := db.Users[group]; ok {
		return nil, fmt.Errorf("user group name %q conflicts with existing user", group)
	}
	for existing := range db.Users {
		if caseFold(existing) == caseFold(group) {
			return nil, fmt.Errorf("user group name %q conflicts with user %q (case)", group, existing)
		}
	}

	hasAdd := false
	for _, op := range ops {
		if _, ok := db.Users[op.User]; !ok {
			return nil, fmt.Errorf("user %q not found", op.User)
		}
		if op.Add {
			hasAdd = true
		}
	}

	_, exists := db.UserGroups[group]
	if !exists && !hasAdd {
		return nil, fmt.Errorf("user group %q not found", group)
	}
	if !exists {
		for existing := range db.UserGroups {
			if caseFold(existing) == caseFold(group) {
				return nil, fmt.Errorf("user group name %q conflicts with %q (case)", group, existing)
			}
		}
	}

	updated := cloneDB(db)
	if updated.UserGroups == nil {
		updated.UserGroups = map[string][]string{}
	}
	members := map[string]bool{}
	for _, member := range updated.UserGroups[group] {
		members[member] = true
	}
	for _, op := range ops {
		if op.Add {
			members[op.User] = true
		} else {
			delete(members, op.User)
		}
	}
	updated.UserGroups[group] = sortedBoolKeys(members)

	if errs := validateDB(updated); len(errs) > 0 {
		return nil, fmt.Errorf("updated db.yaml would be invalid: %s", strings.Join(errs, "; "))
	}

	oldExpected := computeExpectedIPSets(db)
	newExpected := computeExpectedIPSets(updated)
	return &userGroupPlan{
		UpdatedDB:         updated,
		IPSetDeltas:       diffExpectedIPSets(cfg, oldExpected, newExpected),
		Group:             group,
		MembershipChanged: !slices.Equal(db.UserGroups[group], updated.UserGroups[group]),
	}, nil
}
