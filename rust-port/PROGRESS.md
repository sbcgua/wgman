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
- Phase 5 follow-up ordering fix completed and ready to commit.

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
- Phase 5 committed as `237140d`.
- The original Phase 5 worker was interrupted after the commit. A replacement
  medium-reasoning worker `Boyle` inspected the current state, kept the patch
  structure, and made one parity fix: `diff_expected_ipsets` now sorts delete
  operations before add operations for matching set/entry keys, matching the
  Go comparator. A regression test covers the ordering.
- Follow-up checks passed:
  - `cargo test`
  - `cargo fmt --check`
  - `cargo clippy --all-targets -- -D warnings`
  - `make check`
- Next step after the follow-up fix commit: start Phase 6 with a
  medium-reasoning worker agent.
