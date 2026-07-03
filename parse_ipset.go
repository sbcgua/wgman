package main

import (
	"fmt"
	"strings"
)

// IPSetEntry is one parsed "add" line from "ipset list -o save" output.
type IPSetEntry struct {
	Entry   string // e.g. "10.8.0.10,192.168.122.100" or "10.8.0.5"
	Comment string // optional; empty string when not present
}

// ParseIPSetEntries parses the output of "ipset list <setname> -o save".
// "create" header lines are skipped; "add" lines are returned.
func ParseIPSetEntries(output string) ([]IPSetEntry, error) {
	var entries []IPSetEntry
	for i, line := range splitLines(output) {
		switch {
		case strings.HasPrefix(line, "create "):
			// skip header
		case strings.HasPrefix(line, "add "):
			entry, err := parseIPSetAddLine(line)
			if err != nil {
				return nil, fmt.Errorf("ipset line %d: %w", i+1, err)
			}
			entries = append(entries, *entry)
		default:
			return nil, fmt.Errorf("ipset line %d: unexpected line %q", i+1, line)
		}
	}
	return entries, nil
}

// parseIPSetAddLine parses: add <setname> <entry> [comment "<comment>"]
// The comment value may contain spaces and is stored without surrounding quotes.
func parseIPSetAddLine(line string) (*IPSetEntry, error) {
	// Split into at most 4 parts: "add", "<setname>", "<entry>", "<rest>"
	parts := strings.SplitN(line, " ", 4)
	if len(parts) < 3 {
		return nil, fmt.Errorf("add line too short: %q", line)
	}

	entry := &IPSetEntry{Entry: parts[2]}

	if len(parts) < 4 || strings.TrimSpace(parts[3]) == "" {
		return entry, nil
	}

	rest := strings.TrimSpace(parts[3])
	if !strings.HasPrefix(rest, "comment ") {
		return nil, fmt.Errorf("unexpected trailing content %q in: %q", rest, line)
	}

	commentVal := strings.TrimPrefix(rest, "comment ")
	commentVal = strings.TrimSpace(commentVal)
	if strings.HasPrefix(commentVal, `"`) && strings.HasSuffix(commentVal, `"`) && len(commentVal) >= 2 {
		commentVal = commentVal[1 : len(commentVal)-1]
	}
	entry.Comment = commentVal
	return entry, nil
}
