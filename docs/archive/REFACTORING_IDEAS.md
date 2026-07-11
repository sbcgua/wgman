# Refactoring Ideas

## 1. Separate Command Orchestration From Command Logic

Apply the `cmd_check.go`/`check.go` and `cmd_deploy.go`/`deploy.go` pattern to
create, modify, and remove:

- Keep argument validation, prompts, output, flags, and exit codes in
  `cmd_create.go`, `cmd_mod.go`, and `cmd_remove.go`.
- Move planning, live-state application, and rollback routines into
  `create.go`, `mod.go`, and `remove.go`.
- Split tests along the same boundary: command tests should cover CLI behavior,
  while logic tests should call planners and apply/rollback routines directly.
- Keep command-specific transaction ordering explicit until genuinely shared
  behavior emerges; avoid introducing a generic transaction framework early.

## 2. Narrow Engine-Facing System Interfaces

Keep `SystemAdapter` as the complete OS boundary held by `App`, but make core
engines depend on smaller consumer-defined interfaces:

- `Check` needs only read operations such as interface subnet discovery,
  WireGuard dump retrieval, and ipset listing.
- Deploy application needs only WireGuard and ipset mutation operations.
- Key generation and root/terminal checks remain command-level capabilities.

This makes engine dependencies explicit and prevents unrelated system methods
from becoming accidental requirements in focused tests.

## 3. Add Focused Output Tests

Add `output_test.go` with buffer-based tests for the shared print functions:

- Check error and drift sections.
- Ipset and WireGuard delta rendering, including comments and removals.
- Apply errors with successful and failed rollback.
- Remove plans with empty and non-empty access lists.

Command tests should continue to cover where output is emitted; these focused
tests would cover the exact rendering contracts independently.

## Guardrail

Do not extract the repeated command preflight sequence yet. Root checks,
clean-state requirements, and command-specific failure wording differ enough
that a common helper could hide behavior instead of simplifying it.
