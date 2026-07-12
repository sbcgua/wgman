package main

import (
	"fmt"
	"strings"
)

// cmdInitIPSets implements "wgman init-ipsets".
// By default it creates the managed ipsets defined in config.yaml using
// idempotent creation (-exist), so re-running the command on an
// already-configured host is safe. It can also flush and destroy the managed
// sets for WireGuard hook teardown or manual cleanup.
func cmdInitIPSets(gf *globalFlags, args []string, app *App) int {
	if len(args) > 0 {
		fmt.Fprintln(app.Stderr, "error: init-ipsets takes no positional arguments")
		return 2
	}
	if rejectUnsupportedDryRun("init-ipsets", gf, app.Stderr) {
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

	if gf.initIPSetsFlush || gf.initIPSetsDestroy {
		return runIPSetLifecycle(cfg, gf, app)
	}

	// All-access set: one IPv4 address per admin user.
	if err := app.Sys.IPSetCreate(cfg.Sets.All, "hash:ip", false); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(app.Stdout, "ipset %q: created (hash:ip)\n", cfg.Sets.All)

	// IP matrix set: pairs of source/destination IPv4 networks with comments.
	if err := app.Sys.IPSetCreate(cfg.Sets.IPMatrix, "hash:net,net", true); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(app.Stdout, "ipset %q: created (hash:net,net)\n", cfg.Sets.IPMatrix)

	// Port matrix set: source IPv4, destination port, destination IPv4 triples.
	if err := app.Sys.IPSetCreate(cfg.Sets.PortMatrix, "hash:ip,port,ip", true); err != nil {
		fmt.Fprintln(app.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(app.Stdout, "ipset %q: created (hash:ip,port,ip)\n", cfg.Sets.PortMatrix)

	fmt.Fprintln(app.Stdout, "init-ipsets: OK")
	return 0
}

func runIPSetLifecycle(cfg *Config, gf *globalFlags, app *App) int {
	setNames := managedIPSetNames(cfg)
	if gf.initIPSetsFlush {
		for _, setname := range setNames {
			if err := app.Sys.IPSetFlush(setname); err != nil {
				if isMissingIPSetError(err) {
					fmt.Fprintf(app.Stdout, "ipset %q: already absent, flush skipped\n", setname)
					continue
				}
				fmt.Fprintln(app.Stderr, "error:", err)
				return 1
			}
			fmt.Fprintf(app.Stdout, "ipset %q: flushed\n", setname)
		}
	}
	if gf.initIPSetsDestroy {
		for _, setname := range setNames {
			if err := app.Sys.IPSetDestroy(setname); err != nil {
				if isMissingIPSetError(err) {
					fmt.Fprintf(app.Stdout, "ipset %q: already absent, destroy skipped\n", setname)
					continue
				}
				fmt.Fprintf(app.Stderr, "error: %v\n", err)
				if isIPSetInUseError(err) {
					fmt.Fprintln(app.Stderr, "hint: run wgman-firewall-hook down before destroying managed ipsets")
				}
				return 1
			}
			fmt.Fprintf(app.Stdout, "ipset %q: destroyed\n", setname)
		}
	}
	fmt.Fprintln(app.Stdout, "init-ipsets: OK")
	return 0
}

func managedIPSetNames(cfg *Config) []string {
	return []string{cfg.Sets.All, cfg.Sets.IPMatrix, cfg.Sets.PortMatrix}
}

func isMissingIPSetError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "cannot be found") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no such file")
}

func isIPSetInUseError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "in use") ||
		strings.Contains(msg, "is in use") ||
		strings.Contains(msg, "kernel component")
}
