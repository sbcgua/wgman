# WGMAN Improvement 1 Progress

## Start Status

- Started Phase 0 on 2026-07-05.
- Initial `git status --short` output was clean.
- Read `IMPROVEMENT1.PLAN.md`, `docs/SPEC.md`, and `docs/NOTES.md`.

## Baseline Verification

- `GOCACHE=/tmp/wgman-gocache go test ./...` passed.
- `GOCACHE=/tmp/wgman-gocache go vet ./...` passed with no output.
- `gofmt -l .` passed with no output.
- No pre-existing failures were found before implementation changes.

## Phase 1 Verification

- `GOCACHE=/tmp/wgman-gocache go test ./...` passed.
- `GOCACHE=/tmp/wgman-gocache go vet ./...` passed with no output.
- `gofmt -l .` passed with no output.
- Existing `testdata/valid-offline/db.yaml` still loads through the test suite.

## Phase 2 Verification

- `GOCACHE=/tmp/wgman-gocache go test ./...` passed.
- `GOCACHE=/tmp/wgman-gocache go vet ./...` passed with no output.
- `gofmt -l .` passed with no output.
- Tests cover `create -c`, `add ... -c`, empty comment rejection, and saved
  `db.yaml` comment metadata.

## Phase 3 Verification

- `GOCACHE=/tmp/wgman-gocache go test ./...` passed.
- `GOCACHE=/tmp/wgman-gocache go vet ./...` passed with no output.
- `gofmt -l .` passed with no output.
- Tests cover inactive users absent from live state, inactive WireGuard peer
  removal deltas, inactive ipset delete deltas, active missing peers producing
  add deltas, unknown peers remaining hard errors, and inactive users being
  excluded from expected ipsets.

## Phase 4 Verification

- `GOCACHE=/tmp/wgman-gocache go test ./...` passed.
- `GOCACHE=/tmp/wgman-gocache go vet ./...` passed with no output.
- `gofmt -l .` passed with no output.
- Tests cover deploy dry-run reporting inactive peer removal and active peer
  addition, `--yes` applying inactive peer removal and active peer addition,
  active peer additions applying before ipset adds, inactive ipset deletes
  applying before peer removal, confirmation rejection applying nothing, peer
  removal failure returning exit code 1, and unrelated hard errors still being
  refused.

## Phase 5 Verification

- `GOCACHE=/tmp/wgman-gocache go test ./...` passed.
- `GOCACHE=/tmp/wgman-gocache go vet ./...` passed with no output.
- `gofmt -l .` passed with no output.
- Tests cover toggle parsing, unknown bare operation rejection, activate and
  deactivate dry-runs, deactivate writing `inactive: true` and removing live
  state, activate omitting `inactive` and restoring live state, inactive-user
  DB-only access edits, comment/access preservation, and redundant toggle
  no-ops.

## Behavior Decisions

- User metadata fields are rendered in deterministic DB output after `pub`.
- `inactive: false` is represented by omitting the field; missing `inactive`
  continues to mean active.
- Empty `comment` is omitted from deterministic DB output.
- `create`/`add -c` comments are trimmed before storing. Empty or
  whitespace-only explicit comments are rejected with exit code 2.
- Duplicate `-c` usage is rejected with exit code 2.
- The existing shared two-pass CLI parser now registers `-c` so it can appear
  before or after `create`/`add`; the flag is rejected for other commands.
- Inactive users are excluded from expected WireGuard peers and managed ipsets.
- Inactive users that still have live WireGuard peers produce removable
  `PeerDeltas` and do not produce hard errors.
- Active users missing live WireGuard peers produce add `PeerDeltas` and do
  not produce hard errors, so manual YAML reactivation can be repaired with
  `deploy`.
- `CheckResult.OK()` is false when peer cleanup is pending, while
  `CheckResult.Clean()` remains true if there are no hard errors.
- `deploy` applies WireGuard peer additions, then ipset deltas, then
  WireGuard peer removals. This keeps manual activation repair ordered as peer
  creation followed by access entries, and inactive cleanup ordered as ipset
  entry deletion followed by peer removal.
- Deploy-driven cleanup does not change `db.yaml`, so peer removal failures are
  reported directly without DB rollback.
- Redundant `mod <user> activate` and `mod <user> deactivate` succeed as no-op
  commands with a clear message.
- Inactive toggles apply live changes before saving `db.yaml`, with best-effort
  live rollback on live failure after partial apply or DB save failure.
- Access edits for inactive users can be DB-only changes and are saved even
  when no live ipset deltas are produced.

## Completed Phases

- Phase 0: Baseline And Guardrails.
- Phase 1: DB Schema Extension For User Metadata.
- Phase 2: Create/Add Comment Flag.
- Phase 3: Inactive State Planning And Check Semantics.
- Phase 4: Deploy Reconciliation For Inactive Users.
- Phase 5: Mod Activate/Deactivate.

## Known Gaps Or Follow-Up Work

- Phase 6 and later are not implemented in this pass.

## Post-Review Follow-Up Verification

- Implemented deploy reactivation repair for active DB users missing from live
  WireGuard, using add-capable `PeerDeltas`.
- Implemented DB-only access edits for inactive users.
- `GOCACHE=$USERPROFILE\AppData\Local\Temp\wgman-gocache GOPATH=$USERPROFILE\AppData\Local\Temp\wgman-gopath go test ./...` passed.
- `GOCACHE=$USERPROFILE\AppData\Local\Temp\wgman-gocache GOPATH=$USERPROFILE\AppData\Local\Temp\wgman-gopath go vet ./...` passed with no output.
- `gofmt -l .` passed with no output.
