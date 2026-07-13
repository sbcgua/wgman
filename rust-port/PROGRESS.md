# WGMAN Rust Port Progress

This file is the resume log for the Rust port. It should be updated before and
after each implementation or review agent session so another agent can recover
the current state without relying on chat history.

## Operating Rules

- Work stays under `rust-port/` unless the user explicitly authorizes
  otherwise.
- Execute `rust-port/PLAN.md` phase by phase, in order.
- Use one implementation subagent per phase, with medium reasoning.
- Do not run implementation subagents in parallel.
- Spawn high-reasoning review agents when a phase is substantial enough to
  benefit from a separate review.
- Commit each phase result and each review result separately.
- Keep the Go implementation as the executable specification.

## Current Status

- Planning interview completed.
- Port plan created at `rust-port/PLAN.md`.
- Rust-specific notes created at `rust-port/docs/NOTES.md`.
- Phase 1 implementation completed: Rust Cargo skeleton, Makefile, module
  layout, `wgman-rs` binary target, initial `App`/`SystemAdapter` shape, fake
  test support, and smoke tests are in place.
- Phase 2 implementation completed and review findings are resolved.
- Phase 3 implementation completed, reviewed, and verified.
- Phase 4 implementation completed, reviewed, and verified.
- Phase 5 implementation completed, reviewed, verified, and committed.
- Phase 6 implementation completed, reviewed, and verified.
- Phase 7 implementation completed, reviewed, and verified.
- Phase 8 implementation completed, reviewed, and verified.
- Phase 9 audit/packaging completed and locally verified.

## Decisions Captured

- Full parity with the functional Go version is required.
- Final Rust binary name: `wgman-rs`.
- Runtime commands, flags, and default config path remain compatible with Go.
- Output should be close to Go output, but byte-for-byte parity is not
  mandatory.
- Common maintained Rust crates are acceptable.
- The plan should keep core complexity in DB/config, `check`, and `deploy`;
  command handlers should remain thin.
- Preparatory artifacts live under `rust-port/`.

## Session Log

### Planning Baseline

- Created planning artifacts under `rust-port/`.
- Next step: commit the baseline, then start Phase 1 with a medium-reasoning
  worker agent.
- Progress tracking baseline committed as `22e52a6`.

### Phase 1: Rust Skeleton And Project Conventions

- Added `Cargo.toml`, `Cargo.lock`, `Makefile`, `src/main.rs`, `src/lib.rs`,
  module placeholder files, and command module placeholders under
  `rust-port/`.
- Configured the binary target as `wgman-rs`.
- Added `SystemAdapter` with `RealSystemAdapter` plus an `App<S>` CLI
  dependency-injection boundary.
- Implemented smoke-only CLI behavior for no args, `help`, `-h`, `--help`, and
  unsupported commands.
- Added integration smoke tests using a local fake system adapter.
- Updated Rust notes with Phase 1 conventions.
- Acceptance checks passed: `cargo test`, `cargo fmt --check`,
  `cargo clippy --all-targets -- -D warnings`, and `make check`.
- Phase 1 committed as `aaa01c8`.

### Phase 2: Data Model, YAML, And DB Persistence

- Implementation worker `Rawls` completed Phase 2 under `rust-port/` only.
- Changed files from the implementation session:
  - `rust-port/Cargo.toml`
  - `rust-port/Cargo.lock`
  - `rust-port/src/model.rs`
  - `rust-port/src/config.rs`
  - `rust-port/src/db.rs`
  - `rust-port/src/utils.rs`
  - `rust-port/tests/config_db.rs`
  - `rust-port/docs/NOTES.md`
  - `rust-port/PROGRESS.md`
- Implemented Rust data models, YAML loading with unknown-field/tag rejection,
  config/DB validation, resource port parsing, clone/access normalization, and
  atomic deterministic DB writes.
- Local checks passed before review:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
- High-reasoning review agent `Gibbs` found issues:
  - High: `rust-port/src/db.rs` writes YAML-ambiguous strings as plain scalars.
    Example risk: `pub_key: "123"` is saved as `pub: 123`, then reload fails
    because YAML parses it as an integer rather than a string.
  - Medium: empty users map is written as `users:` instead of `users: {}`,
    causing reload to treat it as null/missing.
- Review-identified missing tests:
  - save/load round trips for YAML-ambiguous strings such as `"123"`,
    numeric-looking names, scalar-looking comments, and access targets;
  - empty top-level sections, especially `users: {}`.
