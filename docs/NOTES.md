# Implementation Notes

These notes capture project conventions and design decisions that are useful for
future changes. They intentionally omit implementation history.

## Configuration And Data

- Default config directory: `/etc/wireguard/wgman` (`defaultConfigDir`).
- Config files are `config.yaml`, `db.yaml`, and `user.conf.template`.
- YAML loading uses `yaml.Decoder.KnownFields(true)`, so unknown fields are
  rejected.
- `validateDB` is the single home for DB internal consistency checks. `LoadDB`
  converts its returned messages to an error, and `Check` reuses the same
  messages as hard errors before live system validation.
- User and VM names must match `^[A-Za-z0-9_-]+$`.
- Names are case-sensitive for lookup, but case-only conflicts are rejected
  with simple ASCII folding (`caseFold`).
- User and VM IPs must be IPv4. IPv6 and non-IP values are hard errors.
- User IPs in `db.yaml` are stored as plain IPv4 values, without `/32`.
- User records support optional `comment` metadata and optional
  `inactive: true`. Missing `inactive` means active. Deterministic writes omit
  empty comments and omit `inactive` when false.
- Duplicate user IPs, duplicate public keys, duplicate access entries, and
  access references to unknown users or VMs are rejected.
- Access entry `"*"` means all-access/admin and must be the only entry for
  that user.
- Private keys are never written to `db.yaml`.

## System Boundaries

- All external system interaction goes through `SystemAdapter`.
- Real command execution lives in `system_real.go`; tests should use
  `fakeSystem` from `testhelpers_test.go`.
- `exec.Command` is called with argument slices only.
- `InterfaceSubnet` uses Go's `net` package rather than shelling out.
- Root checks belong in command handlers. Core routines such as `Check` remain
  root-agnostic and fakeable.
- The CLI dependency-injection boundary is `App` in `cli.go`.

## Check And Drift Policy

- `check.go` is the read-only reconciliation engine: it validates desired state
  against live state and produces hard errors and planned deltas.
- `deploy.go` applies already-planned deltas in dependency-aware order. CLI
  concerns such as loading files, flags, output, prompts, and exit codes remain
  in `cmd_check.go` and `cmd_deploy.go`.
- `CheckResult` separates hard errors from ipset drift.
- `CheckResult.PeerDeltas` holds safe WireGuard peer operations for known DB
  users, such as adding missing active peers and removing live peers for
  inactive users.
- WireGuard peer deltas use an explicit action and always carry `AllowedIP`,
  including remove operations. Rollback inverts peer deltas without consulting
  DB state, so remove deltas must keep the last intended allowed IP.
- `deploy` may reconcile ipset drift and known-user peer drift when there are
  no hard errors.
- `create`, `remove`, `mod`, `list`, and `show` require a fully clean
  `CheckResult`.
- Configured ipsets are fully owned by `wgman`; unexpected entries in those
  sets are safe for `deploy` to delete.
- Inactive users are excluded from expected WireGuard peers and managed
  ipsets. Inactive users present in live WireGuard are removable drift, while
  active users missing from WireGuard are addable drift.
- Missing configured ipsets are hard errors and should suggest
  `wgman init-ipsets`.
- `CheckResult.HardErrors`, `Drift`, `IPSetDeltas`, and `PeerDeltas` are
  sorted before return for stable output and assertions.
- `CheckResult.WGDump` is populated after a successful WireGuard dump parse and
  can be nil if checking stops early.

## WireGuard And Ipset Parsing

- `wg show <interface> dump` peer allowed IPs are normalized from `/32` to
  plain IPv4.
- Multiple WireGuard allowed IPs for one managed peer are rejected.
- `ParseIPSet` returns set name, set type, and entries.
- Parsed ipset `create` names must match the configured set being checked.
- `add` lines must refer to the same set as the `create` line.
- All-access set type is `hash:ip`; entries must be one IPv4 value.
- Matrix set type is `hash:net,net`; entries must be two IPv4 or IPv4/CIDR
  values separated by a comma.
