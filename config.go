package main

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultConfigDir = "/etc/wireguard/wgman"

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
