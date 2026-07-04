# WGMAN — Handoff Document

**Date:** 2026-07-04  
**Status:** Review-1 pre-work and Phase 5 complete; `go test ./...` passes (0 failures, 1 expected skip); binary builds; `go vet` and `gofmt` clean.

---

## What Has Been Done

### Phase 0 — Bootstrap (complete)

- Go module initialised (`module wgman`, Go 1.26.4).
- `gopkg.in/yaml.v3 v3.0.1` added as the only external dependency.
- Source files created:
  - [main.go](main.go) — `main()` delegates to `run(os.Args[1:])`.
  - [cli.go](cli.go) — command routing; `help`/`-h`/no-args prints usage; `check` dispatched; all other commands return "unknown command".
  - [model.go](model.go) — `Config`, `ConfigSets`, `DB`, `UserEntry`, `IpsetDeltaOp`, `CheckResult` types (with `OK()` and `Clean()` helpers).
  - [system.go](system.go) — `SystemAdapter` interface (all external command and OS calls).
  - [validate.go](validate.go) — `ValidateOffline(cfg, db)` pure function.
  - [config.go](config.go) — `LoadConfig`, `LoadDB` with `yaml.Decoder.KnownFields(true)`; all structural validation.
- Test scaffolding in [testhelpers_test.go](testhelpers_test.go): `testHelper`, `makeTempDir`, `writeFile`, `assertNoError`, `assertError`, `fakeSystem` (implements `SystemAdapter`).

### Phase 1 — Config and Database Validation (complete)

`LoadConfig` / `LoadDB` enforce:
- Required fields (`interface`, `sets.all`, `sets.matrix`; user `ip`/`pub`).
- `yaml.Decoder.KnownFields(true)` — unknown YAML fields are rejected.
- Name pattern `^[A-Za-z0-9_-]+$` for users and VMs.
- Case-conflicting names rejected (fold check).
- Duplicate IPs and duplicate public keys rejected.
- `access` entries validated against known users and VMs.

`ValidateOffline` additionally checks:
- User and VM IP values are valid IPv4 addresses.
- `"*"` in an access list must be the sole entry (no mixing with VM names).

Testdata fixture at [testdata/valid-offline/](testdata/valid-offline/) (config.yaml + db.yaml).

### Phase 2 — Parse Live System Output (complete)

New files:
- [parse_wg.go](parse_wg.go) — `ParseWGDump(output)` → `*WGDumpResult` / `[]WGPeer`.
  - Tab-separated `wg show <iface> dump` format (server line + peer lines).
  - `normalizeWGAllowedIP`: strips `/32` → plain IPv4; rejects multiple allowed-ips.
  - `splitLines` shared utility (used by ipset parser too).
- [parse_ipset.go](parse_ipset.go) — `ParseIPSetEntries(output)` → `[]IPSetEntry`.
  - Skips `create` header lines; parses `add <setname> <entry> [comment "<comment>"]`.
  - Handles quoted comments with spaces.
- [system_real.go](system_real.go) — `RealSystem` struct implementing `SystemAdapter`.
  - `IsRoot()`: `os.Getuid() == 0`.
  - `InterfaceSubnet(iface)`: Go `net.InterfaceByName` + `Addrs()`, no shell-out.
  - All `wg`/`ipset` commands via `exec.Command` with argument slices.

Test files: [parse_wg_test.go](parse_wg_test.go), [parse_ipset_test.go](parse_ipset_test.go).

### Phase 3 — Full `wgman check` (complete)

New file:
- [check.go](check.go) — `Check(cfg, db, sys)` → `*CheckResult`.
  - Calls `ValidateOffline` first; returns early on hard errors.
  - Validates all user IPs are within the interface subnet.
  - Validates WG peers match db users by pubkey; checks IP consistency.
  - Computes expected ipset state via `computeExpectedIPSets(db)`.
  - Detects drift (missing/extra entries) via `reconcileIPSet`; populates `Deltas`.
  - Output is sorted for deterministic assertions.

CLI updates ([cli.go](cli.go)):
- `cmdCheck` now uses `&RealSystem{}` and calls `Check(cfg, db, sys)`.
- Root check added at entry: non-root gets a clear error and exit code 1.
- Separate sections printed for hard errors vs ipset drift.

