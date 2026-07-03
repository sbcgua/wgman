# Review 1: Phases 0-4

Review scope: current Go implementation after phases 0-4, checked against [docs/SPEC.md](docs/SPEC.md), [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md), and [PROGRESS.md](PROGRESS.md).

## Findings

### High: ipset parsing is too permissive for safe deploy planning

References: [parse_ipset.go](parse_ipset.go:16), [parse_ipset.go](parse_ipset.go:37), [check.go](check.go:105), [check.go](check.go:120)

`ParseIPSetEntries` currently skips `create` lines and stores only raw `add` entries. It does not validate the live set name, set type, family, or entry shape. That means a malformed managed entry, a wrong set type, or an unexpected `add` line can be interpreted as ordinary drift and turned into a delete delta.

This matters before phase 5 because `deploy` will trust the `CheckResult.Deltas`. The spec's unit-test strategy says malformed managed entries should be rejected, and the check command is expected to verify the configured all/matrix sets. Before implementing `deploy`, make ipset parsing/checking return hard errors for:

- all-access set not being the expected single-net style set;
- matrix set not being the expected net,net style set;
- `add` lines for the wrong set name;
- all-access entries that are not valid IPv4/net values;
- matrix entries that are not exactly two valid IPv4/net values.

### Medium: command-level code is not injectable enough for phase 5+

References: [cli.go](cli.go:95), [cmd_list_show.go](cmd_list_show.go:13), [cmd_list_show.go](cmd_list_show.go:92)

The command handlers instantiate `&RealSystem{}` directly and write directly to `os.Stdout`/`os.Stderr`. Current tests cover inner helpers like `runList` and `runShow`, but not the real command wiring. That is manageable for read-only commands, but it will become awkward for `deploy`, `mod`, `create`, and `remove`, where tests need fake system calls, fake stdin confirmation, dry-run behavior, and captured output.

Before phase 5, introduce a small app/deps boundary, for example carrying `SystemAdapter`, stdin, stdout, stderr, and a clock hook. Keep `main()` as the only place that constructs real OS dependencies. This will let command-level tests verify behavior without root and without real WireGuard/ipset tools.

### Medium: command argument validation is too loose

References: [cli.go](cli.go:95), [cmd_list_show.go](cmd_list_show.go:34), [cmd_list_show.go](cmd_list_show.go:92)

`cmdCheck` ignores positional args, `cmdShow` ignores positional args, and `cmdList` silently uses only the first positional arg. Examples like `wgman check alice`, `wgman show alice`, or `wgman list alice bob` should fail fast instead of ignoring operator input.

This is worth fixing before adding write commands, because silent extra arguments are risky for a root-operated administration tool. Recommended behavior:

- `check`: reject any positional args;
- `show`: reject any positional args;
- `list`: reject more than one positional arg;
- planned commands listed in help but not implemented yet should return "not implemented" rather than "unknown command", or be omitted from help until wired.

### Medium: IPv4 validation currently accepts IPv6 addresses

References: [validate.go](validate.go:15), [validate.go](validate.go:24)

`ValidateOffline` uses `net.ParseIP(...) == nil`, which accepts IPv6. The spec and ipset/WireGuard examples are IPv4-oriented, and the unit-test strategy calls out IPv4 specifically. User and VM validation should require `ip := net.ParseIP(value); ip != nil && ip.To4() != nil`.

This should be corrected before `create` and `mod`, because those phases will mutate `db.yaml` and compute live access state from these addresses.

### Low: real command errors lose useful stderr for read operations

References: [system_real.go](system_real.go:38), [system_real.go](system_real.go:46), [system_real.go](system_real.go:95)

`WGDump`, `IPSetList`, and `WGPubKey` use `Output()`, so stderr from failed commands is discarded. The write-side methods already use `CombinedOutput()`. Using `CombinedOutput()` consistently would make target-host failures easier to diagnose.

## Direction Assessment

The overall direction is good. The project has the right broad shape: typed YAML loading, pure validation/parsing functions, a system adapter, structured check results, deterministic tests, and read-only command helpers. This matches the spec and the vertical-slice plan.

The main concern is not the amount of implemented functionality; it is the boundary between checked live state and future deploy actions. Phase 5 should not start applying `CheckResult.Deltas` until ipset parsing validates the managed sets strongly enough, and command handlers are injectable enough to test root-level behavior without real system tools.

## Suggested Pre-Phase-5 Work

1. Tighten ipset parse/check behavior as described above.
2. Add command-level dependency injection for system adapter and I/O.
3. Add strict positional argument validation for implemented commands.
4. Change user/VM IP validation to require IPv4.
5. Add tests for the above before starting `deploy`.

I added these as explicit Review-1 pre-work hints in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

## Verification

I attempted to run:

```sh
go test ./...
```

but this workspace environment does not have `go` on `PATH`, so tests could not be executed here. [PROGRESS.md](PROGRESS.md) reports that tests passed in the previous agent's Go environment.
