# WGMAN — Handoff Document

**Date:** 2026-07-03  
**Status:** Phases 0–4 complete; `go test ./...` passes (0 failures, 1 expected skip); binary builds; `go vet` and `gofmt` clean.

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
  - `runList(db, result, filter, w)`: outputs alphabetically sorted Users + VMs sections; with filter shows named user's access list; returns 1 if `!result.OK()`.
  - `runShow(db, result, now, w)`: tabwriter-aligned table `NAME IP ENDPOINT RX TX LAST HANDSHAKE`; endpoint without port; bytes and handshake formatted.
  - Both refuse (exit 1) if check has hard errors or ipset drift.

CLI updates ([cli.go](cli.go)):
- `list` and `show` added to the command switch.
- Inline error printing in `cmdCheck` replaced by shared `printCheckErrors(result)` helper (reused by `runList` and `runShow`).

Model update ([model.go](model.go)):
- `CheckResult.WGDump *WGDumpResult` added — populated by `Check()` after the WG dump is parsed; `nil` if check aborted before that point.

Test files: [format_test.go](format_test.go), [cmd_list_show_test.go](cmd_list_show_test.go) — 20+ cases covering formatting helpers, list output, filter behavior, show column content, and refusal on bad check state.

---

## What Comes Next

Proceed from **Phase 5** in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

**Phase 5 — `deploy`:**
- `ApplyDeltas(cfg *Config, deltas []IpsetDeltaOp, sys SystemAdapter) error` — apply add/delete ipset operations.
- `cmdDeploy` — loads config/db, calls `Check`, refuses on hard errors; proceeds if only drift; reports planned deltas, prompts (unless `--yes`), applies.
- Support `--dry-run` (report without applying) and `--yes` (skip prompt).
- Test: deploy refuses hard errors; no drift → no changes; dry-run reports only; apply add/delete deltas; `--yes` bypasses prompt.

**Phase 6 — `mod`:**
- Parse `+vm,-vm` comma-separated expressions.
- Validate user and VM names.
- Refuse on pre-existing drift.
- Update `db.yaml` atomically.
- Apply corresponding deltas.

Subsequent phases (7–10) are fully described in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

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
| Root check | in CLI layer only (`cmdCheck`); `Check()` itself is root-agnostic |
| Check result order | `HardErrors`, `Drift`, `Deltas` all sorted before return |
| `CheckResult.WGDump` | populated after successful WG dump parse; nil on early exit |
| `list` filter | username → access list; blank → all users + all VMs |
| `show` output | tabwriter table: NAME IP ENDPOINT RX TX LAST HANDSHAKE |
| `sortedKeys` | generic helper in `cmd_list_show.go`; requires Go 1.18+ |
| Flag ordering | two-pass `flag.FlagSet` parse: flags allowed before or after command |

---

## Suggested Skills

- **`grilling`** — use if new design questions arise before implementing Phases 4–5 (e.g. `show` output format, `list` column layout).
- **`handoff`** — use again if work needs to be paused after further phases.
