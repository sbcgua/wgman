Implemented the Rust port under `rust-port/` through Phase 9, including final audit/review fixes.

Key results:
- Rust binary target: `wgman-rs`
- Added Rust project, `Makefile`, `README.md`, `PLAN.md`, `PROGRESS.md`, and notes under `rust-port/`
- Ported config/DB, parsers/formatting, check engine, deploy engine, and all commands:
  `check`, `list`, `show`, `init-ipsets`, `deploy`, `create`/`add`, `remove`, `mod`
- Added fake-system integration coverage across the port
- Kept Go source untouched except for reading it
- Separate phase/review commits were created, latest:
  - `9f504e4` Phase 9 audit rust port parity
  - `3ea28bb` Fix phase 9 review findings

Verification:
- `make check` passed from `rust-port/`
- This includes `cargo fmt --check`, `cargo clippy --all-targets -- -D warnings`, and `cargo test`
- Worktree is clean

Residual risk documented in `rust-port/docs/NOTES.md`: real Linux `wg`/`ipset` mutation paths are covered through fake-system tests and static review, not exercised against live system tools in this environment.