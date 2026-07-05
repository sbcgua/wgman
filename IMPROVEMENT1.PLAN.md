<!-- markdownlint-disable MD032 MD007 MD029 -->
# WGMAN Improvement 1 Plan

This is a handoff plan for a fresh agent implementing the first post-Phase-11 improvement set.

Primary user-facing behavior remains defined by [docs/SPEC.md](docs/SPEC.md). Update that spec when this improvement changes command behavior or config format. Keep [README.md](README.md) user-facing; do not link development logs from it. Durable implementation conventions belong in [docs/NOTES.md](docs/NOTES.md).

## Required Progress Tracking

Create and maintain `IMPROVEMENT1.PROGRESS.md` while implementing this plan. It should record:

- completed phases;
- behavior decisions made during implementation;
- verification commands and results;
- known gaps or follow-up work.

Do not duplicate long phase history into `docs/NOTES.md`. Update `docs/NOTES.md` only with general conventions or non-obvious facts that would help a future agent safely change the project.

## Decisions Already Resolved

- Inactive users are intentionally absent from live WireGuard and managed ipsets.
- `check` must not report an inactive DB user as missing from WireGuard.
- `check` must treat an inactive user that is still present in WireGuard or managed ipsets as drift or error that `deploy` can reconcile where possible.
- `deploy` should remove inactive users from WireGuard and relevant ipsets.
- `wgman mod <user> deactivate` and `wgman mod <user> activate` should apply live changes immediately, matching existing `mod` behavior.
- `activate` should omit the `false` valued `inactive` field from written YAML for readability.
- `create -c "comment text"` and `add -c "comment text"` should store an optional user comment. Later comment editing is manual YAML editing only for this improvement.
- `show` color should be automatic for interactive terminals and suppressed for redirected output. `--no-color` must force no color.

## Phase 0: Baseline And Guardrails

Goal: verify the current tree and establish a progress file before changing behavior.

Steps:

1. Read:
  - [docs/SPEC.md](docs/SPEC.md)
  - [docs/NOTES.md](docs/NOTES.md)
  - this plan
2. Create `IMPROVEMENT1.PROGRESS.md` with the current start status.
3. Run baseline checks with writable Go caches if needed:

  ```powershell
  go test ./...
  go vet ./...
  gofmt -l .
  ```

4. Note any pre-existing failures in `IMPROVEMENT1.PROGRESS.md` before making changes.

Acceptance:

- Baseline is documented.
- No implementation changes are mixed into the baseline note.

## Phase 1: DB Schema Extension For User Metadata

Goal: add optional `comment` and `inactive` fields to user records without breaking existing DB files.

Behavior:

- `users.<name>.comment` is optional string metadata.
- `users.<name>.inactive` is optional boolean metadata.
- Missing `inactive` means active.
- Written YAML should omit `inactive` when false.
- Written YAML should omit `comment` when empty.
- Existing `db.yaml` files without these fields must still load.
- Unknown fields must continue to be rejected by `KnownFields(true)`.

Implementation notes:

- Extend `UserEntry` in [model.go](model.go).
- Use `omitempty` tags where useful, but verify deterministic custom rendering
  still behaves correctly.
- Update deterministic YAML node rendering in [config.go](config.go).
- Preserve sorted users, VMs, and access owners.
- Keep the blank line spacing between top-level DB sections.

Tests:

- Load DB with no new fields.
- Load DB with `comment`.
- Load DB with `inactive: true`.
- Reject unknown user fields.
- Save DB omits empty `comment` and false `inactive`.
- Save DB includes non-empty `comment` and true `inactive`.
- Save DB reloads successfully.

Docs:

- Update [docs/SPEC.md](docs/SPEC.md) DB example and config format text.
- Update [docs/NOTES.md](docs/NOTES.md) with the durable schema convention.
- Record completion and verification in `IMPROVEMENT1.PROGRESS.md`.

Acceptance:

