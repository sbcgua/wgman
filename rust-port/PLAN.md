# WGMAN Rust Port Plan

This plan ports the functional Go implementation to Rust under this
`rust-port/` directory. The original Go tree is the executable specification:
the Rust port should preserve command names, flags, default config paths,
output shape, validation policy, drift policy, rollback behavior, and test
coverage scope as closely as practical.

The final binary is named `wgman-rs`. Runtime defaults remain compatible with
the Go version, including `/etc/wireguard/wgman`.

## Non-Goals For Planning Phase

- Do not overwrite, move, or refactor the Go implementation.
- Do not require real `wg`, `ipset`, `iptables`, or root privileges for the
  normal test suite.
- Do not introduce a dual Go/Rust runtime harness as a required deliverable.
  Use the Go tests and fixtures as a parity checklist, and share fixture data
  where it is cheap.

## Architecture

Keep the same responsibility split as the Go implementation:

- `DB/config`: load, validate, normalize, clone, and write YAML state
  deterministically and atomically.
- `check`: read-only consistency and reconciliation engine. It validates the
  desired DB/config state against live WireGuard/ipset state and returns hard
  errors, drift, and concrete deltas.
- `deploy`: state application engine. It applies already-planned peer and
  ipset deltas in dependency-aware order and supports rollback.
- `commands`: thin CLI handlers that parse command-specific arguments, enforce
  root and cleanliness policies, print plans/results, and delegate the hard
  work to DB/check/deploy helpers.
- `system`: fakeable boundary around root checks, terminal detection,
  interface subnet discovery, WireGuard commands, ipset commands, and key
  generation.

Suggested module layout:

```text
rust-port/
  Cargo.toml
  Makefile
  docs/
    NOTES.md
  src/
    main.rs
    cli.rs
    model.rs
    config.rs
    db.rs
    check.rs
    deploy.rs
    system.rs
    system_real.rs
    parse_wg.rs
    parse_ipset.rs
    client_config.rs
    format.rs
    output.rs
    utils.rs
    commands/
      mod.rs
      check.rs
      list.rs
      show.rs
      init_ipsets.rs
      deploy.rs
      create.rs
      remove.rs
      modify.rs
  tests/
    fixtures/
```

This layout may be adjusted during implementation if Rust module ergonomics
call for it, but changes should preserve the responsibility boundaries above.

## Dependency Direction

Use common, maintained Rust crates where they reduce risk:

- CLI: `clap`, while preserving the Go help/output text where needed.
- YAML: `serde` plus a safe YAML crate. Reject unknown fields and arbitrary
  YAML tags/object forms.
- Errors: `thiserror` for typed lower-level errors; `anyhow` is acceptable at
  command/application boundaries.
- IP/CIDR: `ipnet` or standard library helpers, with explicit IPv4-only
  validation.
- Regex/name validation: `regex`.
- Atomic writes and tests: `tempfile` where useful.
- Terminal/color: a small explicit ANSI formatting module; avoid letting a
  color crate change output layout.

Every external command must be executed with argument arrays, not shell command
strings.

## Phase 1: Rust Skeleton And Project Conventions

Deliverables:

- Create `Cargo.toml`, `Cargo.lock`, `src/main.rs`, and the module skeleton.
- Add `Makefile` targets for at least:
  - `make build`
  - `make test`
  - `make fmt`
  - `make lint`
  - `make check`
- Configure the binary name as `wgman-rs`.
- Establish the `SystemAdapter` trait and a fake implementation for tests.
- Add `App`/dependency-injection shape equivalent to Go.
- Add initial `docs/NOTES.md` updates for Rust-specific conventions.

Acceptance checks:

- `cargo test` passes with smoke tests.
- `cargo fmt --check` passes.
- `cargo clippy --all-targets -- -D warnings` passes or the plan documents
  why a warning must be deferred.

Subagent boundary:

- This phase owns project scaffolding only. It should not implement full
  command behavior beyond help/no-arg smoke behavior.

## Phase 2: Data Model, YAML, And DB Persistence

Deliverables:

- Port `Config`, `ConfigSets`, `DB`, `UserEntry`, `ResourceEntry`,
  `ResourcePort`, and `ResourcePorts`.
