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
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(helpText)
		return 0
	}

	cmd := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	gf := &globalFlags{}
	fs.StringVar(&gf.configDir, "config-dir", defaultConfigDir, "config directory")
	fs.BoolVar(&gf.yes, "yes", false, "skip confirmation prompts")
	fs.BoolVar(&gf.dryRun, "dry-run", false, "show planned changes without applying")

	if err := fs.Parse(rest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	switch cmd {
	case "check":
		return cmdCheck(gf, fs.Args())
	default:
		fmt.Fprintf(os.Stderr, "wgman: unknown command %q\nRun 'wgman help' for usage.\n", cmd)
		return 2
	}
}

// cmdCheck implements "wgman check".
func cmdCheck(gf *globalFlags, _ []string) int {
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

	result := ValidateOffline(cfg, db)
	if len(result.HardErrors) > 0 {
		fmt.Fprintln(os.Stderr, "check: hard errors:")
		for _, e := range result.HardErrors {
			fmt.Fprintln(os.Stderr, "  -", e)
		}
		fmt.Fprintln(os.Stderr, "check: FAILED")
		return 1
	}

	fmt.Println("check: OK (offline validation passed; live WireGuard/ipset checks not yet implemented)")
	return 0
}