CLI test updates ([cli_test.go](cli_test.go)):
- `TestRun_CheckValid` replaced by `TestRun_CheckNotRoot` (verifies root rejection).
- `TestRun_CheckMissingDir` skips when not root (correct; only reached past root gate).

Test file: [check_test.go](check_test.go) — 13 table-driven cases covering the full check matrix.

### Phase 4 — Read-Only UX: `list` and `show` (complete)

New files:
- [format.go](format.go) — formatting helpers:
  - `formatBytes(n)` → `"2.07Mb"` style (B / Kb / Mb / Gb with 2 d.p.).
  - `formatHandshake(ts, now)` → `"2d23h48m40s"` age string; `"never"` for zero.
  - `endpointHost(endpoint)` → strips `:port`, passes `"(none)"` through.
- [cmd_list_show.go](cmd_list_show.go) — `cmdList`, `runList`, `runListUser`, `cmdShow`, `runShow`, `sortedKeys` generic helper.
  - `runList(db, result, filter, stdout, stderr)`: outputs alphabetically sorted Users + VMs sections; with filter shows named user's access list; returns 1 if `!result.OK()`.
  - `runShow(db, result, now, stdout, stderr)`: tabwriter-aligned table `NAME IP ENDPOINT RX TX LAST HANDSHAKE`; endpoint without port; bytes and handshake formatted.
  - Both refuse (exit 1) if check has hard errors or ipset drift.

CLI updates ([cli.go](cli.go)):
- `list` and `show` added to the command switch.
- Inline error printing in `cmdCheck` replaced by shared `printCheckErrors(result, w)` helper (reused by `runList` and `runShow`).

Model update ([model.go](model.go)):
- `CheckResult.WGDump *WGDumpResult` added — populated by `Check()` after the WG dump is parsed; `nil` if check aborted before that point.

Test files: [format_test.go](format_test.go), [cmd_list_show_test.go](cmd_list_show_test.go) — 20+ cases covering formatting helpers, list output, filter behavior, show column content, and refusal on bad check state.

### Review-1 Pre-Work (complete)

Applied before Phase 5 per reviewer recommendations:

- **IPv4 validation** ([validate.go](validate.go)): user and VM IPs now require `ip.To4() != nil`, so IPv6 addresses are rejected. Tests added: `TestValidateOffline_IPv6UserIP`, `TestValidateOffline_IPv6VMIP`.
- **Dependency injection** ([cli.go](cli.go)): new `App` struct carrying `Sys SystemAdapter`, `Stdin io.Reader`, `Stdout io.Writer`, `Stderr io.Writer`, `Now func() time.Time`. `run()` is a thin wrapper; `runApp(args, app)` is the testable core. `main()` is the only place that constructs a real `App`. `printCheckErrors` now takes an explicit `io.Writer`.
- **Argument validation**: `check` and `show` reject any positional arguments (exit 2); `list` rejects more than one positional argument (exit 2). Tests added in [cli_test.go](cli_test.go).
- **`CombinedOutput` for reads** ([system_real.go](system_real.go)): `WGDump`, `IPSetList`, and `WGPubKey` now use `CombinedOutput()` so stderr from failed commands is included in error messages.

### Phase 5 — `init-ipsets` (complete)

New files:
- [cmd_init_ipsets.go](cmd_init_ipsets.go) — `cmdInitIPSets(gf, args, app)`.
  - Requires root.
  - Loads `config.yaml` for set names.
  - Creates the all-access set as `hash:ip` (family inet, no comments).
  - Creates the matrix set as `hash:net,net` (family inet, with comments).
  - Uses `-exist` flag for idempotent creation: safe to re-run.
  - Rejects positional arguments.
- [cmd_init_ipsets_test.go](cmd_init_ipsets_test.go) — 9 test cases:
  - creates all-access set as `hash:ip`;
  - creates matrix set as `hash:net,net` with comments;
  - loads set names from `config.yaml`;
  - requires root;
  - rejects missing config;
  - rejects positional arguments;
  - idempotent via fake adapter;
  - propagates create error;
  - `check` missing-ipset hint includes `init-ipsets`.

