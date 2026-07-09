package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultConfigDir = "/etc/wireguard/wgman"

// nameRe is the allowed pattern for user and VM names.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var saveDBAtomic = SaveDBAtomic

// LoadConfig reads and validates config.yaml from dir.
func LoadConfig(dir string) (*Config, error) {
	path := dir + "/config.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg.Interface == "" {
		return fmt.Errorf("config.yaml: interface is required")
	}
	if cfg.Sets.All == "" {
		return fmt.Errorf("config.yaml: sets.all is required")
	}
	if cfg.Sets.Matrix == "" {
		return fmt.Errorf("config.yaml: sets.matrix is required")
	}
	return nil
}

// LoadDB reads and validates db.yaml from dir.
func LoadDB(dir string) (*DB, error) {
	path := dir + "/db.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read db.yaml: %w", err)
	}

	var db DB
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&db); err != nil {
		return nil, fmt.Errorf("parse db.yaml: %w", err)
	}

	if errs := validateDB(&db); len(errs) > 0 {
		return nil, validationErrors(errs)
	}
	return &db, nil
}

type validationErrors []string

func (errs validationErrors) Error() string {
	return strings.Join(errs, "; ")
}

func validateDB(db *DB) []string {
	var errs []string

	if db.Users == nil {
		errs = append(errs, "db.yaml: users section is required")
	}
	if db.VMs == nil {
		errs = append(errs, "db.yaml: vms section is required")
	}

	// Validate user names and required fields.
	seenLower := map[string]string{} // lowercase name -> original name
	for name, u := range db.Users {
		if !nameRe.MatchString(name) {
			errs = append(errs, fmt.Sprintf("db.yaml: invalid user name %q", name))
		}
		if conflict, ok := seenLower[caseFold(name)]; ok {
			errs = append(errs, fmt.Sprintf("db.yaml: user name %q conflicts with %q (case)", name, conflict))
		}
		seenLower[caseFold(name)] = name
		if u.IP == "" {
			errs = append(errs, fmt.Sprintf("db.yaml: user %q: ip is required", name))
		} else if ip := net.ParseIP(u.IP); ip == nil || ip.To4() == nil {
			errs = append(errs, fmt.Sprintf("db.yaml: user %q has invalid ip %q", name, u.IP))
		}
		if u.Pub == "" {
			errs = append(errs, fmt.Sprintf("db.yaml: user %q: pub is required", name))
		}
	}

	// Validate VM names.
	seenVMLower := map[string]string{}
	for name, ipValue := range db.VMs {
		if !nameRe.MatchString(name) {
			errs = append(errs, fmt.Sprintf("db.yaml: invalid vm name %q", name))
		}
		if conflict, ok := seenVMLower[caseFold(name)]; ok {
			errs = append(errs, fmt.Sprintf("db.yaml: vm name %q conflicts with %q (case)", name, conflict))
		}
		seenVMLower[caseFold(name)] = name
		if parsed := net.ParseIP(ipValue); parsed == nil || parsed.To4() == nil {
			errs = append(errs, fmt.Sprintf("db.yaml: vm %q has invalid ip %q", name, ipValue))
		}
	}

	// Validate duplicate IPs across users.
	seenIPs := map[string]string{} // ip -> user name
	for name, u := range db.Users {
		if u.IP == "" {
			continue
		}
		if prev, ok := seenIPs[u.IP]; ok {
			errs = append(errs, fmt.Sprintf("db.yaml: duplicate ip %s for users %q and %q", u.IP, prev, name))
		}
		seenIPs[u.IP] = name
	}

	// Validate duplicate public keys across users.
	seenPubs := map[string]string{}
	for name, u := range db.Users {
		if u.Pub == "" {
			continue
		}
		if prev, ok := seenPubs[u.Pub]; ok {
			errs = append(errs, fmt.Sprintf("db.yaml: duplicate pub key for users %q and %q", prev, name))
		}
		seenPubs[u.Pub] = name
	}

	// Validate access entries.
	for user, vms := range db.Access {
		if _, ok := db.Users[user]; !ok {
			errs = append(errs, fmt.Sprintf("db.yaml: access references unknown user %q", user))
		}
		hasAdminStar := false
		seenAccess := map[string]bool{}
		for _, vm := range vms {
			if seenAccess[vm] {
				errs = append(errs, fmt.Sprintf("db.yaml: access for user %q contains duplicate entry %q", user, vm))
			}
			seenAccess[vm] = true
			if vm == "*" {
				hasAdminStar = true
				continue
			}
			if !nameRe.MatchString(vm) {
				errs = append(errs, fmt.Sprintf("db.yaml: access for user %q: invalid vm name %q", user, vm))
			}
			if _, ok := db.VMs[vm]; !ok {
				errs = append(errs, fmt.Sprintf("db.yaml: access for user %q references unknown vm %q", user, vm))
			}
		}
		if hasAdminStar && len(vms) > 1 {
			errs = append(errs, fmt.Sprintf("db.yaml: user %q: access contains \"*\" mixed with other VMs; \"*\" must be the sole entry", user))
		}
	}

	sort.Strings(errs)
	return errs
}

