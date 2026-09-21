package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type recreatePlan struct {
	UpdatedDB  *DB
	User       string
	IP         string
	OldPubKey  string
	PrivateKey string
	PubKey     string
	Inactive   bool
}

func cmdRecreate(gf *globalFlags, args []string, app *App) int {
	if rejectUnsupportedDryRun("recreate", gf, app.Stderr) {
		return 2
	}
	if len(args) != 1 {
		fmt.Fprintln(app.Stderr, "error: recreate requires <name>")
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
		fmt.Fprintln(app.Stderr, "recreate: FAILED (resolve hard errors or drift before recreating users)")
		return 1
	}
	if result.WGDump == nil {
		fmt.Fprintln(app.Stderr, "recreate: no WireGuard data available")
		return 1
	}

	plan, err := planRecreateUser(db, args[0], app.Sys)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	clientConfig, err := renderClientConfig(
		filepath.Join(gf.configDir, "user.conf.template"),
		plan.PrivateKey, plan.IP, result.WGDump.ServerPubKey,
	)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	clientConfigPath := plan.User + ".vpn.conf"
	if err := writeRecreatedClientConfig(clientConfigPath, clientConfig); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	if err, rollbackErr := applyRecreateUserPlan(gf.configDir, cfg.Interface, plan, app.Sys); err != nil {
		_ = os.Remove(clientConfigPath)
		printApplyAndRollbackError(app.Stderr, err, rollbackErr)
		return 1
	}

	fmt.Fprintf(app.Stdout, "recreate: recreated %s\n", plan.User)
	return 0
}

func planRecreateUser(db *DB, user string, sys SystemAdapter) (*recreatePlan, error) {
	if !nameRe.MatchString(user) {
		return nil, fmt.Errorf("invalid user name %q", user)
	}
	entry, ok := db.Users[user]
	if !ok {
		return nil, fmt.Errorf("user %q not found", user)
	}
	privateKey, err := sys.WGGenKey()
	if err != nil {
		return nil, err
	}
	pubKey, err := sys.WGPubKey(privateKey)
	if err != nil {
		return nil, err
	}
	if pubKey == "" {
		return nil, fmt.Errorf("generated public key is empty")
	}
	for existing, other := range db.Users {
		if other.Pub == pubKey {
			return nil, fmt.Errorf("generated public key conflicts with existing user %q", existing)
		}
	}

	updated := cloneDB(db)
	updatedEntry := updated.Users[user]
	updatedEntry.Pub = pubKey
	updated.Users[user] = updatedEntry
	if errs := validateDB(updated); len(errs) > 0 {
		return nil, fmt.Errorf("updated db.yaml would be invalid: %s", strings.Join(errs, "; "))
	}
	return &recreatePlan{
		UpdatedDB:  updated,
		User:       user,
		IP:         entry.IP,
		OldPubKey:  entry.Pub,
		PrivateKey: privateKey,
		PubKey:     pubKey,
		Inactive:   entry.Inactive,
	}, nil
}

func applyRecreateUserPlan(configDir, iface string, plan *recreatePlan, sys SystemAdapter) (applyErr, rollbackErr error) {
	if plan.Inactive {
		return saveDBAtomic(configDir, plan.UpdatedDB), nil
	}
	if err := sys.WGDelPeer(iface, plan.OldPubKey); err != nil {
		return fmt.Errorf("remove WireGuard peer for %q: %w", plan.User, err), nil
	}
	if err := sys.WGSetPeer(iface, plan.PubKey, plan.IP); err != nil {
		return fmt.Errorf("add WireGuard peer for %q: %w", plan.User, err),
			restoreRecreatedPeer(iface, plan, false, sys)
	}
	if err := saveDBAtomic(configDir, plan.UpdatedDB); err != nil {
		return err, restoreRecreatedPeer(iface, plan, true, sys)
	}
	return nil, nil
}

func restoreRecreatedPeer(iface string, plan *recreatePlan, removeNew bool, sys SystemAdapter) error {
	var errs []string
	if removeNew {
		if err := sys.WGDelPeer(iface, plan.PubKey); err != nil {
			errs = append(errs, fmt.Sprintf("remove replacement peer: %v", err))
		}
	}
	if err := sys.WGSetPeer(iface, plan.OldPubKey, plan.IP); err != nil {
		errs = append(errs, fmt.Sprintf("restore original peer: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
