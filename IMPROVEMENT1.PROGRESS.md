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
  removal deltas, inactive ipset delete deltas, active missing peers remaining
  hard errors, unknown peers remaining hard errors, and inactive users being
  excluded from expected ipsets.

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
- `CheckResult.OK()` is false when peer cleanup is pending, while
  `CheckResult.Clean()` remains true if there are no hard errors.

## Completed Phases

- Phase 0: Baseline And Guardrails.
- Phase 1: DB Schema Extension For User Metadata.
- Phase 2: Create/Add Comment Flag.
- Phase 3: Inactive State Planning And Check Semantics.

## Known Gaps Or Follow-Up Work

- Phase 4 and later are not implemented in this pass.
- `deploy` does not yet apply `PeerDeltas`; that is the explicit Phase 4
  follow-up.
