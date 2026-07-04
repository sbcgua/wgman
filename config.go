package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

const defaultConfigDir = "/etc/wireguard/wgman"

// nameRe is the allowed pattern for user and VM names.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

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

	if err := validateDB(&db); err != nil {
		return nil, err
	}
	return &db, nil
}

func validateDB(db *DB) error {
	if db.Users == nil {
		return fmt.Errorf("db.yaml: users section is required")
	}
	if db.VMs == nil {
		return fmt.Errorf("db.yaml: vms section is required")
	}

	// Validate user names and required fields.
	seenLower := map[string]string{} // lowercase name -> original name
	for name, u := range db.Users {
		if !nameRe.MatchString(name) {
			return fmt.Errorf("db.yaml: invalid user name %q", name)
		}
		if conflict, ok := seenLower[caseFold(name)]; ok {
			return fmt.Errorf("db.yaml: user name %q conflicts with %q (case)", name, conflict)
		}
		seenLower[caseFold(name)] = name
		if u.IP == "" {
			return fmt.Errorf("db.yaml: user %q: ip is required", name)
		}
		if u.Pub == "" {
			return fmt.Errorf("db.yaml: user %q: pub is required", name)
		}
	}

	// Validate VM names.
	seenVMLower := map[string]string{}
	for name := range db.VMs {
		if !nameRe.MatchString(name) {
			return fmt.Errorf("db.yaml: invalid vm name %q", name)
		}
		if conflict, ok := seenVMLower[caseFold(name)]; ok {
			return fmt.Errorf("db.yaml: vm name %q conflicts with %q (case)", name, conflict)
		}
		seenVMLower[caseFold(name)] = name
	}

	// Validate duplicate IPs across users.
	seenIPs := map[string]string{} // ip -> user name
	for name, u := range db.Users {
		if prev, ok := seenIPs[u.IP]; ok {
			return fmt.Errorf("db.yaml: duplicate ip %s for users %q and %q", u.IP, prev, name)
		}
		seenIPs[u.IP] = name
	}

	// Validate duplicate public keys across users.
	seenPubs := map[string]string{}
	for name, u := range db.Users {
		if prev, ok := seenPubs[u.Pub]; ok {
			return fmt.Errorf("db.yaml: duplicate pub key for users %q and %q", prev, name)
		}
		seenPubs[u.Pub] = name
	}

	// Validate access entries.
	for user, vms := range db.Access {
		if _, ok := db.Users[user]; !ok {
			return fmt.Errorf("db.yaml: access references unknown user %q", user)
		}
		seenAccess := map[string]bool{}
		for _, vm := range vms {
			if seenAccess[vm] {
				return fmt.Errorf("db.yaml: access for user %q contains duplicate entry %q", user, vm)
			}
			seenAccess[vm] = true
			if vm == "*" {
				continue
			}
			if !nameRe.MatchString(vm) {
				return fmt.Errorf("db.yaml: access for user %q: invalid vm name %q", user, vm)
			}
			if _, ok := db.VMs[vm]; !ok {
				return fmt.Errorf("db.yaml: access for user %q references unknown vm %q", user, vm)
			}
		}
	}

	return nil
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
	return buf.Bytes(), nil
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

// caseFold lowercases a string for case-insensitive comparison.
func caseFold(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
