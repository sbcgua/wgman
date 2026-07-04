# WGMAN — Handoff Document

**Date:** 2026-07-04  
**Status:** Review-3 pre-work for Phase 10 complete; Phase 10 polish itself has not been started. `go test ./...` and `go vet ./...` pass (using writable `GOCACHE=/tmp/go-build` in this sandbox); binary builds; `gofmt` clean.

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
- [parse_ipset.go](parse_ipset.go) — `ParseIPSet(output)` → `*ParsedIPSet{SetName, SetType, Entries}`.
  - Parses and records the `create` line's set name and type (previously skipped).
  - Returns hard error if a `create` line is missing before `add` lines.
  - Returns hard error if an `add` line names a different set than the `create` line.
  - Returns hard error on duplicate `create` lines.
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

### Review-1 HIGH Finding — Strict ipset parsing (complete)

Applied before Phase 6 per the Review-1 HIGH finding:

- **`ParseIPSet`** ([parse_ipset.go](parse_ipset.go)): replaces the old `ParseIPSetEntries`. Returns `*ParsedIPSet{SetName, SetType, Entries}`. Validates that add lines reference the same set name as the create line; rejects add-before-create and duplicate create lines. New parser error cases added to [parse_ipset_test.go](parse_ipset_test.go).
- **Set type and entry shape validation** ([check.go](check.go)): `validateAllAccessIPSet` hard-errors if set type ≠ `hash:ip` or any entry is not a plain IPv4 address. `validateMatrixIPSet` hard-errors if set type ≠ `hash:net,net` or any entry is not two comma-separated IPv4/net values. Validation failures skip `reconcileIPSet` so malformed entries cannot become spurious delete deltas. `isValidIPv4OrCIDR` helper added.
- **Test fixtures updated**: `buildCleanFakeSystem` and `TestCheck_ExtraIPSetEntry` in [check_test.go](check_test.go) updated to use `hash:ip` for the all-access set. Six new check tests added: `AllAccessWrongSetType`, `MatrixWrongSetType`, `AllAccessInvalidEntryShape`, `MatrixInvalidEntryShape`, `AddLineSetNameMismatch`, and the existing `MissingIPSet` hint test.

---

### Phase 6 — `deploy` (complete)

New files:
- [cmd_deploy.go](cmd_deploy.go) — `ApplyDeltas`, `printDeltas`, `cmdDeploy`.
  - `ApplyDeltas(deltas []IpsetDeltaOp, sys SystemAdapter) error` — iterates deltas and calls `sys.IPSetAdd` or `sys.IPSetDel`; returns first error.
  - `printDeltas(deltas, w)` — formats each delta as `add <set> <entry>  # <comment>` or `del <set> <entry>`.
  - `cmdDeploy` — requires root; loads config/db; calls `Check`; refuses on hard errors (`!result.Clean()`); reports "no changes needed" when `len(Deltas) == 0`; prints planned changes; honours `--dry-run` (print plan, no apply) and `--yes` (skip prompt); otherwise prompts `"Apply these changes? [y/N] "` reading from `app.Stdin`; applies via `ApplyDeltas`; reports applied count.
- [cmd_deploy_test.go](cmd_deploy_test.go) — 11 test cases:
  - rejects positional arguments (exit 2);
  - requires root;
  - refuses on hard errors;
  - clean state reports "no changes needed";
  - `--dry-run` prints plan but applies nothing;
  - `--yes` applies without prompt;
  - confirmation "y" applies;
  - confirmation "n" aborts cleanly;
  - applied add delta recorded in fake adapter;
  - applied delete delta recorded in fake adapter;
  - apply error propagated (exit 1).

CLI update ([cli.go](cli.go)):
- `deploy` added to command switch.

Test helper update ([testhelpers_test.go](testhelpers_test.go)):
- `ipsetAddErr error` and `ipsetDelErr error` fields added to `fakeSystem`.
- `IPSetAdd` and `IPSetDel` return these errors when set.

