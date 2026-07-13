package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const helpText = `wgman - wireguard and resource access manager

Usage: wgman <command> [flags] [args...]

Commands:
  check        Validate config/db and live WireGuard/ipset state
  list [user]  List users, resources, optional user access, or users for a resource with -r
  show         Show live WireGuard peers mapped to user names
  init-ipsets  Create, flush, or destroy the managed ipsets defined in config.yaml
  deploy       Reconcile ipset state from db.yaml (supports --dry-run, --yes)
  create       Create a new VPN user: create <name> [-c comment] [ip] [res1,res2...]
  add          Alias for create
  remove       Remove an existing VPN user: remove <name> (supports --dry-run, --yes)
  del          Alias for remove
  mod          Modify access or active state: mod <name> <+res1,-res2...|activate|deactivate> (supports --dry-run)
  usergroup    List or edit user group membership: usergroup <group> [+user,-user...] (supports --dry-run)
  help         Show this help message

Global flags:
  --config-dir <dir>  Config directory (default /etc/wireguard/wgman)
  --yes               Skip interactive confirmation prompts
  --dry-run           Show planned changes without applying them (deploy, remove, mod, usergroup only)
  --flush             Flush managed ipsets (init-ipsets only)
  --destroy           Destroy managed ipsets (init-ipsets only)
  --no-color          Disable colorized terminal output

List flags:
  -r <resource>       List users with access to a VM/resource, or "*" for all-access users

Run 'wgman help' or 'wgman -h' for this message.
`

// globalFlags holds parsed global flags.
type globalFlags struct {
	configDir         string
	yes               bool
	dryRun            bool
	noColor           bool
	createComment     string
	createCommentSet  bool
	listResource      string
	listResourceSet   bool
	initIPSetsFlush   bool
	initIPSetsDestroy bool
}

type parsedCommand struct {
	name string
	args []string
	gf   *globalFlags
	help bool
}

type trackedStringFlag struct {
	value *string
	set   *bool
	name  string
}

func (f trackedStringFlag) String() string {
	if f.value == nil {
		return ""
	}
	return *f.value
}

func (f trackedStringFlag) Set(value string) error {
	if *f.set {
		return fmt.Errorf("%s specified more than once", f.name)
	}
	*f.value = value
	*f.set = true
	return nil
}

func rejectUnsupportedDryRun(cmd string, gf *globalFlags, w io.Writer) bool {
	if !gf.dryRun {
		return false
	}
	fmt.Fprintf(w, "error: %s does not support --dry-run\n", cmd)
	return true
}

func newGlobalFlagSet(name string, stderr io.Writer, gf *globalFlags) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	configDirDefault := gf.configDir
	if configDirDefault == "" {
		configDirDefault = defaultConfigDir
	}
	fs.StringVar(&gf.configDir, "config-dir", configDirDefault, "config directory")
	fs.BoolVar(&gf.yes, "yes", gf.yes, "skip confirmation prompts")
	fs.BoolVar(&gf.dryRun, "dry-run", gf.dryRun, "show planned changes without applying")
	fs.BoolVar(&gf.initIPSetsFlush, "flush", gf.initIPSetsFlush, "flush managed ipsets")
	fs.BoolVar(&gf.initIPSetsDestroy, "destroy", gf.initIPSetsDestroy, "destroy managed ipsets")
	fs.BoolVar(&gf.noColor, "no-color", gf.noColor, "disable colorized terminal output")
	fs.Var(trackedStringFlag{value: &gf.createComment, set: &gf.createCommentSet, name: "-c"}, "c", "create/add user comment")
	return fs
}

func newCommandFlagSet(cmd string, stderr io.Writer, gf *globalFlags) *flag.FlagSet {
	fs := newGlobalFlagSet("wgman "+cmd, stderr, gf)
	if cmd == "list" {
		fs.Var(trackedStringFlag{value: &gf.listResource, set: &gf.listResourceSet, name: "-r"}, "r", "list users with access to VM/resource")
	}
	return fs
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

func (app *App) IsStdoutTTY() bool {
	return app.Sys.IsTerminal(app.Stdout)
}

// run is the entry point called from main; it builds a real App and delegates.
func run(args []string) int {
	return runApp(args, newRealApp())
}

func parseCommandArgs(args []string, stderr io.Writer) (*parsedCommand, error) {
	if len(args) == 0 {
		return &parsedCommand{help: true}, nil
	}

	gf := &globalFlags{}
	fs := newGlobalFlagSet("wgman", stderr, gf)

	// First pass: consume any flags that appear before the command name.
	// fs.Parse stops at the first non-flag argument (the command).
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return &parsedCommand{help: true}, nil
		}
		return nil, err
	}

	remaining := fs.Args() // [command, args...]
	if len(remaining) == 0 || remaining[0] == "help" || remaining[0] == "-h" || remaining[0] == "--help" {
		return &parsedCommand{help: true}, nil
	}

	cmd := remaining[0]

	// Second pass: consume any flags that appear after the command name.
	cmdFS := newCommandFlagSet(cmd, stderr, gf)
	if err := cmdFS.Parse(remaining[1:]); err != nil {
		if err == flag.ErrHelp {
			return &parsedCommand{help: true}, nil
		}
		return nil, err
	}

	cmdArgs := cmdFS.Args() // positional args for the command
	if gf.createCommentSet && cmd != "create" && cmd != "add" {
		err := fmt.Errorf("-c is only supported by create/add")
		fmt.Fprintln(stderr, "error:", err)
		return nil, err
	}
	if (gf.initIPSetsFlush || gf.initIPSetsDestroy) && cmd != "init-ipsets" {
		err := fmt.Errorf("--flush and --destroy are only supported by init-ipsets")
		fmt.Fprintln(stderr, "error:", err)
		return nil, err
	}
	if gf.createCommentSet {
		gf.createComment = strings.TrimSpace(gf.createComment)
		if gf.createComment == "" {
			err := fmt.Errorf("create comment must not be empty")
			fmt.Fprintln(stderr, "error:", err)
			return nil, err
		}
	}

	return &parsedCommand{name: cmd, args: cmdArgs, gf: gf}, nil
}

// runApp is the testable core of the CLI; app supplies all OS dependencies.
func runApp(args []string, app *App) int {
	parsed, err := parseCommandArgs(args, app.Stderr)
	if err != nil {
		return 2
	}
	if parsed.help {
		fmt.Fprint(app.Stdout, helpText)
		return 0
	}

	switch parsed.name {
	case "check":
		return cmdCheck(parsed.gf, parsed.args, app)
	case "list":
		return cmdList(parsed.gf, parsed.args, app)
	case "show":
		return cmdShow(parsed.gf, parsed.args, app)
	case "init-ipsets":
		return cmdInitIPSets(parsed.gf, parsed.args, app)
	case "deploy":
		return cmdDeploy(parsed.gf, parsed.args, app)
	case "mod":
		return cmdMod(parsed.gf, parsed.args, app)
	case "create", "add":
		return cmdCreate(parsed.gf, parsed.args, app)
	case "remove", "del":
		return cmdRemove(parsed.gf, parsed.args, app)
	case "usergroup":
		return cmdUserGroup(parsed.gf, parsed.args, app)
	default:
		fmt.Fprintf(app.Stderr, "wgman: unknown command %q\nRun 'wgman help' for usage.\n", parsed.name)
		return 2
	}
}
