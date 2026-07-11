package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// cmdList implements "wgman list [filter]".
func cmdList(gf *globalFlags, args []string, app *App) int {
	if len(args) > 1 {
		fmt.Fprintln(app.Stderr, "error: list takes at most one positional argument")
		return 2
	}
	if rejectUnsupportedDryRun("list", gf, app.Stderr) {
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

	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}
	color := false
	if !gf.noColor {
		color = app.IsStdoutTTY()
	}
	return runListWithColor(db, result, filter, app.Stdout, app.Stderr, color)
}

// runList is the testable core of "list".
// result must be OK() for output to be written; returns 1 on failure.
func runList(db *DB, result *CheckResult, filter string, stdout, stderr io.Writer) int {
	return runListWithColor(db, result, filter, stdout, stderr, false)
}

func runListWithColor(db *DB, result *CheckResult, filter string, stdout, stderr io.Writer, color bool) int {
	if !result.OK() {
		printCheckErrors(result, stderr)
		fmt.Fprintln(stderr, "list: FAILED")
		return 1
	}

	if filter != "" {
		return runListUser(db, filter, stdout, stderr, color)
	}

	fmt.Fprintln(stdout, "Users:")
	for _, name := range sortedKeys(db.Users) {
		u := db.Users[name]
		nameCell := showUserNameCell(name, u.Inactive, color)
		fmt.Fprintf(stdout, "  %s%s %-15s %s\n", nameCell.display, paddingFor(nameCell.plain, 20), u.IP, formatAccessSummary(db.Access[name], color))
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "VMs:")
	for _, name := range sortedKeys(db.VMs) {
		fmt.Fprintf(stdout, "  %-20s %s\n", name, db.VMs[name])
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "Resources:")
	if len(db.Resources) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	} else {
		for _, name := range sortedKeys(db.Resources) {
			resource := db.Resources[name]
			fmt.Fprintf(stdout, "  %-20s %s %s\n", name, resource.VM, formatResourcePorts(resource.Ports, color))
		}
	}

	// fmt.Fprintln(stdout, "list: OK")
	return 0
}

func paddingFor(s string, width int) string {
	if len(s) >= width {
		return ""
	}
	return strings.Repeat(" ", width-len(s))
}

func formatResourcePorts(ports ResourcePorts, color bool) string {
	if len(ports) == 0 {
		return "(none)"
	}
	byProtocol := make(map[string][]int)
	for _, port := range ports {
		byProtocol[port.Protocol] = append(byProtocol[port.Protocol], port.Port)
	}
	protocols := sortedKeys(byProtocol)
	groups := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		sort.Ints(byProtocol[protocol])
		groups = append(groups, formatResourcePortGroup(protocol, byProtocol[protocol], color))
	}
	return strings.Join(groups, " ")
}

func formatResourcePortGroup(protocol string, ports []int, color bool) string {
	prefix := protocol + ":"
	if !color {
		return prefix + joinPorts(ports)
	}
	switch protocol {
	case "tcp":
		prefix = colorPink(prefix)
	case "udp":
		prefix = colorCyan(prefix)
	}
	return prefix + joinPorts(ports)
}

func joinPorts(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, fmt.Sprint(port))
	}
	return strings.Join(parts, ",")
}

func formatAccessSummary(vms []string, color bool) string {
	if len(vms) == 0 {
		return "(" + colorAccessItem("none", color) + ")"
	}
	sorted := make([]string, len(vms))
	copy(sorted, vms)
	sort.Strings(sorted)
	for i, vm := range sorted {
		sorted[i] = colorAccessItem(vm, color)
	}
	return "(" + strings.Join(sorted, ",") + ")"
}

func colorAccessItem(item string, color bool) string {
	if !color {
		return item
	}
	switch item {
	case "none":
		return colorGrey(item)
	case "*":
		return colorRed(item)
	default:
		return item
	}
}

// runListUser prints the access list for a single named user.
func runListUser(db *DB, filter string, stdout, stderr io.Writer, color bool) int {
	u, ok := db.Users[filter]
	if !ok {
		fmt.Fprintf(stderr, "error: user %q not found\n", filter)
		fmt.Fprintln(stderr, "list: FAILED")
		return 1
	}

	vms := db.Access[filter]
	nameCell := showUserNameCell(filter, u.Inactive, color)
	fmt.Fprintf(stdout, "%s:\n", nameCell.display)
	if len(vms) == 0 {
		fmt.Fprintf(stdout, "  (%s)\n", colorAccessItem("none", color))
		fmt.Fprintln(stdout, "list: OK")
		return 0
	}

	sorted := make([]string, len(vms))
	copy(sorted, vms)
	sort.Strings(sorted)
	for _, vm := range sorted {
		fmt.Fprintf(stdout, "  %s\n", colorAccessItem(vm, color))
	}
	fmt.Fprintln(stdout, "list: OK")
	return 0
}