- Review findings were resolved after resumption:
  - `db.rs` now quotes scalars that would parse back as non-string YAML
    values.
  - empty users are written as `users: {}`.
  - regression tests cover ambiguous string round trips and empty users.
- Acceptance checks passed after fixes:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`

### Pause: User Requested Stop

- User requested a pause because a connection loss may happen soon.
- Current worktree has uncommitted Phase 2 implementation changes plus this
  progress update.
- The intended next action after user confirmation is to resolve the Phase 2
  review findings, not to start a new phase.

### Resume After Pause

- User confirmed resumption.
- Fixed the Phase 2 review findings locally and reran all Phase 2 checks.
- Next step: start Phase 3 with a medium-reasoning worker agent after the
  Phase 2 implementation commit.

### Phase 3: Parsers, Formatting, And Utility Functions

- Implemented WireGuard dump parsing in `src/parse_wg.rs`, including server
  line parsing, peer rows, numeric field errors, IPv4 `/32` allowed-IP
  normalization, `(none)` endpoint preservation, and rejection of multiple
  allowed IPs.
- Implemented ipset save parsing in `src/parse_ipset.rs`, including create
  set name/type parsing, add entries, optional comments with spaces, empty
  sets, duplicate create/add-before-create errors, trailing-content errors,
  and create/add set-name mismatch errors.
- Added Phase 3 utilities in `src/utils.rs`: endpoint host stripping,
  split-lines helper, confirmation prompt helper, IPv4/CIDR validation,
  IPv4 CIDR parsing, IPv4/u32 conversion, alongside existing sorted keys and
  ASCII case fold helpers.
- Implemented formatting in `src/format.rs`: ANSI color helpers, visible-width
  table writer, byte formatting/coloring, handshake age parts, and plain/color
  handshake formatting.
- Added `tests/phase3.rs` covering Go parity cases from parser, formatter, and
  utility tests.
- Updated Rust implementation notes with Phase 3 parser/format conventions.
- Local acceptance checks passed:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- High-reasoning review agent `Godel` found no Phase 3 issues. Residual risk:
  Go reference tests could not be run because `go` is not installed in this
  environment.
- Phase 3 committed as `699312c`.

### Phase 4: Check Engine

- Implemented `CheckResult`, `ExpectedIPSets`, `IPSetDelta`, `WGPeerDelta`, and
  `WGPeerDeltaAction` in `src/model.rs`.
- Extended `SystemAdapter` with read-only live-state methods for interface
  subnet discovery, WireGuard dump reads, and ipset list reads.
- Implemented real read-only command execution in `src/system_real.rs` using
  argument arrays for `ip`, `wg`, and `ipset`.
- Implemented `check::check` and `compute_expected_ipsets` in `src/check.rs`.
  The engine validates DB/config first, checks interface subnet membership,
  parses live WireGuard state, produces safe peer add/remove deltas, rejects
  unknown peers and peer IP mismatches as hard errors, skips ipsets after WG
  hard errors, validates managed ipset name/type/entry shape, separates hard
  errors from drift, and emits concrete ipset add/delete deltas.
- Added `tests/phase4.rs` with Go-parity coverage for clean state, resource
  access, inactive users, peer drift, hard WG errors, subnet errors, missing
  sets, malformed ipsets, drift/deltas, stable expected comments, and expected
  ipset computation.
- Updated `docs/NOTES.md` with Phase 4 check-engine conventions.
- Local acceptance checks passed:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- High-reasoning review agent `Dirac` found one low-severity stability issue:
  early WG dump/parse error returns did not sort previously collected hard
  errors, and identified a missing inactive-user ipset cleanup test.
- Review findings were resolved:
  - `check.rs` now sorts before WG dump/parse early returns.
  - `tests/phase4.rs` now directly covers inactive-user ipset delete drift.
- Acceptance checks passed after review fixes:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- Next step after commit: start Phase 5 with a medium-reasoning worker agent.

### Phase 5: Deploy Engine And Rollback

- Implemented the deploy engine in `src/deploy.rs`: tracked ipset apply,
  ipset inversion, expected-ipset diffing, tracked WireGuard peer apply, peer
  inversion, full dependency-ordered state apply, and reverse dependency-order
  rollback.
- Extended `SystemAdapter` and `RealSystemAdapter` with mutation methods for
  ipset add/delete and WireGuard peer add/remove.
- Extended the Rust test fake with mutation error injection and operation
  recording.
- Added `tests/phase5.rs` covering Go deploy test parity for partial
  failures, stable diffs, dependency ordering, completed operation tracking,
  and rollback ordering.
- Local acceptance checks passed:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- High-reasoning review agent `Ampere` found no Phase 5 issues. Residual risk:
  real `wg`/`ipset` mutation paths are covered by code review and fake-system
  tests but not exercised against real Linux tools in this environment.
- During parent review, `diff_expected_ipsets` was adjusted to sort delete
  operations before add operations for matching set/entry keys, matching the
  Go comparator. A regression test covers the ordering.
- Phase 5 committed as `237140d`.
- Next step after the Phase 5 commit: start Phase 6 with a medium-reasoning
  worker agent.

### Phase 6: Read-Only Commands

- Implemented Go-compatible CLI help text, global flag parsing, command
  dispatch, no-args/help behavior, unknown-command exit code `2`, and
  create/add-only `-c` parsing validation.
- Implemented `check`, `list`, and `show` handlers under `src/commands/`.
  Handlers enforce root checks, load config/db, run the existing check engine,
  and keep command logic thin.
- Added output helpers for check findings, future planned ipset delta output,
  and colored status output.
- `check` reports hard errors, WireGuard drift, ipset drift, and final
  OK/FAILED status with TTY-aware color. Planned delta details remain reserved
  for deploy/remove/mod planning paths to match Go.
- `list` requires a clean check result and prints users, VMs, resources, or
  a single-user access filter with inactive suffixes and Go-style colorization.
- `show` requires a clean check result and prints WireGuard peers mapped to
  users with endpoint port stripping, byte/handshake formatting, inactive
  suffixes, sorting by user name, and Go-style colorization.
- Added `tests/phase6.rs` covering read-only CLI parsing, root checks,
  unsupported dry-run behavior, check/list/show output, color suppression, and
  unclean-state refusals. Updated smoke tests for the Go-style unknown command
  message.
- Mutation commands (`init-ipsets`, `deploy`, `create`/`add`, `remove`, `mod`)
  remain intentionally unsupported for later phases.
- Local acceptance checks passed:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- High-reasoning review agent `Newton` found two Phase 6 parity issues:
  - `check` printed planned ipset deltas, unlike Go `cmdCheck`.
  - explicit boolean flag assignment forms such as `--dry-run=false` and
    single-dash long flags such as `-config-dir` were rejected.
- Review findings were resolved:
  - `check` no longer prints planned ipset deltas.
  - CLI flag parsing accepts Go-style boolean assignments and single-dash long
    global flags.
  - regression tests cover both fixes.
- Acceptance checks passed after review fixes:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- After the Phase 6 commit, pause before starting Phase 7 until the user
  confirms continuation.

### Phase 7: Init And Deploy Commands

- Implemented `init-ipsets` under `src/commands/init_ipsets.rs`. It enforces
  root/no positional args, loads `config.yaml`, creates all three configured
  managed sets with Go-compatible types/comment support, and reports
  `init-ipsets: OK`.
- Extended `SystemAdapter`, `RealSystemAdapter`, and the test fake with
  `ipset_create`. The real adapter uses `ipset create ... family inet -exist`
  and enables comments for the two matrix sets.
- Implemented `deploy` under `src/commands/deploy.rs`. It enforces root/no
  positional args, loads config/db, runs `check`, refuses hard errors, reports
  planned peer/ipset changes, supports `--dry-run`, prompts unless `--yes`,
  and applies changes only through `apply_state_deltas`.
- Added `print_peer_deltas` and wired CLI dispatch to `init-ipsets` and
  `deploy`; create/add/remove/mod remain unsupported for Phase 8.
- Added `run_with_input` to the CLI app so confirmation prompts can read from
  injected input in tests and from stdin in the real binary.
- Added `tests/phase7.rs` covering Go-parity init/deploy behavior, dry-run,
  confirmation abort, `--yes`, peer add ordering, inactive cleanup ordering,
  and apply failures.
- Local acceptance checks passed:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- High-reasoning review agent `Bacon` found no Phase 7 issues. Residual risk:
  real `wg`/`ipset` mutation paths are covered by code review and fake-system
  tests but not exercised against real Linux tools in this environment.
- After review, additional regression coverage was added for prompt-confirmed
  deploy and deleting unexpected ipset entries without peer drift.
- Acceptance checks passed after the added coverage:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- Next step after commit: start Phase 8 with a medium-reasoning worker agent.

### Phase 8: Create, Remove, And Modify Commands

- Implemented `create`/`add` under `src/commands/create.rs`: Go-compatible
  argument parsing including command-local `-c`, optional IP or access list,
  `/32` IP normalization, next-IP allocation, key generation through
  `SystemAdapter`, duplicate/case-conflict checks, client config rendering,
  no-overwrite `0600` config writes, DB mutation planning, live apply, DB
  save, and best-effort rollback of live state plus generated config.
- Implemented client config generation in `src/client_config.rs`, including
  comment-line stripping, leading blank trimming, placeholder replacement, and
  no-overwrite writes.
- Extended `SystemAdapter`, `RealSystemAdapter`, and the integration-test fake
  with `wg_gen_key` and `wg_pub_key`; the real adapter uses `wg genkey` and
  `wg pubkey` with argument arrays/stdin.
- Implemented `remove` under `src/commands/remove.rs`: clean-state refusal,
  planned-removal summary, prompt/`--yes`, `--dry-run`, DB mutation planning,
  deploy-engine live apply, DB save, and rollback on live or save failures.
- Implemented `mod` under `src/commands/modify.rs`: access expression parsing,
  activation/deactivation, inactive-user DB-only access edits, dry-run, no-op
  detection, validation of updated DB, Go-compatible DB-first access edits,
  and rollback-backed live mutations for activation toggles.
- Wired create/add/remove/mod into CLI dispatch and removed the Phase 8
  unsupported-command path.
- Added `tests/phase8.rs` covering client config rendering/writes, create
  success/no-private-key and rollback, create parsing/drift/dry-run errors,
  remove dry-run/prompt/apply/rollback, mod access edits, activation, dry-run,
  rollback, and strict CLI parsing edge cases.
- Local checks passed:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- Phase 8 implementation committed as `7644ac7`.
- High-reasoning review agent `Avicenna` found two Phase 8 issues:
  - Medium: `mod` access-edit failure semantics diverged from Go by applying
    ipsets before saving `db.yaml` and rolling back on live failure.
  - Low: `cmd_create` did not reject `--dry-run` when called directly, relying
    only on CLI dispatch.
- Review findings were resolved:
  - `mod` access edits now save `db.yaml` first and apply ipset deltas without
    rollback, matching Go's failure semantics and leaving live drift for
    `deploy` if an ipset operation fails.
  - `cmd_create` now rejects `--dry-run` directly through the shared CLI guard.
  - Regression tests cover both fixes.
- Acceptance checks passed after review fixes:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- Residual review risks: real `wg`/`ipset` command paths were not exercised
  against live Linux tools, and additional Go edge-case parity tests remain
  useful for Phase 9 audit coverage.
- Next step after commit: start Phase 9 final audit and packaging with a
  medium-reasoning worker agent.

### Phase 9: Parity Audit And Packaging

- Audited the Phase 8 Rust command coverage against the requested Go parity
  tests and `docs/SPEC.md`, focusing on create/remove/mod edge cases and
  rollback behavior.
- Added focused Rust parity coverage in `tests/phase8.rs` for:
  - resource-port access deltas in `create` and `mod`;
  - duplicate and case-conflicting create users;
  - supplied IP conflicts, generated public-key conflicts, and resource access
    creation;
  - missing remove users and admin all-access set deletion;
  - inactive-user DB-only access edits;
  - redundant `activate`/`deactivate` no-ops;
  - DB-save failure rollback for `create` and `remove` using the real atomic
    writer with read-only Unix temp directories.
- Confirmed the added parity tests do not require root, WireGuard, ipset,
  iptables, or live network interfaces.
- Added `rust-port/README.md` with concise build/check/usage notes for the
  final artifact.
- Updated `rust-port/docs/NOTES.md` with Phase 9 test conventions, packaging
  expectations, and the intentional limitation that live Linux `wg`/`ipset`
  command paths remain fake-tested in the normal suite.
- Verification run during Phase 9:
  - `cargo test --test phase8`
  - `cargo fmt`
  - `cargo run --quiet -- help`
  - `make check`
- `make check` passed from `rust-port/`: `cargo fmt --check`,
  `cargo clippy --all-targets -- -D warnings`, and `cargo test` all succeeded.
- `wgman-rs help` output was sanity-checked with `cargo run --quiet -- help`;
  it includes the expected command list, create/add alias, dry-run/yes support,
  and color/config flags.
- Phase 9 audit/packaging committed as `9f504e4`.
- High-reasoning final review agent `Jason` found one Phase 9 issue:
  permission-based DB-save failure tests could fail under root-run Linux CI
  because root can bypass directory write bits.
- Review finding was resolved:
  - the two permission-based save-failure tests now skip when effective UID is
    `0`;
  - implementation notes now document that these tests are normal non-root
    Linux coverage and skip under root.
