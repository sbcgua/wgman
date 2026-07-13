package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// cmdList implements "wgman list [filter]".
func cmdList(gf *globalFlags, args []string, app *App) int {
	if gf.listResourceSet && len(args) > 0 {
		fmt.Fprintln(app.Stderr, "error: list -r cannot be used with a user or user group filter")
		return 2
	}
	if gf.listResourceSet && gf.listResource == "" {
		fmt.Fprintln(app.Stderr, "error: list -r requires a resource or VM name")
		return 2
	}
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
	if gf.listResourceSet {
		return runListResourceWithColor(db, result, gf.listResource, app.Stdout, app.Stderr, color)
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
		fmt.Fprintf(stdout, "  %s%s %-15s %s\n", nameCell.display, paddingFor(nameCell.plain, 20), u.IP, formatAccessSummary(db, effectiveAccessForUser(db, name), color))
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, formatSectionHeader("User Groups", color))
	if len(db.UserGroups) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	} else {
		nameWidth := maxWidth(sortedKeys(db.UserGroups), 20)
		for _, name := range sortedKeys(db.UserGroups) {
			members := append([]string(nil), db.UserGroups[name]...)
			sort.Strings(members)
			memberText := strings.Join(members, ",")
			if memberText == "" {
				memberText = "(none)"
			}
			fmt.Fprintf(stdout, "  %-*s %s\n", nameWidth, name+":", memberText)
		}
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, formatSectionHeader("VMs", color))
	for _, name := range sortedKeys(db.VMs) {
		fmt.Fprintf(stdout, "  %-20s %s\n", name, db.VMs[name])
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, formatSectionHeader("Resources", color))
	if len(db.Resources) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	} else {
		names := sortedKeys(db.Resources)
		nameWidth := maxWidth(names, 20)
		vmWidth := 0
		for _, name := range names {
			if len(db.Resources[name].VM) > vmWidth {
				vmWidth = len(db.Resources[name].VM)
			}
		}
		for _, name := range names {
			resource := db.Resources[name]
			fmt.Fprintf(stdout, "  %-*s %-*s %s\n", nameWidth, name, vmWidth, resource.VM, formatResourcePorts(resource.Ports, false))
		}
	}

	// fmt.Fprintln(stdout, "list: OK")
	return 0
}

func runListResource(db *DB, result *CheckResult, resource string, stdout, stderr io.Writer) int {
	return runListResourceWithColor(db, result, resource, stdout, stderr, false)
}

func runListResourceWithColor(db *DB, result *CheckResult, resource string, stdout, stderr io.Writer, color bool) int {
	if !result.OK() {
		printCheckErrors(result, stderr)
		fmt.Fprintln(stderr, "list: FAILED")
		return 1
	}
	return runListTargetUsers(db, resource, stdout, stderr, color)
}

func paddingFor(s string, width int) string {
	if len(s) >= width {
		return ""
	}
	return strings.Repeat(" ", width-len(s))
}

func maxWidth(values []string, minimum int) int {
	width := minimum
	for _, value := range values {
		if len(value) > width {
			width = len(value)
		}
	}
	return width
}

func formatSectionHeader(label string, color bool) string {
	if !color {
		return label + ":"
	}
	switch label {
	case "VMs":
		return colorYellow(label + ":")
	case "Resources":
		return colorLightBlue(label + ":")
	default:
		return label + ":"
	}
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

func formatAccessSummary(db *DB, vms []string, color bool) string {
	if len(vms) == 0 {
		return "(" + colorAccessItem(db, "none", color) + ")"
	}
	sorted := make([]string, len(vms))
	copy(sorted, vms)
	sort.Strings(sorted)
	for i, vm := range sorted {
		sorted[i] = colorAccessItem(db, vm, color)
	}
	return "(" + strings.Join(sorted, ",") + ")"
}

func colorAccessItem(db *DB, item string, color bool) string {
	if !color {
		return item
	}
	switch item {
	case "none":
		return colorGrey(item)
	case "*":
		return colorRed(item)
	default:
		if _, ok := db.VMs[item]; ok {
			return colorYellow(item)
		}
		if _, ok := db.Resources[item]; ok {
			return colorLightBlue(item)
		}
		return item
	}
}

// runListUser prints the access list for a single named user.
func runListUser(db *DB, filter string, stdout, stderr io.Writer, color bool) int {
	u, ok := db.Users[filter]
	if ok {
		return runListRealUser(db, filter, u, stdout, color)
	}
	if _, ok := db.UserGroups[filter]; ok {
		return runListUserGroup(db, filter, stdout, color)
	}

	fmt.Fprintf(stderr, "error: user or user group %q not found\n", filter)
	fmt.Fprintln(stderr, "list: FAILED")
	return 1
}

func runListRealUser(db *DB, filter string, u UserEntry, stdout io.Writer, color bool) int {
	entries := effectiveAccessEntriesForUser(db, filter)
	nameCell := showUserNameCell(filter, u.Inactive, color)
	fmt.Fprintf(stdout, "%s:\n", nameCell.display)
	if len(entries) == 0 {
		fmt.Fprintf(stdout, "  (%s)\n", colorAccessItem(db, "none", color))
		// fmt.Fprintln(stdout, "list: OK")
		return 0
	}

	for _, entry := range entries {
		fmt.Fprintf(stdout, "  %s\n", formatAccessEntryWithGroups(db, entry, color))
	}
	// fmt.Fprintln(stdout, "list: OK")
	return 0
}

func runListUserGroup(db *DB, group string, stdout io.Writer, color bool) int {
	fmt.Fprintf(stdout, "%s:\n", group)
	vms := db.Access[group]
	if len(vms) == 0 {
		fmt.Fprintf(stdout, "  (%s)\n", colorAccessItem(db, "none", color))
		return 0
	}
	sorted := append([]string(nil), vms...)
	sort.Strings(sorted)
	for _, vm := range sorted {
		fmt.Fprintf(stdout, "  %s\n", colorAccessItem(db, vm, color))
	}
	return 0
}

func runListTargetUsers(db *DB, target string, stdout, stderr io.Writer, color bool) int {
	if target != "*" {
		if _, ok := db.VMs[target]; !ok {
			if _, ok := db.Resources[target]; !ok {
				fmt.Fprintf(stderr, "error: resource or VM %q not found\n", target)
				fmt.Fprintln(stderr, "list: FAILED")
				return 1
			}
		}
	}

	fmt.Fprintf(stdout, "%s:\n", colorAccessItem(db, target, color))
	entries := effectiveUsersForTarget(db, target)
	if len(entries) == 0 {
		fmt.Fprintf(stdout, "  (%s)\n", colorAccessItem(db, "none", color))
		return 0
	}
	for _, entry := range entries {
		fmt.Fprintf(stdout, "  %s\n", formatTargetUserEntry(db, entry, color))
	}
	return 0
}

func formatAccessEntryWithGroups(db *DB, entry EffectiveAccessEntry, color bool) string {
	out := colorAccessItem(db, entry.Target, color)
	if !entry.Direct && len(entry.Groups) > 0 {
		out += " (" + strings.Join(entry.Groups, ",") + ")"
	}
	return out
}

func formatTargetUserEntry(db *DB, entry EffectiveTargetUserEntry, color bool) string {
	user := db.Users[entry.User]
	nameCell := showUserNameCell(entry.User, user.Inactive, color)
	out := nameCell.display
	if !entry.Direct && len(entry.Groups) > 0 {
		out += " (" + strings.Join(entry.Groups, ",") + ")"
	}
	return out
}
