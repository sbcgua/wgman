package main

import (
	"fmt"
	"slices"
	"strings"
)

type modAccessOp struct {
	Add      bool
	Resource string
}

type modAccessPlan struct {
	UpdatedDB     *DB
	IPSetDeltas   []IpsetDeltaOp
	User          string
	AccessChanged bool
}

type modTogglePlan struct {
	UpdatedDB   *DB
	IPSetDeltas []IpsetDeltaOp
	PeerDeltas  []WGPeerDeltaOp
	NoOp        bool
}

// parseModExpression parses comma-separated access edits such as
// "+sandbox,-mailvm".
func parseModExpression(expr string) ([]modAccessOp, error) {
	if expr == "" {
		return nil, fmt.Errorf("mod expression is required")
	}
	parts := strings.Split(expr, ",")
	ops := make([]modAccessOp, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("empty mod operation")
		}
		if len(part) < 2 || (part[0] != '+' && part[0] != '-') {
			return nil, fmt.Errorf("operation %q must start with + or -", part)
		}
		resource := part[1:]
		if resource != "*" && !resourceNameRe.MatchString(resource) {
			return nil, fmt.Errorf("invalid resource name %q", resource)
		}
		key := resource
		if seen[key] {
			return nil, fmt.Errorf("duplicate resource %q in mod expression", resource)
		}
		seen[key] = true
		ops = append(ops, modAccessOp{Add: part[0] == '+', Resource: resource})
	}
	return ops, nil
}

func cmdMod(gf *globalFlags, args []string, app *App) int {
	if len(args) != 2 {
		fmt.Fprintln(app.Stderr, "error: mod requires <name> <+res1,-res2...|activate|deactivate>")
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
		fmt.Fprintln(app.Stderr, "mod: FAILED (resolve hard errors or drift before modifying access)")
		return 1
	}

	if isModToggleAction(args[1]) {
		return cmdModToggle(gf, cfg, db, args[0], args[1], app)
	}

	ops, err := parseModExpression(args[1])
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 2
	}

	plan, err := planModAccess(cfg, db, args[0], ops)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	if len(plan.IPSetDeltas) == 0 && !plan.AccessChanged {
		fmt.Fprintln(app.Stdout, "mod: no changes needed")
		return 0
	}

	changeCount := len(plan.IPSetDeltas)
	if plan.AccessChanged && len(plan.IPSetDeltas) == 0 {
		changeCount = 1
	}
	fmt.Fprintf(app.Stdout, "mod: planned changes (%d):\n", changeCount)
	if len(plan.IPSetDeltas) > 0 {
		printIPSetDeltas(plan.IPSetDeltas, app.Stdout)
	} else {
		fmt.Fprintf(app.Stdout, "  update db access for %s\n", plan.User)
	}

	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "mod: dry-run, no changes applied")
		return 0
	}

	if err := saveDBAtomic(gf.configDir, plan.UpdatedDB); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	if err := ApplyIPSetDeltas(plan.IPSetDeltas, app.Sys); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintf(app.Stdout, "mod: applied %d change(s)\n", changeCount)
	return 0
}

func isModToggleAction(action string) bool {
	return action == "activate" || action == "deactivate"
}

func cmdModToggle(gf *globalFlags, cfg *Config, db *DB, user, action string, app *App) int {
	plan, err := planModToggle(cfg, db, user, action)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	if plan.NoOp {
		fmt.Fprintf(app.Stdout, "mod: %s already %s; no changes needed\n", user, toggleStateName(action))
		return 0
	}

	changeCount := len(plan.IPSetDeltas)
	changeCount += len(plan.PeerDeltas)

	fmt.Fprintf(app.Stdout, "mod: planned changes (%d):\n", changeCount)
	printIPSetDeltas(plan.IPSetDeltas, app.Stdout)
	printPeerDeltas(plan.PeerDeltas, app.Stdout)

	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "mod: dry-run, no changes applied")
		return 0
	}

	appliedDeltas, liveErr := ApplyStateDeltasTracked(cfg.Interface, plan.IPSetDeltas, plan.PeerDeltas, app.Sys)
	if liveErr != nil {
		rollbackErr := RollbackStateDeltas(cfg.Interface, appliedDeltas, app.Sys)
		printApplyAndRollbackError(app.Stderr, liveErr, rollbackErr)
		return 1
	}

	if err := saveDBAtomic(gf.configDir, plan.UpdatedDB); err != nil {
		rollbackErr := RollbackStateDeltas(cfg.Interface, appliedDeltas, app.Sys)
		printApplyAndRollbackError(app.Stderr, err, rollbackErr)
		return 1
	}

	fmt.Fprintf(app.Stdout, "mod: applied %d change(s)\n", changeCount)
	return 0
}