---

### Review-2 Pre-Work (complete)

Applied before Phase 7 per the Review-2 recommendations:

- **Configured ipset set-name validation** ([check.go](check.go)): `validateAllAccessIPSet` and `validateMatrixIPSet` now verify `ParsedIPSet.SetName` matches the configured set being checked. Regression tests added for wrong `create` set names in [check_test.go](check_test.go).
- **Duplicate access rejection** ([config.go](config.go)): `LoadDB` rejects duplicate entries inside each user's `access` list. Regression coverage added in [config_test.go](config_test.go).
- **Atomic deterministic db writer** ([config.go](config.go)): `SaveDBAtomic(dir, db)` writes a temp file in the config directory, chmods it to `0600`, and renames it over `db.yaml`. YAML output is deterministic: users, VMs, and access owners are sorted; access lists are written in caller-provided normalized order; `"*"` is explicitly quoted. Tests cover deterministic output and re-loading the written DB.

### Phase 7 — `mod` (complete)

New files:
- [cmd_mod.go](cmd_mod.go) — `parseModExpression`, `cmdMod`, `planModAccess`, access normalization helpers, and expected-ipset diffing.
  - Implements `wgman mod <name> <+res1,-res2...>`.
  - Requires root.
  - Parses comma-separated add/remove operations; invalid names and duplicate resources in the expression are rejected.
  - Validates target user and VM/resource names; supports `"*"` as the all-access resource while preserving the existing rule that `"*"` must be the sole access entry.
  - Calls `Check` first and refuses both hard errors and pre-existing ipset drift.
  - Plans the updated `db.yaml` and system deltas before writing anything.
  - Writes `db.yaml` atomically with deterministic duplicate-free access ordering.
  - Applies corresponding `IpsetDeltaOp` values through `ApplyDeltas`.
  - Supports `--dry-run`; dry-run prints planned deltas but does not write `db.yaml` or apply system updates.
  - Removing absent access is treated as a no-op and reports `mod: no changes needed`.
- [cmd_mod_test.go](cmd_mod_test.go) — tests cover:
  - valid and invalid mod expression parsing;
  - add/remove planning and expected add/delete deltas;
  - missing user and unknown VM refusal;
  - refusal on pre-existing drift;
  - dry-run no-write/no-apply behavior;
  - validation-failure no-write/no-apply behavior;
  - successful DB write plus ipset delta application;
  - removing absent access as a no-op.

CLI update:
- `mod` added to the command switch in [cli.go](cli.go). It was already present in help text.

Verification:
- `env GOCACHE=/tmp/go-build go test ./...`
- `env GOCACHE=/tmp/go-build go build -o /tmp/wgman .`

---

### Phase 8 — `create` (complete)

New files:
- [cmd_create.go](cmd_create.go) — `cmdCreate`, create argument parsing, IP allocation, client config rendering, no-overwrite config writing, and create planning.
  - Implements `wgman create <name> [ip] [res1,res2...]`.
  - Requires root.
  - Calls `Check` first and refuses both hard errors and pre-existing ipset drift.
  - Parses optional IP versus comma-separated access list:
    - `create carol` auto-allocates the next user IP;
    - `create carol 10.8.0.20` uses the supplied IPv4;
    - `create carol 10.8.0.20/32` normalizes to `10.8.0.20`;
    - `create carol sandbox,mailvm` treats the second arg as access and auto-allocates IP.
  - Auto-allocates from the interface subnet using the highest existing in-subnet user IP plus one.
  - Rejects duplicate/case-conflicting usernames, duplicate IPs, duplicate public keys, supplied IPs outside the interface subnet, unknown access resources, and invalid generated public keys.
  - Generates WireGuard private/public keys through `SystemAdapter`.
  - Reads `user.conf.template` from the config directory and writes `<user>.vpn.conf` in the current directory with mode `0600`.
  - Refuses to overwrite an existing generated client config.
  - Never writes the private key to `db.yaml`.
  - Writes the generated client config, applies live WireGuard/ipset state, then commits updated `db.yaml` atomically. Post-live failures trigger best-effort rollback so WireGuard hard drift is not left behind silently.
  - Adds the WireGuard peer through `WGSetPeer` and applies access ipset deltas through `ApplyDeltas`.