// cloneDB returns an independent copy suitable for planning DB changes.
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

// normalizeDBAccess sorts and deduplicates access lists and removes empty ones.
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

// normalizeAccessSet converts an access set into a sorted list.
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

// SaveDBAtomic writes db.yaml in a deterministic order using a temporary file
// in the same directory, then renames it into place.
func SaveDBAtomic(dir string, db *DB) error {
	data, err := marshalDBDeterministic(db)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".db.yaml.*")
	if err != nil {
		return fmt.Errorf("create temporary db.yaml: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary db.yaml: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temporary db.yaml: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary db.yaml: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary db.yaml: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, "db.yaml")); err != nil {
		return fmt.Errorf("replace db.yaml: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open config directory for sync: %w", err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync config directory: %w", err)
	}
	return nil
}

func marshalDBDeterministic(db *DB) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}

	root.Content = append(root.Content,
		scalarNode("users"), usersNode(db.Users),
		scalarNode("vms"), stringMapNode(db.VMs),
		scalarNode("access"), accessNode(db.Access),
	)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("encode db.yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close db.yaml encoder: %w", err)
	}
	return addBlankLinesBetweenDBSections(buf.Bytes()), nil
}

func addBlankLinesBetweenDBSections(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\nvms:\n"), []byte("\n\nvms:\n"))
	data = bytes.ReplaceAll(data, []byte("\naccess:\n"), []byte("\n\naccess:\n"))
	return data
}

func usersNode(users map[string]UserEntry) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range sortedKeys(users) {
		user := users[name]
		entry := &yaml.Node{Kind: yaml.MappingNode}
		entry.Content = append(entry.Content,
			scalarNode("ip"), scalarNode(user.IP),
			scalarNode("pub"), scalarNode(user.Pub),
		)
		if user.Comment != "" {
			entry.Content = append(entry.Content, scalarNode("comment"), scalarNode(user.Comment))
		}
		if user.Inactive {
			entry.Content = append(entry.Content, scalarNode("inactive"), boolNode(true))
		}
		node.Content = append(node.Content, scalarNode(name), entry)
	}
	return node
}

func stringMapNode(values map[string]string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		node.Content = append(node.Content, scalarNode(key), scalarNode(values[key]))
	}
	return node
}

func accessNode(access map[string][]string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	keys := make([]string, 0, len(access))
	for key, entries := range access {
		if len(entries) > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, entry := range access[key] {
			seq.Content = append(seq.Content, scalarNode(entry))
		}
		node.Content = append(node.Content, scalarNode(key), seq)
	}
	return node
}

func scalarNode(value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
	if value == "*" {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}

func boolNode(value bool) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool"}
	if value {
		node.Value = "true"
	} else {
		node.Value = "false"
	}
	return node
}