- Load `config.yaml` and `db.yaml` with unknown-field rejection.
- Validate:
  - required config fields;
  - user/VM/resource names and case-only conflicts;
  - IPv4-only users and VMs;
  - duplicate user IPs and public keys;
  - resource VM references and unique normalized ports;
  - access references and `*` exclusivity;
  - VM/resource shared access-target namespace conflicts.
- Normalize resource ports to explicit `tcp:<port>`/`udp:<port>` pairs.
- Clone and normalize DB access deterministically.
- Save DB atomically with `0600` permissions, same-directory temp file,
  fsync-before-rename, rename, and directory fsync.
- Produce deterministic YAML output close to the Go implementation.

Acceptance checks:

- Rust tests cover the cases from `config_test.go` and `db_test.go`.
- Generated YAML omits empty comments and false `inactive`, preserves readable
  section ordering, and does not write private keys.

Subagent boundary:

- This phase does not call live system commands and does not implement command
  mutations.

## Phase 3: Parsers, Formatting, And Utility Functions

Deliverables:

- Port WireGuard dump parsing:
  - interface/server line;
  - peer rows;
  - `/32` normalization to plain IPv4;
  - `(none)` endpoint handling;
  - malformed-row errors.
- Port ipset save parsing:
  - `create` set name/type;
  - `add` entries with optional comments;
  - all-access, IP matrix, and port matrix entry shapes.
- Port utilities:
  - sorted keys;
  - endpoint host stripping;
  - IPv4/CIDR parsing and uint32 conversion;
  - ASCII case fold;
  - confirmation prompt helper.
- Port table/byte/handshake formatting and explicit ANSI color helpers.

Acceptance checks:

- Rust tests cover `parse_wg_test.go`, `parse_ipset_test.go`,
  `utils_test.go`, and `format_test.go` behavior.
- Colorized output must not change visible table alignment once ANSI escapes
  are stripped.

Subagent boundary:

- This phase may define parser/format structs used by later phases but should
  avoid command orchestration.

## Phase 4: Check Engine

Deliverables:

- Implement `CheckResult` with hard errors, drift, ipset deltas, peer deltas,
  and parsed WireGuard dump.
- Implement expected ipset state computation from DB:
  - all-access set for `*`;
  - full VM access through `ip_matrix`;
  - resource access through `port_matrix`;
  - inactive users excluded from expected live state.
- Validate interface subnet membership.
- Validate live WireGuard peers:
  - unknown peers are hard errors;
  - peer IP mismatch is a hard error;
  - missing active peers produce safe add deltas;
  - present inactive peers produce safe remove deltas.
- Validate live ipsets:
  - missing configured sets are hard errors with `init-ipsets` hint;
  - malformed managed entries are hard errors;
  - missing/extra valid entries are drift with concrete add/delete deltas.
- Sort errors, drift, and deltas for stable tests/output.

Acceptance checks:

- Rust tests cover `check_test.go`, including hard-error/drift separation,
  resource access, inactive users, missing sets, set type validation, malformed
  entries, and peer deltas.

Subagent boundary:

- This phase is read-only: no live state mutation beyond fake system reads.

## Phase 5: Deploy Engine And Rollback

Deliverables:

- Apply ipset deltas with tracked completed operations.
- Invert ipset deltas in reverse order.
- Diff old/new expected ipset state for command-local changes.
- Apply WireGuard peer deltas with tracked completed operations.
- Invert peer deltas in reverse order.
- Apply full state deltas in dependency order:
  - peer additions;
  - ipset deltas;
  - peer removals.
- Roll back completed work in the reverse dependency order.

Acceptance checks:

- Rust tests cover `deploy_test.go`, including partial failures, stable diff
  ordering, activation/deactivation ordering, and rollback ordering.

Subagent boundary:

- This phase exposes the engine used by commands but does not implement
  command argument parsing.

## Phase 6: Read-Only Commands

Deliverables:

- Implement CLI parsing and global flags:
  - `--config-dir`;
  - `--yes`;
  - `--dry-run`;
  - `--no-color`;
  - create/add-only `-c`.
- Implement help/no-args/unknown-command behavior.
- Implement `check`, `list`, and `show`.
- Enforce root checks in command handlers, not in core engines.
- Preserve Go output closely, including table headings, status summaries,
  inactive `~` suffixes, and color suppression on non-TTY or `--no-color`.

Acceptance checks:

- Rust tests cover `cli_test.go`, `cmd_check_test.go`,
  `cmd_list_test.go`, and `cmd_show_test.go` behavior relevant to read-only
  commands and global parsing.

