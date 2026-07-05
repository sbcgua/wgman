# WGMAN Implementation Plan

This document is a handoff plan for a fresh agent implementing `wgman`.

Primary specification: [docs/SPEC.md](../SPEC.md). Do not re-derive behavior from this plan when the spec is more precise; use this file for sequencing, engineering approach, and environment setup.

## Suggested Skills

- `handoff`: use if the work is paused and another agent needs to resume.
- `grilling`: use only if new product/design ambiguity appears and the user wants further stress-testing before implementation.

## Starting Assumptions

- Language choice is Go.
- The agent starts in an empty dev container with Go installed.
- The dev container does not have WireGuard, `iptables`, or `ipset`, so normal development must rely on pure unit tests and fake system adapters.
- Runtime target remains a Linux host with WireGuard tools and ipset/firewall state already configured, as described in [docs/SPEC.md](../SPEC.md).
- The final deliverable is one executable file named `wgman`; the source can be split into multiple Go files for maintainability.

## Phase 0: Bootstrap The Go Project

1. Confirm workspace contents:

   ```sh
   ls
   sed -n '1,260p' docs/SPEC.md
   ```

2. Initialize the Go module from the repository root if `go.mod` does not exist:

   ```sh
   go mod init wgman
   ```

3. Install the only required runtime library, the YAML parser:

   ```sh
   go get gopkg.in/yaml.v3@v3.0.1
   go mod tidy
   ```

4. Use only the standard library plus `gopkg.in/yaml.v3` unless there is a strong reason to add another dependency. The CLI can be implemented with `flag` or minimal custom parsing.

5. Recommended optional local quality tools, not runtime dependencies:

   ```sh
   go install honnef.co/go/tools/cmd/staticcheck@latest
   ```

   If network access is unavailable, skip optional tools and rely on `go test`, `go vet`, and `gofmt`.

## Phase 1: Establish Project Structure

Keep the implementation compact but testable. A reasonable initial layout:

```text
.
+-- go.mod
+-- go.sum
+-- main.go
+-- cli.go
+-- config.go
+-- model.go
+-- parse_wg.go
+-- parse_ipset.go
+-- check.go
+-- deploy.go
+-- system.go
+-- create.go
+-- format.go
+-- *_test.go
```

This still builds to one executable. Avoid nested packages until there is a real need.

Important boundary: put all subprocess calls, root checks, interface address discovery, and file writes behind small interfaces or function variables. The check/deploy logic should accept already-loaded state or an adapter so it can be tested without real system tools.

## Phase 2: Core Data Model And YAML Handling

Implement typed structs for:

- global config loaded from `config.yaml`;
- database loaded from `db.yaml`;
- live WireGuard state;
- live ipset state;
- validation findings;
- deploy deltas.

YAML requirements:

- Use `yaml.Decoder.KnownFields(true)` for config/database decoding.
- Use plain typed structs and maps, not arbitrary YAML node execution or custom object tags.
- Normalize IPs during loading/validation where appropriate, but preserve a clear distinction between raw parse errors and semantic validation errors.
- Write `db.yaml` atomically: temp file in same directory, close, then rename.
- It is acceptable for tool-written YAML to not preserve comments.

Add `--config-dir`, defaulting to `/etc/wireguard/wgman`, early. This makes local tests and fixture-based runs practical.

## Phase 3: Pure Parsers

Implement parsers before command logic. They are critical and easy to unit test.

1. `wg show <interface> dump` parser:
   - parse server/interface line separately from peer lines;
   - normalize peer allowed IPs from `/32` to plain IPv4;
   - parse endpoint, transfer counters, latest handshake, and persistent keepalive fields only to the degree needed by the spec;
   - return structured errors for malformed rows.

2. `ipset list <setname> -o save` parser:
   - parse the set name and set type from `create` lines;
   - parse `add` lines for both all-access and matrix sets;
   - parse optional comments without depending on comments for correctness;
   - return structured entries that can be compared to expected state.

3. Interface address discovery parser:
   - keep the actual command in `system.go`;
   - parse command output in a pure function with fixtures.

Do not attempt to shell-parse by string concatenation. Use `exec.Command` with argument slices in the system adapter.

## Phase 4: Validation And Check Routine

Implement the internal check routine as the central state reconciliation function. Its behavior is defined in [docs/SPEC.md](../SPEC.md), especially the "Check", "Deploy", "Interview findings", and "Unit testing strategy" sections.

Recommended design:

```go
type CheckResult struct {
    HardErrors []Finding
    Drift      []Finding
    Deltas     DeltaPlan
}
```

The check routine should:

- validate static config/database content;
- compare database users to live WireGuard peers by public key;
- compare database-derived expected access to live ipset entries;
- classify system access differences as drift, not hard errors;
- return a delta plan even when the caller only intends to report it.

Keep formatting of findings separate from detection. Tests should assert structured results, not fragile output strings.