func toggleStateName(action string) string {
	if action == "activate" {
		return "active"
	}
	return "inactive"
}

func planModToggle(cfg *Config, db *DB, user, action string) (*modTogglePlan, error) {
	if !nameRe.MatchString(user) {
		return nil, fmt.Errorf("invalid user name %q", user)
	}
	entry, ok := db.Users[user]
	if !ok {
		return nil, fmt.Errorf("user %q not found", user)
	}

	switch action {
	case "activate":
		if !entry.Inactive {
			return &modTogglePlan{UpdatedDB: db, NoOp: true}, nil
		}
	case "deactivate":
		if entry.Inactive {
			return &modTogglePlan{UpdatedDB: db, NoOp: true}, nil
		}
	default:
		return nil, fmt.Errorf("unknown mod action %q", action)
	}

	updated := cloneDB(db)
	updatedEntry := updated.Users[user]
	updatedEntry.Inactive = action == "deactivate"
	updated.Users[user] = updatedEntry

	if errs := validateDB(updated); len(errs) > 0 {
		return nil, fmt.Errorf("updated db.yaml would be invalid: %s", strings.Join(errs, "; "))
	}

	oldExpected := computeExpectedIPSets(db)
	newExpected := computeExpectedIPSets(updated)
	plan := &modTogglePlan{
		UpdatedDB:   updated,
		IPSetDeltas: diffExpectedIPSets(cfg, oldExpected, newExpected),
	}
	if action == "activate" {
		plan.PeerDeltas = []WGPeerDeltaOp{{User: user, PubKey: entry.Pub, AllowedIP: entry.IP, Action: WGPeerAdd}}
	} else {
		plan.PeerDeltas = []WGPeerDeltaOp{{User: user, PubKey: entry.Pub, AllowedIP: entry.IP, Action: WGPeerRemove}}
	}
	return plan, nil
}

func planModAccess(cfg *Config, db *DB, user string, ops []modAccessOp) (*modAccessPlan, error) {
	if !nameRe.MatchString(user) {
		return nil, fmt.Errorf("invalid user name %q", user)
	}
	if _, ok := db.Users[user]; !ok {
		return nil, fmt.Errorf("user %q not found", user)
	}

	updatedDb := cloneDB(db)
	accessSet := map[string]bool{}
	for _, resource := range updatedDb.Access[user] {
		accessSet[resource] = true
	}

	for _, op := range ops {
		if op.Resource != "*" {
			if _, ok := updatedDb.VMs[op.Resource]; !ok {
				if _, ok := updatedDb.Resources[op.Resource]; !ok {
					return nil, fmt.Errorf("unknown access target %q", op.Resource)
				}
			}
		}
		if op.Add {
			accessSet[op.Resource] = true
		} else {
			delete(accessSet, op.Resource)
		}
	}

	updatedDb.Access[user] = normalizeAccessSet(accessSet)
	normalizeDBAccess(updatedDb)
	if errs := validateDB(updatedDb); len(errs) > 0 {
		return nil, fmt.Errorf("updated db.yaml would be invalid: %s", strings.Join(errs, "; "))
	}

	oldExpected := computeExpectedIPSets(db)
	newExpected := computeExpectedIPSets(updatedDb)
	return &modAccessPlan{
		UpdatedDB:     updatedDb,
		IPSetDeltas:   diffExpectedIPSets(cfg, oldExpected, newExpected),
		User:          user,
		AccessChanged: !slices.Equal(db.Access[user], updatedDb.Access[user]),
	}, nil
}
