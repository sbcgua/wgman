package main

import (
	"fmt"
	"sort"
	"strings"
)

type modAccessOp struct {
	Add      bool
	Resource string
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
		fmt.Fprintln(app.Stderr, "error: mod requires <name> <+res1,-res2...>")
		return 2
	}
	if !app.Sys.IsRoot() {
		fmt.Fprintln(app.Stderr, "error: wgman must be run as root")
		return 1
	}

	ops, err := parseModExpression(args[1])
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 2
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

	updated, deltas, err := planModAccess(cfg, db, args[0], ops)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	if len(deltas) == 0 {
		fmt.Fprintln(app.Stdout, "mod: no changes needed")
		return 0
	}

	fmt.Fprintf(app.Stdout, "mod: planned changes (%d):\n", len(deltas))
	printDeltas(deltas, app.Stdout)

	if gf.dryRun {
		fmt.Fprintln(app.Stdout, "mod: dry-run, no changes applied")
		return 0
	}

	if err := SaveDBAtomic(gf.configDir, updated); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	if err := ApplyDeltas(deltas, app.Sys); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintf(app.Stdout, "mod: applied %d change(s)\n", len(deltas))
	return 0
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
	if result := ValidateOffline(cfg, updated); !result.OK() {
		return nil, nil, fmt.Errorf("updated db.yaml would be invalid: %s", strings.Join(result.HardErrors, "; "))
	}

	oldAll, oldMatrix := computeExpectedIPSets(db)
	newAll, newMatrix := computeExpectedIPSets(updated)
	deltas := diffExpectedIPSets(cfg, oldAll, oldMatrix, newAll, newMatrix)
	return updated, deltas, nil
}

func diffExpectedIPSets(cfg *Config, oldAll, oldMatrix, newAll, newMatrix map[string]string) []IpsetDeltaOp {
	var deltas []IpsetDeltaOp
	deltas = append(deltas, diffExpectedIPSet(cfg.Sets.All, oldAll, newAll)...)
	deltas = append(deltas, diffExpectedIPSet(cfg.Sets.Matrix, oldMatrix, newMatrix)...)
	sort.Slice(deltas, func(i, j int) bool {
		if deltas[i].Set != deltas[j].Set {
			return deltas[i].Set < deltas[j].Set
		}
		if deltas[i].Entry != deltas[j].Entry {
			return deltas[i].Entry < deltas[j].Entry
		}
		return !deltas[i].Add && deltas[j].Add
	})
	return deltas
}

func diffExpectedIPSet(setname string, oldExpected, newExpected map[string]string) []IpsetDeltaOp {
	var deltas []IpsetDeltaOp
	for entry, comment := range newExpected {
		if _, ok := oldExpected[entry]; !ok {
			deltas = append(deltas, IpsetDeltaOp{
				Set:     setname,
				Entry:   entry,
				Comment: comment,
				Add:     true,
			})
		}
	}
	for entry, comment := range oldExpected {
		if _, ok := newExpected[entry]; !ok {
			deltas = append(deltas, IpsetDeltaOp{
				Set:     setname,
				Entry:   entry,
				Comment: comment,
				Add:     false,
			})
		}
	}
	return deltas
}

func cloneDB(db *DB) *DB {
	out := &DB{
		Users:  make(map[string]UserEntry, len(db.Users)),
		VMs:    make(map[string]string, len(db.VMs)),
		Access: make(map[string][]string, len(db.Access)),
	}
	for name, user := range db.Users {
		out.Users[name] = user
	}
	for name, ip := range db.VMs {
		out.VMs[name] = ip
	}
	for name, entries := range db.Access {
		out.Access[name] = append([]string(nil), entries...)
	}
	return out
}

func normalizeDBAccess(db *DB) {
	for user, entries := range db.Access {
		if len(entries) == 0 {
			delete(db.Access, user)
			continue
		}
		set := map[string]bool{}
		for _, entry := range entries {
			set[entry] = true
		}
		normalized := normalizeAccessSet(set)
		if len(normalized) == 0 {
			delete(db.Access, user)
		} else {
			db.Access[user] = normalized
		}
	}
}

func normalizeAccessSet(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	entries := make([]string, 0, len(set))
	for entry := range set {
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	return entries
}
