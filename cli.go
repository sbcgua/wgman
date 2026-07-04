package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

const helpText = `wgman - wireguard and resource access manager

Usage: wgman <command> [flags] [args...]

Commands:
  check        Validate config/db and live WireGuard/ipset state
  list         List users, resources, and optional access for a user
  show         Show live WireGuard peers mapped to user names
  init-ipsets  Create the managed ipsets defined in config.yaml
  deploy       Reconcile ipset state from db.yaml (supports --dry-run, --yes)
  create       Create a new VPN user
  remove       Remove an existing VPN user (supports --dry-run, --yes)
  mod          Modify user VM access (supports --dry-run)
  help         Show this help message

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

// App holds all injectable dependencies for command handlers.
// main() is the only place that constructs an App backed by real OS resources.
type App struct {
	Sys    SystemAdapter
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Now    func() time.Time
}

// newRealApp returns an App wired to real OS dependencies.
func newRealApp() *App {
	return &App{
		Sys:    &RealSystem{},
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Now:    time.Now,
	}
}

// run is the entry point called from main; it builds a real App and delegates.
func run(args []string) int {
	return runApp(args, newRealApp())
}

// runApp is the testable core of the CLI; app supplies all OS dependencies.
func runApp(args []string, app *App) int {
	if len(args) == 0 {
		fmt.Fprint(app.Stdout, helpText)
		return 0
	}

	// Build a single global FlagSet used for both passes.
	fs := flag.NewFlagSet("wgman", flag.ContinueOnError)
	fs.SetOutput(app.Stderr)
	gf := &globalFlags{}
	fs.StringVar(&gf.configDir, "config-dir", defaultConfigDir, "config directory")
	fs.BoolVar(&gf.yes, "yes", false, "skip confirmation prompts")
	fs.BoolVar(&gf.dryRun, "dry-run", false, "show planned changes without applying")

	// First pass: consume any flags that appear before the command name.
	// fs.Parse stops at the first non-flag argument (the command).
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Fprint(app.Stdout, helpText)
			return 0
		}
		return 2
	}

	remaining := fs.Args() // [command, args...]
	if len(remaining) == 0 || remaining[0] == "help" || remaining[0] == "-h" || remaining[0] == "--help" {
		fmt.Fprint(app.Stdout, helpText)
		return 0
	}

	cmd := remaining[0]

	// Second pass: consume any flags that appear after the command name.
	if err := fs.Parse(remaining[1:]); err != nil {
		if err == flag.ErrHelp {
			fmt.Fprint(app.Stdout, helpText)
			return 0
		}
		return 2
	}

	cmdArgs := fs.Args() // positional args for the command

	switch cmd {
	case "check":
		return cmdCheck(gf, cmdArgs, app)
	case "list":
		return cmdList(gf, cmdArgs, app)
	case "show":
		return cmdShow(gf, cmdArgs, app)
	case "init-ipsets":
		return cmdInitIPSets(gf, cmdArgs, app)
	case "deploy":
		return cmdDeploy(gf, cmdArgs, app)
	default:
		fmt.Fprintf(app.Stderr, "wgman: unknown command %q\nRun 'wgman help' for usage.\n", cmd)
		return 2
	}
}

// cmdCheck implements "wgman check".
func cmdCheck(gf *globalFlags, args []string, app *App) int {
	if len(args) > 0 {
		fmt.Fprintln(app.Stderr, "error: check takes no positional arguments")
		return 2
	}
	if !app.Sys.IsRoot() {
		fmt.Fprintln(app.Stderr, "error: wgman must be run as root")
		return 1
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
	printCheckErrors(result, app.Stderr)
	if !result.OK() {
		fmt.Fprintln(app.Stderr, "check: FAILED")
		return 1
	}
	fmt.Fprintln(app.Stdout, "check: OK")
	return 0
}

// printCheckErrors writes hard errors and ipset drift from result to w.
func printCheckErrors(result *CheckResult, w io.Writer) {
	if len(result.HardErrors) > 0 {
		fmt.Fprintln(w, "check: hard errors:")
		for _, e := range result.HardErrors {
			fmt.Fprintln(w, "  -", e)
		}
	}
	if len(result.Drift) > 0 {
		fmt.Fprintln(w, "check: ipset drift:")
		for _, d := range result.Drift {
			fmt.Fprintln(w, "  -", d)
		}
	}
}