Subagent boundary:

- This phase must not implement DB mutations or live mutation commands beyond
  rejecting unsupported paths cleanly.

## Phase 7: Init And Deploy Commands

Deliverables:

- Implement `init-ipsets`:
  - create configured `all`, `ip_matrix`, and `port_matrix` sets;
  - use set types `hash:ip`, `hash:net,net`, and `hash:ip,port,ip`;
  - enable comments where needed.
- Implement `deploy`:
  - run `check`;
  - refuse hard errors;
  - allow safe drift reconciliation;
  - report peer/ipset plans;
  - support confirmation, `--yes`, and `--dry-run`;
  - apply deltas through the deploy engine only.

Acceptance checks:

- Rust tests cover `cmd_init_ipsets_test.go` and `cmd_deploy_test.go`.
- No test requires root or real `ipset`/`wg`.

Subagent boundary:

- This phase owns live reconciliation from existing DB/config state. It does
  not create, remove, or modify DB users.

## Phase 8: Create, Remove, And Modify Commands

Deliverables:

- Implement `create`/`add`:
  - parse optional `-c`, optional IP, and optional access list;
  - auto-allocate next IPv4 in the interface subnet;
  - generate WireGuard keypair through `SystemAdapter`;
  - render `<user>.vpn.conf` from template;
  - write client config mode `0600` without overwrite;
  - apply live state and commit DB in the Go order;
  - rollback best-effort on failure.
- Implement `remove`:
  - refuse pre-existing hard errors or drift;
  - prompt unless `--yes`;
  - support `--dry-run`;
  - remove access and user from DB;
  - delete ipset entries and WireGuard peer through deploy helpers;
  - do not delete generated client configs.
- Implement `mod`:
  - parse `+target,-target` access operations;
  - parse `activate`/`deactivate`;
  - refuse pre-existing hard errors or drift;
  - support `--dry-run`;
  - preserve comments and access for inactive toggles;
  - apply live changes and rollback according to Go behavior.

Acceptance checks:

- Rust tests cover `client_config_test.go`, `cmd_create_test.go`,
  `cmd_remove_test.go`, and `cmd_mod_test.go`.
- Failure-injection tests prove no private key is written to DB and rollback
  paths leave fake live state consistent.

Subagent boundary:

- This phase owns all command-level DB mutations and client config generation.

## Phase 9: Parity Audit And Packaging

Deliverables:

- Audit Rust behavior against Go tests and `docs/SPEC.md`.
- Add or update fixtures for any behavior that was only implicitly covered in
  Go tests.
- Confirm `wgman-rs help` and command output are close to Go output.
- Confirm the normal suite runs without root, WireGuard, ipset, or iptables.
- Add a short README or usage note under `rust-port/` if needed.
- Keep `rust-port/docs/NOTES.md` current with final conventions and known
  intentional differences.

Acceptance checks:

- `make check` passes from `rust-port/`.
- The repository root Go implementation remains untouched except for planned
  documentation/artifact additions under `rust-port/`.

## Testing Matrix

The normal Rust test suite should cover:

- YAML config and DB validation/persistence.
- WireGuard and ipset parsers.
- Expected state and delta generation.
- Check hard errors versus drift.
- Deploy application order and rollback.
- CLI parsing and exit codes.
- Prompt, dry-run, and `--yes` behavior.
- Create/remove/mod failure-injection paths.
- Color/no-color and TTY/non-TTY output.
- Inactive users and port-limited resources.

Real Linux integration tests are optional and should be explicitly gated so
they do not run in the dev container by default.

## Handoff Rules For Implementation Agents

- Read `docs/SPEC.md`, `docs/NOTES.md`, `RUST-PORT.md`, this plan, and
  `rust-port/docs/NOTES.md` before implementation work.
- Stay inside `rust-port/` unless the user explicitly authorizes changes
  elsewhere.
- Keep command modules thin. Put durable behavior in DB/check/deploy/system
  modules.
- Update `rust-port/docs/NOTES.md` when a phase establishes or changes a
  convention. Add only durable implementation conventions there, no delivery tracking infromation.
- Put phases delivery tracking into `rust-port/PROGRESS.md` after each phase completion.
- Preserve Linux assumptions. Do not add Windows-specific behavior except in
  documentation, and use environment-variable examples rather than personal
  paths if Windows notes are unavoidable.
