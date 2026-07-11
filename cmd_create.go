package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var writeClientConfig = writeClientConfigNoOverwrite

type createArgs struct {
	Name    string
	IP      string
	Access  []string
	Comment string
}

func parseCreateArgs(args []string, comment string, commentSet bool) (*createArgs, error) {
	var err error
	args, comment, commentSet, err = extractCreateCommentFlags(args, comment, commentSet)
	if err != nil {
		return nil, err
	}
	if commentSet {
		comment = strings.TrimSpace(comment)
		if comment == "" {
			return nil, fmt.Errorf("create comment must not be empty")
		}
	}
	if len(args) < 1 || len(args) > 3 {
		return nil, fmt.Errorf("create requires <name> [ip] [res1,res2...]")
	}
	if !nameRe.MatchString(args[0]) {
		return nil, fmt.Errorf("invalid user name %q", args[0])
	}

	parsed := &createArgs{Name: args[0], Comment: comment}
	if len(args) == 1 {
		return parsed, nil
	}

	if ip, ok := parseCreateIPArg(args[1]); ok {
		parsed.IP = ip
		if len(args) == 3 {
			access, err := parseCreateAccessList(args[2])
			if err != nil {
				return nil, err
			}
			parsed.Access = access
		}
		return parsed, nil
	}

	if len(args) == 3 {
		return nil, fmt.Errorf("second argument %q is not a valid IPv4 address or CIDR", args[1])
	}
	access, err := parseCreateAccessList(args[1])
	if err != nil {
		return nil, err
	}
	parsed.Access = access
	return parsed, nil
}

func extractCreateCommentFlags(args []string, initial string, initialSet bool) ([]string, string, bool, error) {
	out := make([]string, 0, len(args))
	comment := initial
	commentSet := initialSet
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c":
			if commentSet {
				return nil, "", false, fmt.Errorf("create comment specified more than once")
			}
			if i+1 >= len(args) {
				return nil, "", false, fmt.Errorf("-c requires a comment")
			}
			comment = args[i+1]
			commentSet = true
			i++
		case strings.HasPrefix(arg, "-c="):
			if commentSet {
				return nil, "", false, fmt.Errorf("create comment specified more than once")
			}
			comment = strings.TrimPrefix(arg, "-c=")
			commentSet = true
		default:
			out = append(out, arg)
		}
	}
	return out, comment, commentSet, nil
}

func parseCreateIPArg(s string) (string, bool) {
	if ip := net.ParseIP(s); ip != nil && ip.To4() != nil {
		return ip.String(), true
	}
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil || ip.To4() == nil {
		return "", false
	}
	ones, bits := ipNet.Mask.Size()
	if ones != 32 || bits != 32 {
		return "", false
	}
	return ip.String(), true
}

func parseCreateAccessList(s string) ([]string, error) {
	if s == "" {
		return nil, fmt.Errorf("access list must not be empty")
	}
	parts := strings.Split(s, ",")
	seen := map[string]bool{}
	access := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("empty access entry")
		}
		if part != "*" && !nameRe.MatchString(part) {
			return nil, fmt.Errorf("invalid resource name %q", part)
		}
		if seen[part] {
			return nil, fmt.Errorf("duplicate access entry %q", part)
		}
		seen[part] = true
		access = append(access, part)
	}
	sort.Strings(access)
	return access, nil
}