## Phase 5: Delta Planning And Deploy

Build deploy around a plan/apply split:

```go
func PlanAccessDeltas(db DB, live IPSetState) DeltaPlan
func ApplyDeltaPlan(ctx context.Context, sys System, plan DeltaPlan) error
```

The planner must generate minimal additions and deletions. The applier must only apply the plan it is given. It must not flush ipsets or rebuild them wholesale.

Represent commands as structured operations first, for example:

- add all-access entry;
- delete all-access entry;
- add matrix entry with comment;
- delete matrix entry.

Then convert operations to `ipset` command invocations in one place. This makes `--dry-run`, reporting, and unit tests straightforward.

## Phase 6: CLI And Commands

Implement command parsing after the core logic exists. Keep the CLI thin.

Suggested order:

1. `help` / `-h`
2. `check`
3. `list`
4. `show`
5. `deploy`
6. `mod`
7. `create`
8. `remove`

Rationale: `check`, `list`, and `show` validate read paths first. `deploy` validates write planning without changing `db.yaml`. `mod`, `create`, and `remove` then reuse existing validation and deployment logic.

Global flags to support from the start:

- `--config-dir`
- `--yes`
- `--dry-run`

Command handlers should return errors and result objects where practical; `main` should be responsible for final printing and exit status.

## Phase 7: User Creation And File Updates

Implement creation after check/deploy foundations are in place.

Key implementation notes:

- Generate keys by invoking `wg genkey` and `wg pubkey` through the system adapter on the real host.
- In tests, fake key generation.
- Render the user config from `user.conf.template`.
- Write the generated client config to the current directory.
- Never write private keys to `db.yaml`.
- Update `db.yaml` atomically before applying live system changes, or explicitly document and implement the chosen order. Prefer a recoverable order and clear error reporting.

Consider adding backup behavior for `db.yaml` before mutation, such as `db.yaml.bak`, only if it stays simple. If added, document it in the spec first.

## Phase 8: Unit Tests

Implement tests as each phase is built; do not defer all tests to the end.

Recommended files:

- `parse_wg_test.go`
- `parse_ipset_test.go`
- `config_test.go`
- `check_test.go`
- `deploy_test.go`
- `cli_test.go`
- `create_test.go`

Use table-driven tests with inline fixture strings for external command outputs. Fake system adapters should record intended commands instead of executing them.

Minimum test command:

```sh
go test ./...
```

Useful local quality commands:

```sh
gofmt -w .
go test ./...
go vet ./...
staticcheck ./...
```

Run `staticcheck` only if installed.

## Phase 9: Dev Container Workflow

The dev container can complete most implementation work without WireGuard or firewall tools.

Allowed in the dev container:

- compile the binary;
- run unit tests;
- test CLI argument parsing;
- test parser fixtures;
- test config/database load/write using temp directories;
- test deploy planning with fake adapters.

Not meaningful in the dev container unless tools are installed and configured:

- real `wg show`;
- real `wg set`;
- real `ipset list/add/del`;
- root behavior;
- real interface subnet discovery.

To keep development smooth, avoid command handlers directly calling `os/exec`. They should call the system adapter.

## Phase 10: Real Host Smoke Testing

After unit tests pass, verify on a controlled Linux host or VM.

Suggested smoke-test sequence:

1. Install/build the binary:

   ```sh
   go build -o wgman .
   sudo install -m 0755 wgman /usr/local/sbin/wgman
   ```

2. Prepare a non-production WireGuard interface and dedicated test ipsets.

3. Create `/etc/wireguard/wgman/config.yaml`, `/etc/wireguard/wgman/db.yaml`, and `/etc/wireguard/wgman/user.conf.template` based on [docs/SPEC.md](../SPEC.md).

4. Run:

   ```sh
   sudo wgman check
   sudo wgman list
   sudo wgman show
   sudo wgman deploy --dry-run
   ```

5. Test one controlled access change through `mod` or `deploy`.

6. Test `create` only after confirming generated config files are written to the expected current directory and no private key is added to `db.yaml`.

Do not run first smoke tests against production firewall state.

## Phase 11: Build Artifact

Final build command:

```sh
go build -trimpath -ldflags="-s -w" -o wgman .
```

The output `wgman` binary is the single executable intended for `/usr/local/sbin`.

## Open Engineering Decisions To Resolve During Implementation

Do not block initial implementation on these unless they affect a current phase:

- Exact terminal table formatting for `list` and `show`.
- Whether color output is auto-disabled when stdout is not a terminal.
- Exact backup policy for `db.yaml` mutations.
- Exact order of `db.yaml` mutation versus live system mutation for `create`, `remove`, and `mod`.
- Whether to add a hidden/test-only command or fixture mode. Prefer not to unless tests need it.

If any decision changes command behavior, update [docs/SPEC.md](../SPEC.md) before coding the changed behavior.