- Malformed managed ipset entries are hard errors, not drift to delete.
- Matrix comments use the format `<username> -> <vmname>`.

## Writes And Rollback

- `SaveDBAtomic` writes a same-directory temporary file, chmods it `0600`,
  syncs it, renames it over `db.yaml`, then syncs the config directory.
- Deterministic DB output sorts users, VMs, and access owners.
- Access lists are normalized to sorted, duplicate-free lists before writing;
  users with empty access are omitted from `access`.
- Generated client configs are written as `<user>.vpn.conf` in the current
  working directory, mode `0600`, and are never overwritten.
- Generated client configs strip comment-only lines from `user.conf.template`
  and then remove leading blank lines left by stripped template headers.
- `create` order: write client config, add WireGuard peer, apply ipset deltas,
  commit `db.yaml`. Failures trigger best-effort rollback.
- `remove` order: apply ipset delete deltas, remove WireGuard peer, commit
  `db.yaml`. Failures trigger best-effort rollback.
- `remove` does not delete existing generated client config files.
- Access-only `mod` writes `db.yaml` before applying ipset deltas; it refuses
  to run unless the pre-command state is clean. Inactive-user access edits can
  be DB-only changes because inactive users have no expected live ipset state.

## Command Behavior

- `help`, no args, `-h`, and `--help` print usage and exit 0.
- Exit code 2 is used for usage and argument errors.
- Exit code 1 is used for validation, system, or write failures.
- `--dry-run` is supported only by `deploy`, `remove`, and `mod`.
- `--yes` skips prompts for commands that prompt (`deploy`, `remove`).
- `--no-color` is a global output flag. Color decisions go through `App`'s
  stdout TTY boundary, which delegates terminal detection to `SystemAdapter`;
  tests and non-TTY output default to plain text.
- `create` and `mod` do not prompt after validation.
- `add` is a CLI alias for `create`.
- `create`/`add` support `-c <comment>` for storing user metadata. The value
  is trimmed, empty comments are rejected with exit code 2, and later comment
  edits are manual `db.yaml` edits.
- `deploy`, `remove`, and `mod` print planned deltas before applying or
  reporting dry-run results.
- Command-level planning helpers return plan structs rather than parallel
  result values. Plans use `IPSetDeltas` and `PeerDeltas` field names for live
  system changes.
- `deploy` applies WireGuard peer additions, then ipset deltas, then
  WireGuard peer removals. Activation repair therefore restores the peer before
  access entries, and inactive cleanup deletes managed ipset entries before
  removing the live peer.
- Command rollback uses the same state delta engine as forward application:
  completed peer removals are re-added, completed ipset changes are inverted,
  and completed peer additions are removed.
- `mod <user> activate` and `mod <user> deactivate` succeed as no-ops when the
  user is already in the requested state.
- Inactive toggles apply live changes before committing `db.yaml`; failed live
  changes or failed DB commits trigger best-effort live rollback.
- `list [user]` with no filter prints users with IP/access summaries and VMs.
  With a user filter it prints that user's access list.
- `list` colorizes access markers on interactive stdout: `none` uses grey and
  `*` uses red. `--no-color` suppresses this.
- `show` prints a tabwriter table: `NAME IP ENDPOINT RX TX LAST HANDSHAKE`.
- Endpoint formatting strips the port and preserves `(none)`.
- `show` appends `~` to inactive usernames in both color and no-color modes.
- `show` colorizes selected table values on interactive stdout. Traffic unit
  suffixes use dim cyan, zero-byte traffic (`0B`) uses grey, inactive
  usernames use grey, `never` uses grey, day/minute duration components use
  dim cyan, and hour/second components remain uncolored.

## Tests

- Unit tests should not require root, WireGuard, ipset, or firewall tools.
- Prefer table-driven tests and small fixtures for command output parsers.
- Use temporary config directories and `fakeSystem` for command tests.
- Failure-injection support exists for ipset add/delete/create, WireGuard
  peer add/delete, key generation, public key derivation, and DB save wrappers.
- Use `GOCACHE` under a writable temp directory in restricted sandboxes.