- `go test ./...` passes.
- Existing fixtures still load.
- Deterministic DB output remains readable and stable.

## Phase 2: Create/Add Comment Flag

Goal: support storing user comments at creation time.

Behavior:

- `wgman create -c "comment text" <name> [ip] [res1,res2...]` stores the comment in the new user's DB record.
- `wgman add -c "comment text" <name> [ip] [res1,res2...]` behaves the same.
- Empty comments should be rejected if passed explicitly, or normalized to omitted only if the local command parsing style strongly favors that. Pick one behavior and document it in `IMPROVEMENT1.PROGRESS.md`.
- Comments are not editable through `mod` in this improvement.

Implementation notes:

- Current CLI parsing uses a global `FlagSet`. Decide whether `-c` should be a create/add-specific flag or parsed in the existing two-pass global parser.
- Prefer command-specific parsing if it avoids making `-c` look global.
- Ensure `-c` works before or after `create`/`add` if consistent with current flag ordering expectations.
- Do not store comments in generated client VPN configs.
- Preserve current create rollback behavior.

Tests:

- Parse/create with comment.
- Alias `add -c ...` stores comment.
- Saved `db.yaml` contains the comment.
- Empty or malformed `-c` usage returns exit code 2 if rejected.
- Existing create tests without comments still pass.

Docs:

- Update [docs/SPEC.md](docs/SPEC.md) create syntax.
- Update [README.md](README.md) examples if helpful and still user-facing.
- Update [docs/NOTES.md](docs/NOTES.md) only if a durable CLI parsing convention is established.
- Update `IMPROVEMENT1.PROGRESS.md`.

Acceptance:

- Comments can be added through both `create` and `add`.
- Manual YAML comments are preserved across deterministic DB writes.

## Phase 3: Inactive State Planning And Check Semantics

Goal: teach internal state planning that inactive users should not exist in live WireGuard or managed ipsets.

Behavior:

- Active users are expected exactly as today.
- Inactive users remain valid DB users.
- Inactive users are excluded from expected WireGuard peers.
- Inactive users are excluded from expected all-access and matrix ipset state.
- If an inactive user's WireGuard peer exists live, this must be actionable by `deploy`.
- If inactive user's ipset entries exist live, they should be ipset drift with delete deltas.

Important design point:

- Current `deploy` primarily applies ipset deltas and treats WireGuard mismatches as hard errors. This improvement needs a WireGuard reconciliation plan for inactive users so `deploy` can remove inactive peers. Prefer adding a structured peer delta model rather than encoding this as a string-only hard error.

Implementation notes:

- Extend `CheckResult` with WireGuard peer deltas if needed, for example: `PeerDeltas []WGPeerDeltaOp`.
- Keep hard errors for unsafe or ambiguous peer mismatches, such as an active DB user missing from WireGuard.
- Treat "inactive user peer exists live" as planned removable drift, not as an unrecoverable hard error.
- Ensure `CheckResult.OK()` and `Clean()` semantics remain clear. If new drift categories are introduced, update these helpers deliberately.
- Update `printCheckErrors` or equivalent reporting so users can see inactive peer drift.

Tests:

- Inactive user absent from WireGuard and ipsets is clean.
- Inactive user present in WireGuard produces a removable peer delta.
- Inactive user ipset entries produce delete deltas.
- Active user missing from WireGuard remains a hard error.
- Extra unknown WireGuard peer remains a hard error.
- Expected ipset computation excludes inactive users.

Docs:

- Update [docs/SPEC.md](docs/SPEC.md) state validation and deploy behavior.
- Update [docs/NOTES.md](docs/NOTES.md) with inactive/check/deploy policy.
- Update `IMPROVEMENT1.PROGRESS.md`.

Acceptance:

- `Check` can distinguish inactive-user cleanup from unsafe WireGuard drift.
- Existing hard-error protections remain intact.

## Phase 4: Deploy Reconciliation For Inactive Users

Goal: make `wgman deploy` reconcile inactive users out of live state.