Interface/adapter updates:
- `IPSetCreate(setname, setType string, withComment bool) error` added to `SystemAdapter` ([system.go](system.go)).
- `RealSystem.IPSetCreate` implemented in [system_real.go](system_real.go): runs `ipset create <setname> <setType> family inet -exist [comment]`.
- `fakeSystem.IPSetCreate` added to [testhelpers_test.go](testhelpers_test.go); records `"create:<setname>:<setType>:comment|nocomment"` in `appliedOps`.

CLI updates:
- `init-ipsets` added to help text and command switch in [cli.go](cli.go).
- Missing-ipset hard errors in [check.go](check.go) now hint: `"run 'wgman init-ipsets' to create managed sets"`.

---

## What Comes Next

Proceed from **Phase 6** in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

**Phase 6 — `deploy`:**

Pre-work (Review-1, before this phase):
- Tighten ipset parse/check so live set type and entry shape are validated before trusting deltas (see Review-1 High finding on `parse_ipset.go`).

Functionality:
- `ApplyDeltas(cfg *Config, deltas []IpsetDeltaOp, sys SystemAdapter) error` — apply add/delete ipset operations.
- `cmdDeploy` — loads config/db, calls `Check`, refuses on hard errors; proceeds if only drift; reports planned deltas, prompts (unless `--yes`), applies.
- Support `--dry-run` (report without applying) and `--yes` (skip prompt).

**Phase 7 — `mod`:**
- Parse `+vm,-vm` comma-separated expressions.
- Validate user and VM names.
- Refuse on pre-existing drift.
- Update `db.yaml` atomically.
- Apply corresponding deltas.

Subsequent phases (8–11) are fully described in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

---

## Key Conventions Established

| Topic | Decision |
|---|---|
| Config dir default | `/etc/wireguard/wgman` (`defaultConfigDir` in `config.go`) |
| Name regex | `^[A-Za-z0-9_-]+$` (`nameRe` in `config.go`) |
| Case folding | simple ASCII byte-shift (`caseFold` in `config.go`) |
| Unknown YAML fields | rejected via `KnownFields(true)` |
| `"*"` in access | admin/all-access; must be sole entry |
| IPs in db.yaml | plain IPv4, no `/32` suffix |
| WG allowed-ips | `/32` normalized to plain IPv4 by `normalizeWGAllowedIP` |
| Private keys | never written to `db.yaml` |
| System calls | only via `SystemAdapter`; `exec.Command` lives in `system_real.go` |
| InterfaceSubnet | uses Go `net` package, not shell commands |
| ipset comments | format `username -> vmname`; quoted in save output |
| Test fakes | `fakeSystem` in `testhelpers_test.go` |
| Testdata | `testdata/valid-offline/` |
| Root check | in CLI layer only; `Check()` is root-agnostic |
| Check result order | `HardErrors`, `Drift`, `Deltas` all sorted before return |
| `CheckResult.WGDump` | populated after successful WG dump parse; nil on early exit |
| `list` filter | username → access list; blank → all users + all VMs |
| `show` output | tabwriter table: NAME IP ENDPOINT RX TX LAST HANDSHAKE |
| `sortedKeys` | generic helper in `cmd_list_show.go`; requires Go 1.18+ |
| Flag ordering | two-pass `flag.FlagSet` parse: flags allowed before or after command |
| Dependency injection | `App` struct in `cli.go`; `run()` builds real App; `runApp(args, app)` is testable |
| `printCheckErrors` | takes explicit `io.Writer`; called with `app.Stderr` |
| IPv4 enforcement | `ip.To4() != nil` in `ValidateOffline`; IPv6 is a hard error |
| ipset creation | `-exist` flag; `hash:ip` for all-access, `hash:net,net comment` for matrix |
| Missing ipset hint | check errors include `"run 'wgman init-ipsets' to create managed sets"` |
| `IPSetCreate` ops | recorded as `"create:<name>:<type>:comment|nocomment"` in fakeSystem |

---

## Suggested Skills

- **`grilling`** — use if new design questions arise before implementing Phase 6 (e.g. confirmation UX, deploy output format).
- **`handoff`** — use again if work needs to be paused after further phases.