- [cmd_create_test.go](cmd_create_test.go) — tests cover:
  - optional argument parsing and `/32` IP normalization;
  - invalid create argument forms;
  - auto-IP allocation and tiny subnet rejection;
  - supplied IP/access planning;
  - duplicate and case-conflicting usernames;
  - unknown VM/resource rejection;
  - duplicate IP, duplicate public key, and outside-subnet rejection;
  - template substitution;
  - refusal on pre-existing drift;
  - refusal to overwrite an existing `<user>.vpn.conf`;
  - successful DB write, generated client config, expected `wg set`, expected ipset add, and private-key absence from `db.yaml`.

CLI update:
- `create` added to the command switch in [cli.go](cli.go). It was already present in help text.

Verification:
- `env GOCACHE=/tmp/go-build go test ./...`
- `env GOCACHE=/tmp/go-build go vet ./...`
- `env GOCACHE=/tmp/go-build go build -o /tmp/wgman .`

---

### Phase 9 — `remove` (complete)

New files:
- [cmd_remove.go](cmd_remove.go) — `cmdRemove`, `planRemoveUser`, and remove-plan output helpers.
  - Implements `wgman remove <name>`.
  - Requires root.
  - Calls `Check` first and refuses both hard errors and pre-existing ipset drift.
  - Validates the requested user exists and uses that user's public key from `db.yaml` for WireGuard peer removal.
  - Prints a deletion summary including username, IP, and current access.
  - Supports `--dry-run`; dry-run prints the plan and does not write `db.yaml` or call system adapters.
  - Prompts by default; `--yes` bypasses the prompt.
  - Confirmation rejection exits cleanly without writing `db.yaml` or applying system changes.
  - Applies corresponding ipset delete deltas through tracked delta application.
  - Removes the WireGuard peer through `WGDelPeer`.
  - Removes the user and access entries from `db.yaml` through `SaveDBAtomic` after live removal succeeds. Final DB commit failure triggers best-effort WireGuard/ipset rollback.
  - Does not remove generated client config files.
- [cmd_remove_test.go](cmd_remove_test.go) — tests cover:
  - remove planning and delete deltas;
  - missing user rejection;
  - refusal on pre-existing drift;
  - dry-run no-write/no-apply behavior;
  - confirmation rejection no-write/no-apply behavior;
  - `--yes` applying without prompt;
  - actual DB user/access removal;
  - expected matrix ipset delete operations;
  - expected all-access ipset delete operation for admin users;
  - expected WireGuard peer delete operation.

CLI update:
- `remove` added to the command switch in [cli.go](cli.go). It was already present in help text.

Verification:
- `env GOCACHE=/tmp/go-build go test ./...`
- `env GOCACHE=/tmp/go-build go vet ./...`
- `env GOCACHE=/tmp/go-build go build -o /tmp/wgman .`

---

### Review-3 Pre-Work For Phase 10 (complete)

Implemented only the pre-work block from Phase 10; Phase 10 polish/documentation review remains next.

- **Consistency-preserving create/remove policy**:
  - [cmd_create.go](cmd_create.go): `create` now writes the generated client config, applies `WGSetPeer`, applies access ipset deltas with tracking, then commits `db.yaml`. If `WGSetPeer`, ipset application, or final DB save fails after earlier side effects, it performs best-effort rollback: invert applied ipset deltas, remove the WireGuard peer, and remove the generated config.
  - [cmd_remove.go](cmd_remove.go): `remove` now applies ipset delete deltas with tracking, removes the WireGuard peer, then commits `db.yaml`. If `WGDelPeer` fails, it restores applied ipset deltas. If final DB save fails after live removal, it restores the WireGuard peer and applied ipset deltas.
  - [cmd_deploy.go](cmd_deploy.go): added `ApplyDeltasTracked` and `InvertDeltas` for rollback-aware command paths.
  - [cmd_mod.go](cmd_mod.go): delete deltas now preserve comments so inverse rollback can re-add matrix entries with their original comments.
