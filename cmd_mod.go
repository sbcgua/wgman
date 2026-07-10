package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type modAccessOp struct {
	Add      bool
	Resource string
}

type modTogglePlan struct {
	UpdatedDB *DB
	Deltas    []IpsetDeltaOp
	PeerAdd   *WGPeerDeltaOp
	PeerDel   *WGPeerDeltaOp
	NoOp      bool
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
		if resource != "*" && !nameRe.MatchString(resource) {
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

	updated, deltas, err := planModAccess(cfg, db, args[0], ops)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	dbChanged := !slices.Equal(db.Access[args[0]], updated.Access[args[0]])
	if len(deltas) == 0 && !dbChanged {
		fmt.Fprintln(app.Stdout, "mod: no changes needed")
		return 0
	}

	changeCount := len(deltas)
	if dbChanged && len(deltas) == 0 {
		changeCount = 1
	}
	fmt.Fprintf(app.Stdout, "mod: planned changes (%d):\n", changeCount)
	if len(deltas) > 0 {
		printDeltas(deltas, app.Stdout)
	} else {
		fmt.Fprintf(app.Stdout, "  update db access for %s\n", args[0])
	}

	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "mod: dry-run, no changes applied")
		return 0
	}

	if err := saveDBAtomic(gf.configDir, updated); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	if err := ApplyDeltas(deltas, app.Sys); err != nil {
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

	changeCount := len(plan.Deltas)
	if plan.PeerAdd != nil {
		changeCount++
	}
	if plan.PeerDel != nil {
		changeCount++
	}

	fmt.Fprintf(app.Stdout, "mod: planned changes (%d):\n", changeCount)
	printDeltas(plan.Deltas, app.Stdout)
	if plan.PeerAdd != nil {
		printPeerDeltas([]WGPeerDeltaOp{*plan.PeerAdd}, app.Stdout)
	}
	if plan.PeerDel != nil {
		printPeerDeltas([]WGPeerDeltaOp{*plan.PeerDel}, app.Stdout)
	}

	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "mod: dry-run, no changes applied")
		return 0
	}

	appliedDeltas, liveErr := applyModToggleLive(cfg.Interface, plan, app.Sys)
	if liveErr != nil {
		rollbackErr := rollbackModToggleLive(cfg.Interface, plan, appliedDeltas, app.Sys)
		printApplyAndRollbackError(app.Stderr, liveErr, rollbackErr)
		return 1
	}

	if err := saveDBAtomic(gf.configDir, plan.UpdatedDB); err != nil {
		rollbackErr := rollbackModToggleLive(cfg.Interface, plan, appliedDeltas, app.Sys)
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

	oldAll, oldMatrix := computeExpectedIPSets(db)
	newAll, newMatrix := computeExpectedIPSets(updated)
	plan := &modTogglePlan{
		UpdatedDB: updated,
		Deltas:    diffExpectedIPSets(cfg, oldAll, oldMatrix, newAll, newMatrix),
	}
	if action == "activate" {
		plan.PeerAdd = &WGPeerDeltaOp{User: user, PubKey: entry.Pub, AllowedIP: entry.IP, Add: true}
	} else {
		plan.PeerDel = &WGPeerDeltaOp{User: user, PubKey: entry.Pub, Remove: true}
	}
	return plan, nil
}

func applyModToggleLive(iface string, plan *modTogglePlan, sys SystemAdapter) ([]IpsetDeltaOp, error) {
	if plan.PeerAdd != nil {
		if err := ApplyPeerDeltas(iface, []WGPeerDeltaOp{*plan.PeerAdd}, sys); err != nil {
			return nil, err
		}
	}

	appliedDeltas, err := ApplyDeltasTracked(plan.Deltas, sys)
	if err != nil {
		return appliedDeltas, err
	}

	if plan.PeerDel != nil {
		if err := ApplyPeerDeltas(iface, []WGPeerDeltaOp{*plan.PeerDel}, sys); err != nil {
			return appliedDeltas, err
		}
	}
	return appliedDeltas, nil
}

func rollbackModToggleLive(iface string, plan *modTogglePlan, appliedDeltas []IpsetDeltaOp, sys SystemAdapter) error {
	var errs []string
	if plan.PeerDel != nil {
		user := plan.UpdatedDB.Users[plan.PeerDel.User]
		if err := sys.WGSetPeer(iface, plan.PeerDel.PubKey, user.IP); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if err := ApplyDeltas(InvertDeltas(appliedDeltas), sys); err != nil {
		errs = append(errs, err.Error())
	}
	if plan.PeerAdd != nil {
		if err := sys.WGDelPeer(iface, plan.PeerAdd.PubKey); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func planModAccess(cfg *Config, db *DB, user string, ops []modAccessOp) (*DB, []IpsetDeltaOp, error) {
	if !nameRe.MatchString(user) {
		return nil, nil, fmt.Errorf("invalid user name %q", user)
	}
	if _, ok := db.Users[user]; !ok {
		return nil, nil, fmt.Errorf("user %q not found", user)
	}

	updated := cloneDB(db)
	accessSet := map[string]bool{}
	for _, resource := range updated.Access[user] {
		accessSet[resource] = true
	}

	for _, op := range ops {
		if op.Resource != "*" {
			if _, ok := updated.VMs[op.Resource]; !ok {
				return nil, nil, fmt.Errorf("unknown VM %q", op.Resource)
			}
		}
		if op.Add {
			accessSet[op.Resource] = true
		} else {
			delete(accessSet, op.Resource)
		}
	}

	updated.Access[user] = normalizeAccessSet(accessSet)
	normalizeDBAccess(updated)
	if errs := validateDB(updated); len(errs) > 0 {
		return nil, nil, fmt.Errorf("updated db.yaml would be invalid: %s", strings.Join(errs, "; "))
	}

	oldAll, oldMatrix := computeExpectedIPSets(db)
	newAll, newMatrix := computeExpectedIPSets(updated)
	deltas := diffExpectedIPSets(cfg, oldAll, oldMatrix, newAll, newMatrix)
	return updated, deltas, nil
}
