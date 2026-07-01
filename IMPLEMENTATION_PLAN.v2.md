# WGMAN Implementation Plan v2: Vertical Slices

This is an alternative implementation plan for a fresh agent. It keeps the original [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) intact, but reorganizes the work into vertical slices so useful functionality exists after each middle phase.

Primary specification: [docs/SPEC.md](docs/SPEC.md). Do not duplicate or reinterpret command behavior here when the spec is more precise.

## Suggested Skills

- `handoff`: use if work is paused and another agent needs to resume.
- `grilling`: use only if new product/design ambiguity appears and the user wants further stress-testing before implementation.

## Phase 0: Bootstrap Project And Test Harness

Goal: establish a compilable Go project with dependencies, basic conventions, and a test harness.

Steps:

1. Inspect the current repository:

   ```sh
   ls
   sed -n '1,320p' docs/SPEC.md
   ```

2. Initialize Go module if missing:

   ```sh
   go mod init wgman
   ```

3. Add YAML dependency:

   ```sh
   go get gopkg.in/yaml.v3@v3.0.1
   go mod tidy
   ```

4. Create initial source files:

   ```text
   .
   +-- main.go
   +-- cli.go
   +-- model.go
   +-- config.go
   +-- system.go
   ```

5. Implement a minimal `main` that supports `help` and returns "not implemented" for other commands.

6. Add basic test scaffolding:

   - table-driven test helpers;
   - fake system adapter type;
   - temp config directory helpers.

Tests for this phase:

```sh
go test ./...
```

Acceptance:

- `go test ./...` passes.
- `go build -o wgman .` succeeds.
- `./wgman help` prints a command list.

## Phase 1: Check Slice, Part 1: Config And Database Validation

Goal: make `wgman check --config-dir <dir>` useful for offline validation of `config.yaml` and `db.yaml`, without requiring WireGuard or ipset.

Functionality:

- Load `config.yaml`, `db.yaml`, and validate their structure.
- Use typed structs and `yaml.Decoder.KnownFields(true)`.
- Validate names, duplicate/case-conflicting names, duplicate IPs, VM references in `access`, `*` admin access, and required fields.
- Add an internal check result model with hard errors, drift, and delta plan fields, even if drift/deltas are not populated yet.

Implementation notes:

- Keep file I/O in a small loader boundary.
- Keep semantic validation pure and independently testable.
- Implement `--config-dir` now, defaulting to `/etc/wireguard/wgman`.
- Do not shell out to `wg` or `ipset` in this phase.

Tests for this phase:

- valid minimal config/db;
- missing required keys;
- unknown YAML fields;
- invalid user and VM names;
- duplicate IPs;
- case-conflicting names;
- unknown VM in access;
- invalid `*` usage if represented incorrectly.

Acceptance:

- `wgman check --config-dir ./testdata/valid-offline` can report config/db validity without system access when run through a fake or offline path.
- Unit tests cover the validation matrix above.

## Phase 2: Check Slice, Part 2: Parse Live System Output

Goal: add pure parsers for the external command outputs needed by `check`.

Functionality:

- Parse `wg show <interface> dump` output into live WireGuard state.
- Parse `ipset list <setname> -o save` output into live ipset state.
- Parse interface address/subnet command output through a pure helper.
- Add the real system adapter methods that invoke commands, but test the parsers with fixtures.

Implementation notes:

- Use `exec.Command` with argument slices only inside `system.go`.
- Keep command execution separate from parsing.
- Return structured parse errors with enough context to diagnose malformed rows.

Tests for this phase:

- representative `wg dump` server line and peer lines;
- peer allowed IP normalization from `/32`;
- `(none)` endpoint handling;
- malformed `wg dump` rows;
- `ipset save` output for all-access and matrix sets;
- ipset comments with spaces;
- malformed ipset entries;
- interface address parser fixtures.

Acceptance:

- `go test ./...` passes without WireGuard or ipset installed.
- Parser coverage is enough to make future check behavior deterministic.

## Phase 3: Check Slice, Part 3: Full `wgman check`

Goal: complete the central `check` behavior against config/db plus live state, with fakeable system access.

Functionality:

- Read config/db.
- Read live WireGuard dump, interface subnet, and configured ipsets through the system adapter.
- Validate users against WireGuard peers by public key and normalized IP.
- Validate user IPs are within the interface subnet.
- Compute expected ipset access state from `db.yaml`.
- Compare expected versus live ipset entries.
- Classify findings into hard errors and ipset drift.
- Return prepared deltas for drift.
- Implement user-facing `wgman check` output and exit status.

Implementation notes:

- Treat configured ipsets as fully owned by `wgman`, per [docs/SPEC.md](docs/SPEC.md).
- Formatting should be readable, but tests should primarily assert structured `CheckResult`.
- Do not apply changes in this phase.

Tests for this phase:

- clean state returns success and no deltas;
- config validation errors become hard errors;
- missing WireGuard peer becomes hard error;
- extra WireGuard peer becomes hard error;
- peer IP mismatch becomes hard error;
- missing configured ipset becomes hard error;
- missing ipset access entry becomes drift plus add delta;
- extra ipset access entry becomes drift plus delete delta;
- hard errors and drift can be reported separately.

Acceptance:

- `wgman check` is functionally complete when backed by real commands.
- The same logic is fully testable with fake adapters.

## Phase 4: Read-Only UX Slice: `list` And `show`

Goal: add the read-only commands that depend on completed check/live-state plumbing.

Functionality:

- Implement `wgman list [filter]`.
- Implement `wgman show`.
- Both commands call internal `check` and refuse to continue on failed validation, as specified.
- `list` prints users, resources, and optional access for a user.
- `show` maps WireGuard public keys to user names and formats status fields.

Implementation notes:

- Reuse loaded config/db/live state from the check path where practical to avoid duplicate command calls.
- Keep table formatting simple and stable.
- Color can be minimal; if added, keep it optional or harmless in non-TTY output.

Tests for this phase:

- `list` output for users/resources;
- `list alice` access filtering;
- `list` refuses on check failure;
- `show` maps peer public key to username;
- endpoint displayed without port;
- byte and handshake duration formatting helpers.

Acceptance:

- `wgman list` and `wgman show` are useful on a correctly configured target host.
- Unit tests do not require real WireGuard.

## Phase 5: Access Deploy Slice: `deploy`

Goal: make db-driven access reconciliation functional, with dry-run and confirmation support.

Functionality:

- Implement delta operation model for ipset add/delete actions.
- Implement `wgman deploy`.
- Support `--dry-run` and `--yes`.
- `deploy` calls internal check, refuses hard errors, and applies ipset drift deltas only.
- Apply deltas through the system adapter.
- Report planned and applied changes clearly.

Implementation notes:

- The planner should produce minimal deltas.
- The applier must not flush or rebuild ipsets.
- Convert structured operations to `ipset` command arguments in one place.

Tests for this phase:

- deploy refuses hard errors;
- deploy with no drift reports no changes;
- deploy dry-run reports but does not execute;
- deploy applies add/delete deltas in expected form;
- `--yes` bypasses confirmation;
- without `--yes`, confirmation rejection prevents execution.

Acceptance:

- Admins can manually edit access in `db.yaml`, run `wgman deploy --dry-run`, then run `wgman deploy` to reconcile ipsets.
- Unit tests verify command decisions and fake adapter calls.

## Phase 6: Modify Access Slice: `mod`

Goal: make controlled access edits possible through the CLI.

Functionality:

- Implement `wgman mod <name> <+res1,-res2...>`.
- Parse comma-separated add/remove operations.
- Validate user and VM/resource names.
- Refuse to run if check reports hard errors or ipset drift.
- Update `db.yaml` atomically.
- Apply corresponding access deltas through the same deploy routine.
- Support `--dry-run`.

Implementation notes:

- Plan the db change and system change before writing anything.
- Preserve deterministic ordering in written access lists.
- Reuse delta planning rather than creating special-case ipset commands.

Tests for this phase:

- parse valid and invalid mod expressions;
- refuse missing user;
- refuse unknown VM;
- refuse cleanly on pre-existing drift;
- dry-run does not write db or apply system updates;
- successful mod writes expected db and applies expected ipset deltas;
- removing absent access is either a no-op or clear error, matching the implemented decision documented in the spec if needed.

