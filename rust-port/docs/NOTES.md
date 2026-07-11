# Rust Port Implementation Notes

These notes capture durable conventions for the Rust port. Update this file
when implementation settles a convention or intentionally differs from the Go
version.

## Scope

- The Rust project lives under `rust-port/`.
- The original Go tree is the executable specification and should not be
  modified by Rust-port implementation work unless the user explicitly asks.
- The final binary is `wgman-rs`.
- Runtime defaults remain compatible with Go, including the default config
  directory `/etc/wireguard/wgman`.

## Architecture

- Keep complexity concentrated in three core areas:
  - DB/config loading, validation, normalization, and persistence;
  - `check`, the read-only consistency and reconciliation engine;
  - `deploy`, the live-state application and rollback engine.
- Command handlers should stay thin: parse arguments, enforce root/cleanliness
  policy, print output, and call the engines.
- All external system interaction goes through a fakeable system trait.
- Core engines must be testable without root, WireGuard, ipset, iptables, or a
  real Linux network interface.
- The CLI dependency-injection boundary is `App<S: SystemAdapter>`.
- `main.rs` should stay minimal: construct `RealSystemAdapter`, delegate to
  `cli`, and return the resulting exit code.
- Phase-owned command handlers live under `src/commands/`, while durable
  behavior belongs in the corresponding engine modules.

## Compatibility

- Preserve command names, flags, default paths, prompts, exit-code policy, and
  output shape close to the Go implementation.
- Byte-for-byte output parity is useful but not mandatory where semantic
  behavior and operator readability are preserved.
- `wgman-rs` should remain a single executable from the operator perspective.
  A normal dynamically linked Linux Rust binary is acceptable unless later
  packaging work chooses a static target.

## Dependencies

- Common, maintained Rust crates are acceptable when they reduce risk.
- Prefer explicit user-visible output formatting over crate defaults when crate
  defaults differ from the Go CLI behavior.
- Execute external commands with argument arrays, never shell command strings.
- Phase 1 intentionally had no external crate dependencies. Phase 2 added
  `serde`, `serde_yaml`, and `tempfile` for typed YAML loading and same-dir
  atomic DB writes.

## Parsers, Utilities, And Formatting

- `parse_wg` parses `wg show <interface> dump` output into the server public
  key and peer rows. Peer allowed IPs normalize IPv4 `/32` values to plain
  addresses, keep non-`/32` CIDRs unchanged, preserve `(none)` endpoints, and
  reject multiple allowed IPs for one managed peer.
- `parse_ipset` parses `ipset list <set> -o save` output, including the
  `create` set name/type and `add` entries with optional comments containing
  spaces. Add lines before a create line, duplicate create lines, unexpected
  trailing content, and create/add set-name mismatches are parse errors.
- Phase 3 parser functions return `Result<_, String>` to match the simple
  error style already used by config and DB validation. Later phases may wrap
  these strings in richer command or check errors if needed.
- Utility helpers now cover sorted string keys, endpoint host stripping,
  non-empty line splitting with CR trimming, confirmation prompts, IPv4/CIDR
  validation, IPv4 CIDR parsing, IPv4/u32 conversion, and ASCII case folding.
- Formatting helpers own explicit ANSI escape sequences. `write_table` sizes
  columns from each cell's plain text and writes each cell's display text, so
  color escapes do not affect visible alignment.
- Byte formatting uses Go-compatible binary units and two decimal places for
  `Kb`, `Mb`, and `Gb`. Handshake formatting clamps future timestamps to `0s`,
  renders timestamp `0` as `never`, and colorizes the same day/minute
  components as the Go implementation.

## Check And Drift

- `check::check` is the read-only reconciliation engine. It validates config
  and DB state first, then reads the interface subnet, WireGuard dump, and
  configured ipsets through `SystemAdapter`.
- `SystemAdapter` currently includes read-only live-state methods:
  `interface_subnet`, `wg_dump`, and `ipset_list`. Mutation methods remain for
  later phases.
- `CheckResult` separates `hard_errors`, `drift`, `ipset_deltas`,
  `peer_deltas`, and the parsed `wg_dump`. `ok()` requires no hard errors,
  no ipset drift, and no peer drift. `clean()` only requires no hard errors.
- `compute_expected_ipsets` derives all managed ipset entries from `db.yaml`.
  Active `*` access maps to the all-access set, VM access maps to
  `ip_matrix`, resource ports map to `port_matrix`, and inactive users are
  excluded from all expected live WireGuard/ipset state.
- Expected ipset comments match Go:
  `<user> -> <vm>` for full VM access and
  `<user> -> <resource> <protocol>/<port>` for port-limited resources.
- Active DB users missing from live WireGuard produce `WGPeerDeltaAction::Add`.
  Inactive DB users present in live WireGuard produce
  `WGPeerDeltaAction::Remove`. Unknown live peers and allowed-IP mismatches
  are hard errors.
- If hard errors exist after WireGuard validation, ipset checks are skipped so
  unsafe or ambiguous peer state does not produce deployable ipset drift.
- Missing configured ipsets are hard errors and include the
  `wgman init-ipsets` hint. Wrong set names, wrong set types, parse failures,
  and malformed managed entries are hard errors, not delete drift.
- Valid missing or extra managed entries are reported as drift with concrete
  add/delete `IPSetDelta` values. Hard errors, drift, ipset deltas, and peer
  deltas are sorted before return for stable output and tests.

