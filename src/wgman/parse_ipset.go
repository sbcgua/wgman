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

// ParsedIPSet holds the result of parsing "ipset list <setname> -o save" output.
type ParsedIPSet struct {
	SetName string
	SetType string
	Entries []IPSetEntry
}

// ParseIPSet parses "ipset list <setname> -o save" output.
// The create header line must appear exactly once; all add lines must
// reference the same set name declared in the create line.
func ParseIPSet(output string) (*ParsedIPSet, error) {
	result := &ParsedIPSet{}
	for i, line := range splitLines(output) {
		switch {
		case strings.HasPrefix(line, "create "):
			if result.SetName != "" {
				return nil, fmt.Errorf("ipset line %d: duplicate create line", i+1)
			}
			name, setType, err := parseIPSetCreateLine(line)
			if err != nil {
				return nil, fmt.Errorf("ipset line %d: %w", i+1, err)
			}
			result.SetName = name
			result.SetType = setType
		case strings.HasPrefix(line, "add "):
			entry, addSetName, err := parseIPSetAddLine(line)
			if err != nil {
				return nil, fmt.Errorf("ipset line %d: %w", i+1, err)
			}
			if result.SetName == "" {
				return nil, fmt.Errorf("ipset line %d: add line before create line", i+1)
			}
			if addSetName != result.SetName {
				return nil, fmt.Errorf("ipset line %d: add set name %q does not match create set name %q",
					i+1, addSetName, result.SetName)
			}
			result.Entries = append(result.Entries, *entry)
		default:
			return nil, fmt.Errorf("ipset line %d: unexpected line %q", i+1, line)
		}
	}
	return result, nil
}

// parseIPSetCreateLine parses: create <setname> <settype> [options...]
// Returns the set name and set type.
func parseIPSetCreateLine(line string) (setName, setType string, err error) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("create line too short: %q", line)
	}
	return parts[1], parts[2], nil
}

// parseIPSetAddLine parses: add <setname> <entry> [comment "<comment>"]
// Returns the entry, the set name from the add line, and any parse error.
// The comment value may contain spaces and is stored without surrounding quotes.
func parseIPSetAddLine(line string) (*IPSetEntry, string, error) {
	// Split into at most 4 parts: "add", "<setname>", "<entry>", "<rest>"
	parts := strings.SplitN(line, " ", 4)
	if len(parts) < 3 {
		return nil, "", fmt.Errorf("add line too short: %q", line)
	}

	setName := parts[1]
	entry := &IPSetEntry{Entry: parts[2]}

	if len(parts) < 4 || strings.TrimSpace(parts[3]) == "" {
		return entry, setName, nil
	}

	rest := strings.TrimSpace(parts[3])
	if !strings.HasPrefix(rest, "comment ") {
		return nil, "", fmt.Errorf("unexpected trailing content %q in: %q", rest, line)
	}

	commentVal := strings.TrimPrefix(rest, "comment ")
	commentVal = strings.TrimSpace(commentVal)
	if strings.HasPrefix(commentVal, `"`) && strings.HasSuffix(commentVal, `"`) && len(commentVal) >= 2 {
		commentVal = commentVal[1 : len(commentVal)-1]
	}
	entry.Comment = commentVal
	return entry, setName, nil
}
