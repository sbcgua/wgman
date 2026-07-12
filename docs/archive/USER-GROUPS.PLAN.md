# User Groups Implementation Plan

This plan is a resumable execution artifact for implementing `USER-GROUPS.md`.
Do not start implementation until the user explicitly confirms.

## Scope

Implement user group support for `wgman` while preserving the existing drift
policy, command boundaries, deterministic YAML writes, and testability
conventions documented in `docs/NOTES.md`.

## Agreed behavior

- `user-groups` is optional in `db.yaml`; missing means no user groups.
- User groups are flat lists of users. Unknown users are validation errors.
- Users and user groups share one access-principal namespace; exact and
  case-only conflicts are rejected.
- `access` keys may reference a user or a user group.
- Effective user access is direct user access plus all access from user groups
  containing that user.
- `*` is dominant in merged effective access.
- `create` does not manage user groups.
- `remove <user>` removes the user from all user groups.
- `mod <principal> +res,-res` works for users and user groups.
- `activate` and `deactivate` remain user-only.
- Add exactly one new command: `usergroup`.
- `usergroup` supports `--dry-run`.
- `list <user>` annotates group-only inherited entries with group names.
- General `list` output stays compact and unannotated.
- `list <group>` shows direct group access only.
- Ipset comments remain username/access-target based and do not include user
  group provenance.

## Execution steps

1. Extend the DB model.
   - Add `UserGroups map[string][]string yaml:"user-groups"` to `DB`.
   - Update cloning to deep-copy user group slices.
   - Ensure nil user groups are treated as empty where needed.

2. Update DB validation.
   - Validate user group names with `nameRe`.
   - Reject exact and case-only conflicts between users and user groups.
   - Validate all user group members exist in `users`.
   - Decide whether duplicate users inside one group are rejected; recommended:
     reject duplicates for deterministic admin feedback.
   - Validate `access` owners as users or user groups.
   - Keep `*` sole-entry validation for both users and user groups.

3. Update deterministic DB writes.
   - Emit sections in order: `users`, `user-groups`, `vms`, `resources`,
     `access`.
   - Emit `user-groups: {}` when empty.
   - Emit empty groups as `group: []`.
   - Add a blank line between `user-groups` and `vms`.

4. Add effective access helpers.
   - Compute direct access for a user.
   - Compute user group memberships for a user.
   - Compute merged effective access with `*` dominance.
   - Compute group provenance for `list <user>` annotations.
   - Keep helpers deterministic and testable without system calls.

5. Update check/deploy planning.
   - Change `computeExpectedIPSets` to iterate over active users and use
     effective access.
   - Preserve existing comments: `<username> -> <target>` and
     `<username> -> <resource> <protocol>/<port>`.
   - Ensure inactive users remain excluded from expected WireGuard peers and
     managed ipsets even if they belong to user groups.
   - Keep configured ipsets fully owned and continue producing deltas only.

6. Update `list`.
   - No filter: print users with merged effective access, then a compact user
     group section, then VMs/resources.
   - User filter: print merged effective access; annotate group-only inherited
     entries with sorted group names in brackets.
   - Group filter: print the user group's direct configured access only.
   - Resolve ambiguity through validation, not command-time guesswork.
   - Preserve color behavior where it already exists; avoid adding unnecessary
     new color semantics unless needed for existing access item coloring.

7. Update `mod`.
   - Resolve the target as a principal: user or user group.
   - Access edits work for either principal.
   - `activate`/`deactivate` must reject user groups.
   - Deltas come from old vs new effective expected ipsets.
   - Preserve `--dry-run` and current DB-first access-edit behavior.

8. Add `usergroup` command.
   - Syntax: `wgman usergroup <group> [+user,-user...]`.
   - With no operations: require full clean `check`, then list users in group.
   - With operations: require full clean `check`, validate all users exist,
     create missing group only if at least one add operation exists.
   - Removing from a missing group is an error.
   - Write `db.yaml`, then apply effective ipset deltas.
   - Support `--dry-run` with planned DB and ipset changes.
   - Add command to help text and CLI dispatch.

9. Update `remove`.
   - Delete the user.
   - Delete the user's direct access entry.
   - Remove the user from every user group.
   - Preserve empty user groups.
   - Compute ipset deltas from old vs new effective expected state.
   - Keep WireGuard peer removal behavior unchanged.

10. Check `create`.
    - Reject new user names that conflict with existing user group names.
    - Do not add the new user to any user group.
    - Keep generated config and private key behavior unchanged.

11. Add and update tests.
    - DB load/validation tests for user groups, conflicts, unknown members, and
      access owned by groups.
    - Deterministic save tests for `user-groups`.
    - Effective access tests for direct, inherited, duplicate, multi-group, and
      `*` dominance cases.
    - Check/deploy tests proving expected ipsets are based on effective access.
    - List tests for compact output, user annotations, and group filters.
    - Mod tests for group access edits and group toggle rejection.
    - Usergroup command tests for listing, add/remove, missing group behavior,
      dry-run, and live delta application.
    - Remove tests proving user membership is cleaned from groups.
    - Create tests proving user/group name conflicts are rejected.

12. Update documentation and sample files.
    - Update `docs/SPEC.md` with final user group behavior.
    - Update `README.md` user-facing command docs and examples.
    - Update `share/etc/wireguard/wgman/db.yaml` with `user-groups: {}`.
    - Update `testdata/valid-offline/db.yaml` if deterministic structure
      expects the new section.

13. Run validation.
    - Run Go tests with a writable `GOCACHE` under the workspace or temp dir.
    - Run formatting.
    - Run `git diff --check`.
    - Because the development shell may be Windows but the project targets
      Linux, double-check any shell/script changes for Linux paths and command
      behavior before finishing.

## Implementation guardrails

- Keep external system interaction behind `SystemAdapter`.
- Do not introduce root, WireGuard, or ipset requirements into unit tests.
- Prefer table-driven tests and small fixtures.
- Use `apply_patch` for source edits.
- Do not change firewall hook behavior unless user group support requires it;
  expected current scope does not.
- Preserve existing drift policy: `create`, `remove`, `mod`, `list`, `show`,
  and `usergroup` require clean state; `deploy` may reconcile safe drift.
