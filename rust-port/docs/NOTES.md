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

## Tests

- Normal tests must not require root or real system tools.
- Use fake system adapters and temporary config directories.
- Use the Go tests as the parity checklist, especially for parser edge cases,
  validation policy, drift separation, rollback behavior, and command output.

## Linux Target

- The deployed tool targets Linux hosts with WireGuard, ipset, and firewall
  tooling installed separately.
- The port should not add package installation behavior.
- Avoid Windows-specific implementation assumptions. If Windows notes are ever
  added to docs, avoid personal absolute paths and prefer environment-variable
  examples such as `$USERPROFILE`.
