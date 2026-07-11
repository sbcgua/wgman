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

## Linux Target

- The deployed tool targets Linux hosts with WireGuard, ipset, and firewall
  tooling installed separately.
- The port should not add package installation behavior.
- Avoid Windows-specific implementation assumptions. If Windows notes are ever
  added to docs, avoid personal absolute paths and prefer environment-variable
  examples such as `$USERPROFILE`.