Acceptance:

- `mod` can add/remove VM access for an existing user on a clean system.
- The updated db and live ipsets remain consistent after the command.

## Phase 7: Create User Slice: `create`

Goal: make new user provisioning functional.

Functionality:

- Implement `wgman create <name> [ip] [res1,res2...]`.
- Parse optional IP versus access list.
- Auto-allocate IP from the interface subnet and existing users when not supplied.
- Generate WireGuard private/public keys through the system adapter.
- Render `<user>.vpn.conf` from `user.conf.template` in the current directory.
- Add user and optional access to `db.yaml`.
- Add WireGuard peer and relevant ipset entries through deploy/system routines.
- Refuse to run if check reports hard errors or ipset drift.

Implementation notes:

- Do not store private keys in `db.yaml`.
- Fake key generation in tests.
- Handle existing output config file carefully; prefer refusing to overwrite unless the spec is updated.
- Keep the order of db write versus system update explicit in code comments or documentation.

Tests for this phase:

- optional argument parsing;
- auto-IP allocation;
- supplied IP normalization and validation;
- duplicate/case-conflicting username rejection;
- unknown VM rejection;
- template substitution;
- private key absent from db;
- expected `wg set` and ipset operations recorded by fake adapter;
- refusal on pre-existing drift.

Acceptance:

- A new user can be created on a clean target host and receives a generated client config.
- Tests prove no private key is persisted to `db.yaml`.

## Phase 8: Remove User Slice: `remove`

Goal: make user removal functional and guarded by confirmation/dry-run behavior.

Functionality:

- Implement `wgman remove <name>`.
- Show deletion summary before applying.
- Support `--yes` and `--dry-run`.
- Refuse to run if check reports hard errors or ipset drift.
- Remove user and access entries from `db.yaml`.
- Remove WireGuard peer and corresponding ipset entries through system/deploy routines.

Implementation notes:

- Use public key from `db.yaml` for WireGuard peer removal.
- Do not remove generated client config files.
- Keep planned operations visible before confirmation.

Tests for this phase:

- missing user rejection;
- confirmation rejection prevents db and system changes;
- `--yes` applies without prompt;
- dry-run reports only;
- db user and access removal;
- expected `wg set ... remove` operation;
- expected ipset delete operations.

Acceptance:

- Existing users can be removed safely from db, WireGuard, and managed access sets.

## Phase 9: Polish, Packaging, And Documentation Alignment

Goal: make the command coherent as a user-facing tool.

Functionality:

- Review all help text and command result messages.
- Ensure final line of each command concisely reports success/failure.
- Ensure exit codes are consistent.
- Ensure README commands match implementation.
- Add any missing examples to docs only if they are not already covered by [docs/SPEC.md](docs/SPEC.md).

Tests for this phase:

- smoke-style CLI tests for help and argument errors;
- output golden tests only for stable, high-value text;
- test non-root behavior through fake root checker where possible.

Acceptance:

- `go test ./...`, `go vet ./...`, and `gofmt` pass.
- README and spec do not contradict implemented flags or command names.

## Phase 10: Real Host Smoke Checks And Final Build

Goal: verify the binary on a controlled Linux host with real WireGuard/ipset state.

Build:

```sh
go build -trimpath -ldflags="-s -w" -o wgman .
```

Install:

```sh
sudo install -m 0755 wgman /usr/local/sbin/wgman
```

Smoke test on a non-production host or VM:

```sh
sudo wgman check
sudo wgman list
sudo wgman show
sudo wgman deploy --dry-run
```

Then test one controlled write path:

```sh
sudo wgman deploy
sudo wgman mod testuser +somevm --dry-run
```

Only after dry-run output is correct should real `mod`, `create`, or `remove` be tested.

Acceptance:

- Normal unit tests pass in the dev container.
- Smoke tests pass on a controlled target host.
- Final artifact is one executable file named `wgman`.

## Recommended Implementation Order Summary

1. Bootstrap and test harness.
2. Offline `check` for config/db.
3. Live output parsers.
4. Full `check`.
5. `list` and `show`.
6. `deploy`.
7. `mod`.
8. `create`.
9. `remove`.
10. Polish and real-host verification.