- **Failure-injection tests**:
  - [testhelpers_test.go](testhelpers_test.go): `fakeSystem` can now inject `WGSetPeer` and `WGDelPeer` errors.
  - [cmd_create_test.go](cmd_create_test.go): added tests for client-config write failure and `WGSetPeer` failure; both assert no `db.yaml` change and no unrecoverable live work.
  - [cmd_remove_test.go](cmd_remove_test.go): added a `WGDelPeer` failure test that asserts `db.yaml` is unchanged and prior ipset deletes are rolled back.
- **Unsupported flags**:
  - [cmd_create.go](cmd_create.go): `create --dry-run` is rejected with exit code 2.
  - [cli_test.go](cli_test.go) and [cmd_create_test.go](cmd_create_test.go): regression coverage added for `create --dry-run`.
- **README status**:
  - [README.md](README.md): removed the stale “planned but not implemented yet” wording for `create`, `mod`, and `remove`.
- **DB write durability**:
  - [config.go](config.go): `SaveDBAtomic` now syncs the temporary file before rename and syncs the config directory after rename.

Verification:
- `env GOCACHE=/tmp/go-build go test ./...`
- `env GOCACHE=/tmp/go-build go vet ./...`
- `env GOCACHE=/tmp/go-build go build -o /tmp/wgman .`

---

## What Comes Next

Proceed from **Phase 10** in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

**Phase 10 — Polish, Packaging, And Documentation Alignment:**
- Review help text and command result messages.
- Ensure final command lines report success/failure consistently.
- Ensure exit codes are consistent.
- Align README commands with implementation.
- Add missing examples only if not already covered by [docs/SPEC.md](docs/SPEC.md).

Phase 11 is fully described in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

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
| `ParseIPSet` | returns `*ParsedIPSet{SetName, SetType, Entries}`; validates set name and add-line consistency |
| Set type enforcement | `hash:ip` required for all-access set; `hash:net,net` for matrix set — wrong type is a hard error |
| Entry shape enforcement | all-access: single IPv4; matrix: two IPv4/net values — bad shapes are hard errors, never drift |
| Parsed ipset name enforcement | `ParsedIPSet.SetName` must match the configured set name being checked |
| `db.yaml` writes | `SaveDBAtomic` writes a same-directory temp file, chmods `0600`, syncs it, renames into place, then syncs the config directory |
| Access list writes | normalized to sorted, duplicate-free lists before writing; empty access owners are omitted |
| `mod` drift policy | refuses any hard error or ipset drift before changing `db.yaml` |
| `mod` absent removal | removing access that is not present is a no-op |
| `create` IP parsing | supplied IPv4 accepted as plain IP or `/32`; stored as plain IPv4 |
| `create` IP allocation | picks highest existing in-subnet user IP plus one, inside the interface subnet |
| Generated client configs | written as `<user>.vpn.conf` in the current directory, mode `0600`, no overwrite |
| `create` private keys | generated private key is written only to the client config, never to `db.yaml` |
| `create` apply order | write generated config, then `WGSetPeer`, then ipset deltas, then commit `db.yaml`; failures trigger best-effort rollback |
| `remove` confirmation | prompts by default; `--yes` bypasses; `--dry-run` never prompts |
| `remove` apply order | ipset delete deltas, then `WGDelPeer`, then commit `db.yaml`; failures trigger best-effort rollback |
| Generated client config removal | `remove` does not delete `<user>.vpn.conf` files |

---

## Suggested Skills

- **`grilling`** — use if new design questions arise before implementing Phase 6 (e.g. confirmation UX, deploy output format).
- **`handoff`** — use again if work needs to be paused after further phases.
