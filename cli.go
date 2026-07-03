package main

import (
	"flag"
	"fmt"
	"os"
)

const helpText = `wgman - wireguard and resource access manager

Usage: wgman <command> [flags] [args...]

Commands:
  check   Validate config/db and live WireGuard/ipset state
  list    List users, resources, and optional access for a user
  show    Show live WireGuard peers mapped to user names
  deploy  Reconcile ipset state from db.yaml (supports --dry-run, --yes)
  create  Create a new VPN user
  remove  Remove an existing VPN user (supports --dry-run, --yes)
  mod     Modify user VM access (supports --dry-run)
  help    Show this help message

Global flags:
  --config-dir <dir>  Config directory (default /etc/wireguard/wgman)
  --yes               Skip interactive confirmation prompts
  --dry-run           Show planned changes without applying them

Run 'wgman help' or 'wgman -h' for this message.
`

// globalFlags holds parsed global flags.
type globalFlags struct {
	configDir string
	yes       bool
	dryRun    bool
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Print(helpText)
		return 0
	}

	// Build a single global FlagSet used for both passes.
	fs := flag.NewFlagSet("wgman", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	gf := &globalFlags{}
	fs.StringVar(&gf.configDir, "config-dir", defaultConfigDir, "config directory")
	fs.BoolVar(&gf.yes, "yes", false, "skip confirmation prompts")
	fs.BoolVar(&gf.dryRun, "dry-run", false, "show planned changes without applying")

	// First pass: consume any flags that appear before the command name.
	// fs.Parse stops at the first non-flag argument (the command).
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Print(helpText)
			return 0
		}
		return 2
	}

	remaining := fs.Args() // [command, args...]
	if len(remaining) == 0 || remaining[0] == "help" || remaining[0] == "-h" || remaining[0] == "--help" {
		fmt.Print(helpText)
		return 0
	}

	cmd := remaining[0]

	// Second pass: consume any flags that appear after the command name.
	if err := fs.Parse(remaining[1:]); err != nil {
		if err == flag.ErrHelp {
			fmt.Print(helpText)
			return 0
		}
		return 2
	}

	cmdArgs := fs.Args() // positional args for the command

	switch cmd {
	case "check":
		return cmdCheck(gf, cmdArgs)
	case "list":
		return cmdList(gf, cmdArgs)
	case "show":
		return cmdShow(gf, cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "wgman: unknown command %q\nRun 'wgman help' for usage.\n", cmd)
		return 2
	}
}

// cmdCheck implements "wgman check".
func cmdCheck(gf *globalFlags, _ []string) int {
	sys := &RealSystem{}
	if !sys.IsRoot() {
		fmt.Fprintln(os.Stderr, "error: wgman must be run as root")
		return 1
	}

	cfg, err := LoadConfig(gf.configDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	db, err := LoadDB(gf.configDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	result := Check(cfg, db, sys)
	printCheckErrors(result)
	if !result.OK() {
		fmt.Fprintln(os.Stderr, "check: FAILED")
		return 1
	}
	fmt.Println("check: OK")
	return 0
}

// printCheckErrors writes hard errors and ipset drift from result to stderr.
func printCheckErrors(result *CheckResult) {
	if len(result.HardErrors) > 0 {
		fmt.Fprintln(os.Stderr, "check: hard errors:")
		for _, e := range result.HardErrors {
			fmt.Fprintln(os.Stderr, "  -", e)
		}
	}
	if len(result.Drift) > 0 {
		fmt.Fprintln(os.Stderr, "check: ipset drift:")
		for _, d := range result.Drift {
			fmt.Fprintln(os.Stderr, "  -", d)
		}
	}
}
