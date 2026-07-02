package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"

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
		for _, vm := range vms {
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