## Read-Only Commands

- Phase 6 implements hand-written CLI parsing to preserve Go behavior where
  global flags may appear before or immediately after the command name.
- No args, `help`, `-h`, `--help`, and command-local help flags print the
  static Go-compatible help text and exit `0`.
- Unknown commands and argument/usage errors exit `2`. `check`, `list`, and
  `show` reject `--dry-run`; mutation commands remain intentionally
  unsupported in this phase.
- `--config-dir`, `--yes`, `--dry-run`, `--no-color`, and create/add-only `-c`
  are parsed globally. Go-style single-dash long global flags and explicit
  boolean assignments such as `--dry-run=false` are accepted. `-c` on
  non-create/add commands and empty trimmed comments are usage errors.
- Root checks live in command handlers before config/db loading. Core engines
  remain root-agnostic and fakeable.
- `check` loads config/db, runs the check engine, prints hard errors,
  WireGuard drift, ipset drift, and a final `check: OK` or `check: FAILED`
  status. Planned delta details are reserved for deploy/remove/mod planning
  paths. Status color follows stdout/stderr TTY detection unless `--no-color`
  is set.
- `list` requires a clean check result, then prints users, VMs, resources, or
  an optional user access filter. It preserves inactive `~` suffixes and
  colorizes admin/none/VM/resource markers only on interactive stdout unless
  `--no-color` is set.
- `show` requires a clean check result and parsed WireGuard data, maps peers
  back to DB users, strips endpoint ports, formats byte counters and handshake
  ages, appends inactive `~` suffixes, and colorizes selected values only on
  interactive stdout unless `--no-color` is set.

## Deploy And Rollback

- `deploy.rs` applies already-planned live-state deltas and remains free of
  CLI orchestration, file loading, prompts, and DB mutation.
- Ipset deltas are applied in caller-provided order and tracked after each
  successful live operation. Application stops at the first error.
- Tracked apply errors carry both the completed operations and the error
  message so callers can perform best-effort rollback.
- Ipset rollback inverts completed deltas in reverse order while preserving
  comments on add entries so a deleted expected entry can be restored.
- `diff_expected_ipsets` compares old/new `ExpectedIPSets` and returns stable
  deltas sorted by set name, entry, and operation, with deletes before adds for
  the same set and entry.
- WireGuard peer deltas are applied in caller-provided order and tracked after
  each successful live operation. Add calls `wg set ... allowed-ips`; remove
  calls `wg set ... remove`.
- Peer rollback inverts completed deltas in reverse order and preserves
  `allowed_ip` on remove inversions so deleted peers can be restored.
- Full state application uses the Go dependency order: peer additions, ipset
  changes, then peer removals.
- Full rollback uses reverse dependency order: re-add completed peer removals,
  invert completed ipset changes, then remove completed peer additions.

## Config And DB

- `config.yaml` and `db.yaml` loading rejects unknown fields with serde
  `deny_unknown_fields`.
- YAML tags are rejected before typed deserialization; only plain YAML mappings,
  sequences, strings, booleans, numbers, and nulls are accepted by the loader.
- Missing required scalar fields deserialize to empty values so validation can
  produce Go-compatible required-field errors.
- User and VM names allow only ASCII letters, digits, `_`, and `-`.
- Resource and access-target names allow ASCII letters, digits, `_`, `@`, and
  `-`; `@` remains resource-only because VM names use the stricter pattern.
- Name lookup remains case-sensitive, but ASCII case-fold conflicts are
  rejected for users, VMs, and the shared VM/resource access-target namespace.
- Users and VMs must be plain IPv4 addresses. IPv6, CIDR, and non-IP values
  are rejected at DB validation time.
- Resource ports deserialize from either a scalar or a sequence. Unprefixed
  ports normalize to TCP, and normalized ports render as `tcp:<port>` or
  `udp:<port>`.
- DB validation rejects duplicate user IPs, duplicate public keys, duplicate
  access entries, unknown access owners/targets, resource references to unknown
  VMs, duplicate normalized resource ports, and `*` mixed with other access
  entries.
- DB clone and access-normalization helpers are separate from saving. The DB
  writer sorts map sections and access owners but preserves each access list's
  caller-provided order, matching the Go implementation.
- Deterministic DB writes omit empty comments and false `inactive`; private
  keys are not represented in the Rust DB model.
- `save_db_atomic` writes a same-directory temporary file, chmods it `0600` on
  Unix, fsyncs the file, renames it over `db.yaml`, and fsyncs the directory on
  Unix.

## Tests

- Normal tests must not require root or real system tools.
- Use fake system adapters and temporary config directories.
- Use the Go tests as the parity checklist, especially for parser edge cases,
  validation policy, drift separation, rollback behavior, and command output.
- Smoke tests cover only initial CLI scaffolding: no args, `help`, help flags,
  and unsupported-command usage errors.
- Phase 4 integration tests cover clean check state, resource access, inactive
  users, recoverable WireGuard peer drift, hard WireGuard errors, interface
  subnet validation, ipset drift/deltas, missing sets, malformed managed
  entries, and expected ipset computation.

## Linux Target

- The deployed tool targets Linux hosts with WireGuard, ipset, and firewall
  tooling installed separately.
- The port should not add package installation behavior.
- Avoid Windows-specific implementation assumptions. If Windows notes are ever
  added to docs, avoid personal absolute paths and prefer environment-variable
  examples such as `$USERPROFILE`.