func cmdCreate(gf *globalFlags, args []string, app *App) int {
	if rejectUnsupportedDryRun("create", gf, app.Stderr) {
		return 2
	}
	if !app.Sys.IsRoot() {
		fmt.Fprintln(app.Stderr, "error: wgman must be run as root")
		return 1
	}

	parsed, err := parseCreateArgs(args, gf.createComment, gf.createCommentSet)
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
		fmt.Fprintln(app.Stderr, "create: FAILED (resolve hard errors or drift before creating users)")
		return 1
	}
	if result.WGDump == nil {
		fmt.Fprintln(app.Stderr, "create: no WireGuard data available")
		return 1
	}

	clientConfigPath := parsed.Name + ".vpn.conf"
	if _, err := os.Stat(clientConfigPath); err == nil {
		fmt.Fprintf(app.Stderr, "error: %s already exists\n", clientConfigPath)
		return 1
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(app.Stderr, "error: check %s: %v\n", clientConfigPath, err)
		return 1
	}

	subnet, err := app.Sys.InterfaceSubnet(cfg.Interface)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	privKey, err := app.Sys.WGGenKey()
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	pubKey, err := app.Sys.WGPubKey(privKey)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	updated, deltas, clientIP, err := planCreateUser(cfg, db, parsed, pubKey, subnet)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	templatePath := filepath.Join(gf.configDir, "user.conf.template")
	clientConfig, err := renderClientConfig(templatePath, privKey, clientIP, result.WGDump.ServerPubKey)
	if err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintf(app.Stdout, "create: planned user %s at %s\n", parsed.Name, clientIP)
	if len(deltas) > 0 {
		fmt.Fprintf(app.Stdout, "create: planned access changes (%d):\n", len(deltas))
		printDeltas(deltas, app.Stdout)
	}

	if err := writeClientConfig(clientConfigPath, clientConfig); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	if err := app.Sys.WGSetPeer(cfg.Interface, pubKey, clientIP); err != nil {
		rollbackErr := rollbackCreateLiveState(cfg.Interface, pubKey, clientConfigPath, nil, app.Sys)
		printApplyAndRollbackError(app.Stderr, err, rollbackErr)
		return 1
	}

	appliedDeltas, err := ApplyDeltasTracked(deltas, app.Sys)
	if err != nil {
		rollbackErr := rollbackCreateLiveState(cfg.Interface, pubKey, clientConfigPath, appliedDeltas, app.Sys)
		printApplyAndRollbackError(app.Stderr, err, rollbackErr)
		return 1
	}

	if err := saveDBAtomic(gf.configDir, updated); err != nil {
		rollbackErr := rollbackCreateLiveState(cfg.Interface, pubKey, clientConfigPath, appliedDeltas, app.Sys)
		printApplyAndRollbackError(app.Stderr, err, rollbackErr)
		return 1
	}

	fmt.Fprintf(app.Stdout, "create: created %s\n", parsed.Name)
	return 0
}