Behavior:

- `deploy --dry-run` reports planned WireGuard peer removals and ipset deletes for inactive users.
- `deploy` applies those removals after confirmation unless `--yes` is used.
- `deploy` must not remove active users.
- `deploy` must still refuse unrelated hard errors.

Implementation notes:

- Reuse `SystemAdapter.WGDelPeer`.
- If a new peer-delta model was added in Phase 3, add an applier similar to ipset delta application.
- Keep operation ordering explicit. For inactive cleanup, deleting ipset entries before WireGuard peer removal is reasonable and matches current remove behavior.
- Think through rollback. For deploy-driven cleanup, there is no DB change, so partial live failures should be reported clearly. Full rollback may not be necessary, but do not hide partial application.
- Extend dry-run output in a stable, testable way.

Tests:

- Deploy dry-run reports inactive peer removal and does not call the adapter.
- Deploy with `--yes` removes inactive WireGuard peer.
- Deploy applies inactive user's ipset delete deltas.
- Confirmation rejection applies nothing.
- Peer removal failure returns exit code 1 and reports the error.
- Deploy still refuses active-user hard errors.

Docs:

- Update [README.md](README.md) only if an example is useful to operators.
- Update [docs/SPEC.md](docs/SPEC.md).
- Update [docs/NOTES.md](docs/NOTES.md) with operation ordering or delta model if it becomes a durable convention.
- Update `IMPROVEMENT1.PROGRESS.md`.

Acceptance:

- Admins can mark `inactive: true` manually and run `deploy` to remove the user from live WireGuard and managed ipsets while keeping the DB record.

## Phase 5: Mod Activate/Deactivate

Goal: add a command path to toggle inactive state and apply live changes immediately.

Behavior:

- `wgman mod <user> deactivate`
  - requires root;
  - requires clean pre-command state;
  - writes `inactive: true` to that user's DB record;
  - removes live WireGuard peer and relevant ipset entries;
  - preserves the user's DB record, IP, pubkey, comment, and access list.
- `wgman mod <user> activate`
  - requires root;
  - requires clean pre-command state except for the expected inactive absence if the check model requires special handling;
  - removes the `inactive` field from written YAML;
  - re-adds the WireGuard peer and relevant ipset entries for the existing DB access list.
- Existing `wgman mod <user> <+res1,-res2...>` behavior remains unchanged.
- `activate` and `deactivate` do not conflict with VM changes because VM operations start with `+` or `-`.
- `--dry-run` should report the planned DB and live changes without applying them, consistent with current `mod` support.

Implementation notes:

- Extend `cmdMod` parsing to branch on exactly `activate` or `deactivate`.
- Keep the access-expression parser strict for other values.
- Consider helper functions for:
  - planning inactive toggles;
  - computing active-only expected ipsets before and after the toggle;
  - WireGuard add/remove peer deltas.
- Activation requires `WGSetPeer` with existing public key and IP.
- Deactivation requires `WGDelPeer`.
- Preserve or improve existing rollback guarantees. Since `mod` currently writes DB before live ipset changes, this phase should evaluate whether
  inactive toggles need stronger rollback because WireGuard hard drift can be introduced. Prefer planning all live work, applying live changes, then committing DB, or adding rollback around post-DB live failures.

Tests:

- Parse `activate` and `deactivate`.
- Reject unknown bare mod operation like `enable`.
- Deactivate dry-run writes nothing and applies nothing.
- Deactivate writes `inactive: true`, deletes ipsets, and deletes peer.
- Activate dry-run writes nothing and applies nothing.
- Activate removes `inactive`, adds peer, and adds expected ipset entries.
- Activate preserves `comment`.
- Deactivate preserves access list.
- Redundant deactivate is a no-op or clear success message; choose and document.
- Redundant activate is a no-op or clear success message; choose and document.

Docs:

- Update [docs/SPEC.md](docs/SPEC.md) `mod` command behavior.
- Update [README.md](README.md) with a short activate/deactivate example.
- Update [docs/NOTES.md](docs/NOTES.md) with the chosen inactive-toggle no-op policy and operation ordering if relevant.
- Update `IMPROVEMENT1.PROGRESS.md`.

Acceptance:

- Operators can deactivate and reactivate users without deleting DB records.
- The live system and `db.yaml` remain consistent after successful toggles.

## Phase 6: Show Color Output And `--no-color`

Goal: add restrained color to latest-handshake output without breaking scripts.

Behavior:

- `wgman show` colorizes the `LAST HANDSHAKE` field only when stdout is an interactive terminal.
- `--no-color` suppresses color even on an interactive terminal.
- Redirected output and tests should default to no ANSI color.
- `never` is grey.
- In duration strings, day and minute components are colored in a dim but distinguishable color such as cyan.
- Hour and second components remain uncolored.
- The textual value without ANSI sequences remains unchanged, for example `2d23h48m40s`.

Implementation notes:

- Add `noColor bool` to `globalFlags`.
- Detect TTY through an injectable boundary, not direct calls buried in formatting code. For example add `IsStdoutTTY func() bool` or similar to `App`.
- Avoid a heavy terminal dependency unless there is a strong reason. On Unix targets, `os.Stdout.Stat()` plus `os.ModeCharDevice` may be enough, but test carefully. If Windows behavior is uncertain, default to no color in tests and document target behavior.
- Keep color formatting separate from `formatHandshake` if existing tests depend on plain formatting.
- Prefer small helpers such as `formatHandshakeColor(ts, now, enabled)`.

Tests:

- `--no-color` is accepted globally.
- `show --no-color` has no ANSI escape sequences.
- Non-TTY fake app has no color by default.
- TTY fake app colorizes `never` grey.
- TTY fake app colorizes day and minute components but leaves hour and second substrings uncolored.
- Existing show output tests still pass after stripping or avoiding color.

Docs:

- Update [docs/SPEC.md](docs/SPEC.md) show/color behavior.
- Update [README.md](README.md) if helpful, especially `--no-color`.
- Update [docs/NOTES.md](docs/NOTES.md) with the terminal/color convention.
- Update `IMPROVEMENT1.PROGRESS.md`.

Acceptance:

- Interactive `wgman show` is easier to scan.
- Scripted or redirected output remains stable by default.

## Phase 7: Documentation, Compatibility, And Final Verification

Goal: close the improvement set cleanly.

Steps:

1. Review all user-facing help text.
2. Ensure command result lines remain concise and consistent.
3. Ensure `README.md` remains user-facing and does not link development logs.
4. Ensure [docs/SPEC.md](docs/SPEC.md) reflects the implemented behavior: comments, inactive users, activate/deactivate, and color suppression.
5. Ensure [docs/NOTES.md](docs/NOTES.md) contains only durable conventions, not phase history.
6. Finalize `IMPROVEMENT1.PROGRESS.md` with verification results and any known residual risk.

Verification:

```powershell
go test ./...
go vet ./...
gofmt -l .
go build -trimpath -ldflags='-s -w' -o $USERPROFILE\AppData\Local\Temp\wgman-improvement1.exe .
bash -n share/usr/local/sbin/wgman-firewall-hook.template
```

Acceptance:

- All verification commands pass, or any environment-specific skipped command is clearly documented in `IMPROVEMENT1.PROGRESS.md`.
- `IMPROVEMENT1.PROGRESS.md` is ready for handoff.
- `docs/NOTES.md`, `docs/SPEC.md`, and `README.md` are aligned with behavior.

## Suggested Implementation Order Summary

1. Baseline and progress file.
2. DB schema fields and deterministic YAML.
3. Create/add comment flag.
4. Inactive check semantics and planning model.
5. Deploy inactive reconciliation.
6. Mod activate/deactivate.
7. Show color and `--no-color`.
8. Documentation and final verification.
