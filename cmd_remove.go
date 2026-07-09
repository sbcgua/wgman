package main

import (
	"bufio"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type removePlan struct {
	UpdatedDB *DB
	Deltas    []IpsetDeltaOp
	User      string
	IP        string
	Pub       string
	Access    []string
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
		fmt.Fprint(app.Stdout, "Remove this user? [y/N] ")
		scanner := bufio.NewScanner(app.Stdin)
		answer := ""
		if scanner.Scan() {
			answer = strings.TrimSpace(scanner.Text())
		}
		if answer != "y" && answer != "Y" {
			fmt.Fprintln(app.Stdout, "remove: aborted")
			return 0
		}
	}

	appliedDeltas, err := ApplyDeltasTracked(plan.Deltas, app.Sys)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	if err := app.Sys.WGDelPeer(cfg.Interface, plan.Pub); err != nil {
		rollbackErr := ApplyDeltas(InvertDeltas(appliedDeltas), app.Sys)
		printApplyAndRollbackError(app.Stderr, err, rollbackErr)
		return 1
	}
	if err := saveDBAtomic(gf.configDir, plan.UpdatedDB); err != nil {
		rollbackErr := rollbackRemoveLiveState(cfg.Interface, plan.Pub, plan.IP, appliedDeltas, app.Sys)
		printApplyAndRollbackError(app.Stderr, err, rollbackErr)
		return 1
	}

	fmt.Fprintf(app.Stdout, "remove: removed %s\n", plan.User)
	return 0
}

func rollbackRemoveLiveState(iface, pubKey, ip string, appliedDeltas []IpsetDeltaOp, sys SystemAdapter) error {
	var errs []string
	if err := sys.WGSetPeer(iface, pubKey, ip); err != nil {
		errs = append(errs, err.Error())
	}
	if err := ApplyDeltas(InvertDeltas(appliedDeltas), sys); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
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

	oldAll, oldMatrix := computeExpectedIPSets(db)
	newAll, newMatrix := computeExpectedIPSets(updated)
	deltas := diffExpectedIPSets(cfg, oldAll, oldMatrix, newAll, newMatrix)

	access := append([]string(nil), db.Access[user]...)
	sort.Strings(access)
	return &removePlan{
		UpdatedDB: updated,
		Deltas:    deltas,
		User:      user,
		IP:        entry.IP,
		Pub:       entry.Pub,
		Access:    access,
	}, nil
}
