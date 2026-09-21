package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var writeClientConfig = writeClientConfigNoOverwrite
var writeRecreatedClientConfig = writeClientConfigOverwrite

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

func writeClientConfigOverwrite(path, content string) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary config for %s: %w", path, err)
	}
	tmpName := f.Name()
	defer os.Remove(tmpName)

	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return fmt.Errorf("chmod temporary config for %s: %w", path, err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
