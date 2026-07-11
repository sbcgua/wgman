package main

import (
	"fmt"
	"sort"
	"strings"
)

type removePlan struct {
	UpdatedDB   *DB
	IPSetDeltas []IpsetDeltaOp
	PeerDeltas  []WGPeerDeltaOp
	User        string
	IP          string
	Pub         string
	Access      []string
}

func cmdRemove(gf *globalFlags, args []string, app *App) int {
	if len(args) != 1 {
		fmt.Fprintln(app.Stderr, "error: remove requires <name>")
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
		fmt.Fprintln(app.Stderr, "remove: FAILED (resolve hard errors or drift before removing users)")
		return 1
	}

	plan, err := planRemoveUser(cfg, db, args[0])
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	printRemovePlan(plan, app.Stdout)
	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "remove: dry-run, no changes applied")
		return 0
	}

	if !gf.yes {
		if !confirmAction(app.Stdin, app.Stdout, "Remove this user? [y/N] ") {
			fmt.Fprintln(app.Stdout, "remove: aborted")
			return 0
		}
	}

	if err, rollbackErr := applyRemoveUserPlan(gf.configDir, cfg.Interface, plan, app.Sys); err != nil {
		printApplyAndRollbackError(app.Stderr, err, rollbackErr)
		return 1
	}

	fmt.Fprintf(app.Stdout, "remove: removed %s\n", plan.User)
	return 0
}

func applyRemoveUserPlan(configDir, iface string, plan *removePlan, sys SystemAdapter) (applyErr, rollbackErr error) {
	applied, err := ApplyStateDeltasTracked(iface, plan.IPSetDeltas, plan.PeerDeltas, sys)
	if err != nil {
		return err, RollbackStateDeltas(iface, applied, sys)
	}
	if err := saveDBAtomic(configDir, plan.UpdatedDB); err != nil {
		return err, RollbackStateDeltas(iface, applied, sys)
	}
	return nil, nil
}

func planRemoveUser(cfg *Config, db *DB, user string) (*removePlan, error) {
	if !nameRe.MatchString(user) {
		return nil, fmt.Errorf("invalid user name %q", user)
	}
	entry, ok := db.Users[user]
	if !ok {
		return nil, fmt.Errorf("user %q not found", user)
	}

	updated := cloneDB(db)
	delete(updated.Users, user)
	delete(updated.Access, user)
	normalizeDBAccess(updated)
	if errs := validateDB(updated); len(errs) > 0 {
		return nil, fmt.Errorf("updated db.yaml would be invalid: %s", strings.Join(errs, "; "))
	}

	oldExpected := computeExpectedIPSets(db)
	newExpected := computeExpectedIPSets(updated)
	deltas := diffExpectedIPSets(cfg, oldExpected, newExpected)

	access := append([]string(nil), db.Access[user]...)
	sort.Strings(access)
	return &removePlan{
		UpdatedDB:   updated,
		IPSetDeltas: deltas,
		PeerDeltas:  []WGPeerDeltaOp{{User: user, PubKey: entry.Pub, AllowedIP: entry.IP, Action: WGPeerRemove}},
		User:        user,
		IP:          entry.IP,
		Pub:         entry.Pub,
		Access:      access,
	}, nil
}