func rollbackCreateLiveState(iface, pubKey, clientConfigPath string, appliedDeltas []IpsetDeltaOp, sys SystemAdapter) error {
	var errs []string
	if err := ApplyDeltas(InvertDeltas(appliedDeltas), sys); err != nil {
		errs = append(errs, err.Error())
	}
	if err := sys.WGDelPeer(iface, pubKey); err != nil {
		errs = append(errs, err.Error())
	}
	if err := os.Remove(clientConfigPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func planCreateUser(cfg *Config, db *DB, args *createArgs, pubKey, subnet string) (*DB, []IpsetDeltaOp, string, error) {
	if !nameRe.MatchString(args.Name) {
		return nil, nil, "", fmt.Errorf("invalid user name %q", args.Name)
	}
	if _, ok := db.Users[args.Name]; ok {
		return nil, nil, "", fmt.Errorf("user %q already exists", args.Name)
	}
	for existing := range db.Users {
		if caseFold(existing) == caseFold(args.Name) {
			return nil, nil, "", fmt.Errorf("user name %q conflicts with %q (case)", args.Name, existing)
		}
	}
	if pubKey == "" {
		return nil, nil, "", fmt.Errorf("generated public key is empty")
	}
	for existing, user := range db.Users {
		if user.Pub == pubKey {
			return nil, nil, "", fmt.Errorf("generated public key conflicts with existing user %q", existing)
		}
	}

	clientIP := args.IP
	var err error
	if clientIP == "" {
		clientIP, err = allocateNextUserIP(subnet, db)
		if err != nil {
			return nil, nil, "", err
		}
	}
	if ip := net.ParseIP(clientIP); ip == nil || ip.To4() == nil {
		return nil, nil, "", fmt.Errorf("invalid client IP %q", clientIP)
	}
	_, ipNet, err := parseIPv4CIDR(subnet)
	if err != nil {
		return nil, nil, "", err
	}
	parsedClientIP := net.ParseIP(clientIP)
	if !ipNet.Contains(parsedClientIP) {
		return nil, nil, "", fmt.Errorf("client IP %s is outside interface subnet %s", clientIP, subnet)
	}
	for existing, user := range db.Users {
		if user.IP == clientIP {
			return nil, nil, "", fmt.Errorf("client IP %s conflicts with existing user %q", clientIP, existing)
		}
	}

	updated := cloneDB(db)
	updated.Users[args.Name] = UserEntry{IP: clientIP, Pub: pubKey, Comment: args.Comment}
	if len(args.Access) > 0 {
		updated.Access[args.Name] = append([]string(nil), args.Access...)
	}
	normalizeDBAccess(updated)
	if errs := validateDB(updated); len(errs) > 0 {
		return nil, nil, "", fmt.Errorf("updated db.yaml would be invalid: %s", strings.Join(errs, "; "))
	}

	oldAll, oldMatrix := computeExpectedIPSets(db)
	newAll, newMatrix := computeExpectedIPSets(updated)
	deltas := diffExpectedIPSets(cfg, oldAll, oldMatrix, newAll, newMatrix)
	return updated, deltas, clientIP, nil
}

func allocateNextUserIP(subnet string, db *DB) (string, error) {
	interfaceIP, ipNet, err := parseIPv4CIDR(subnet)
	if err != nil {
		return "", err
	}
	network := uint64(ipv4ToUint32(ipNet.IP))
	maskSize, _ := ipNet.Mask.Size()
	size := uint64(1) << uint64(32-maskSize)
	if size <= 2 {
		return "", fmt.Errorf("interface subnet %q has no usable client addresses", subnet)
	}
	first := network + 1
	last := network + size - 2

	maxUsed := uint64(ipv4ToUint32(interfaceIP))
	if maxUsed < first {
		maxUsed = first - 1
	}
	for _, user := range db.Users {
		ip := net.ParseIP(user.IP)
		if ip == nil || ip.To4() == nil || !ipNet.Contains(ip) {
			continue
		}
		n := uint64(ipv4ToUint32(ip))
		if n > maxUsed {
			maxUsed = n
		}
	}
	next := maxUsed + 1
	if next > last {
		return "", fmt.Errorf("no available IP in %s after existing users", subnet)
	}
	return uint32ToIPv4(uint32(next)), nil
}

func parseIPv4CIDR(subnet string) (net.IP, *net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(subnet)
	if err != nil || ip.To4() == nil {
		return nil, nil, fmt.Errorf("invalid interface subnet %q", subnet)
	}
	ones, bits := ipNet.Mask.Size()
	if ones < 0 || bits != 32 {
		return nil, nil, fmt.Errorf("interface subnet %q is not IPv4", subnet)
	}
	return ip, ipNet, nil
}

func renderClientConfig(templatePath, privateKey, clientIP, serverPublicKey string) (string, error) {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read user.conf.template: %w", err)
	}
	out := trimLeadingBlankLines(stripCommentLines(string(data)))
	replacements := map[string]string{
		"$CLIENT_PRIVATE_KEY": privateKey,
		"$CLIENT_VPN_IP":      clientIP,
		"$SERVER_PUBLIC_KEY":  serverPublicKey,
	}
	for placeholder, value := range replacements {
		out = strings.ReplaceAll(out, placeholder, value)
	}
	return out, nil
}

func stripCommentLines(s string) string {
	lines := strings.SplitAfter(s, "\n")
	var out strings.Builder
	for _, line := range lines {
		withoutNewline := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(strings.TrimSpace(withoutNewline), "#") {
			continue
		}
		out.WriteString(line)
	}
	return out.String()
}

func trimLeadingBlankLines(s string) string {
	lines := strings.SplitAfter(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.Join(lines, "")
}

func writeClientConfigNoOverwrite(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func ipv4ToUint32(ip net.IP) uint32 {
	ip4 := ip.To4()
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

func uint32ToIPv4(n uint32) string {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)).String()
}
