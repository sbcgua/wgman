# User groups support

Currently access to vms and resources are controlled individually per user. Sometimes it is convenient to repeat access configuration for users of the same profile. The idea is to introduce user groups. Example config mode:

```yaml
users:
  admin: ...
  alice: ...
  bob: ...
user-groups:
  admins:
    - admin
  developers:
    - alice
    - bob
vms:
  sandbox: 192.168.122.190
resources:
  mail@sandbox:
    vm: sandbox
    ports: 25
  web@sandbox:
    vm: sandbox
    ports:
      - 80
      - 443
access:
  admins:               # full access for admin group
    - "*"
  developers:           # resources and vms for a user group
    - mail@sandbox
    - web@sandbox
  alice:                # additional individual access
    - sandbox
```

## Details

- user groups are named lists of users
- empty users groups are valid (useful as a structure placeholder)
- users groups and users can be used in access section interchangeably. Thus, they share same name sapce and must be unique: there may not be user `admin` and user group `admin`
- accesses for groups and users merge: so if user1 belongs to group1, and group1 access resource A and B, and user1 access resource B and C - the resulting access would be A,B,C.

## Impact on commands

- `list`:
  - must add a section with user groups (compact list: `group name: user1,user2 ...`)
  - optional argument that now support only users, should also support groups - listing the accesses for the group
  - user list displays the short access for each user: this must be merged access list (so inividual access for the user + access for all groups he is included in)
- `mod`:
  - must support user groups as an arg in addition to user. So `wgman mod group1 +vm1` adds `vm1` access to the `group1`.
  - `activate/deactivate` are not supported for groups.
- Add a new command `usergroup` e.g. `wgman usergroup <group> [-user1,+user2...]`
  - calls the `check` internally for the state and config validation.
  - refuse to run if `check` detects any hard errors or ipset drift
  - if users list is not given - list the users in the given group. In case groups does not exist - issue a corresponding error message
  - if users list is given:
    - the user list is a list of users prefixed with `+/-` that represent intended adding or removing the user from the user group
    - check if users exist
    - if the groups does not exist - create it (if the validation above passed)
  - update the db
  - update system state (deploy) - update the relevant ipsets

## Other considerations

- In the documentation and code - **prefer** using full "user group" term. Later "resource groups" may also be introduced, so pre-avoid ambiguity.
- check and deploy logic should be concentrated in the `check` and `deploy` files respectively, the rest of the commands should delegate the validation and application of rules to them (as it is now).
- After implementation, update the SPEC.md according to the final design. As well as README.md - which is the user-facing documentation. And sample config templates.

## Interview findings

The following decisions were agreed during planning and should guide
implementation.

### Data model and validation

- `user-groups` is an optional section in `db.yaml`; missing means no user
  groups.
- `db.yaml` writes should emit sections in this order: `users`,
  `user-groups`, `vms`, `resources`, `access`.
- If there are no user groups, deterministic output should emit
  `user-groups: {}`.
- Empty user groups are valid and should be written as `group: []`.
- User group names use the same name validation as users and VMs:
  `^[A-Za-z0-9_-]+$`.
- Users and user groups share one access-principal namespace. Exact and
  case-only conflicts between user names and user group names must be rejected.
- User groups are flat lists of users. Nested user groups are not supported.
- Unknown users in `user-groups` are hard validation errors.
- `access` keys may reference either an existing user or an existing user
  group, and must reject anything else.
- Access entries themselves still reference only `*`, VMs, or resources.
- `*` access is dominant when effective access is merged. If a user receives
  `*` from any direct or inherited source, expected live ipsets should contain
  only the all-access entry for that user and no redundant VM/resource entries.
- The existing rule that `*` cannot be mixed with other access entries applies
  to both user and user group access entries.

### Effective access and output

- User effective access is the merge of direct user access and access from all
  user groups containing that user.
- `wgman list` with no filter displays each user's merged effective access, but
  keeps the compact clean output without inherited-access annotations.
- `wgman list <user>` displays the user's merged effective access.
- In `wgman list <user>`, access received only from user groups should be
  annotated with the contributing user group name in brackets, for example
  `mailserv (mailgroup)`.
- If a user inherits the same access target from multiple user groups, list all
  contributing groups sorted in one bracket, for example
  `mailserv (devs,mailgroup)`.
- If the user also has direct access to the same target, show only the target
  name with no bracket annotation.
- `wgman list <group>` displays the group's direct configured access only, with
  semantics equivalent to the current user-specific listing behavior.
- `wgman list` with no filter must add a user group section using a compact
  format like `group: user1,user2`.

### Command behavior

- `create` and `add` do not manage user group membership. User groups are
  managed through manual YAML edits or the `usergroup` command.
- `remove <user>` should also remove the user from all user groups as part of
  the same DB update.
- `mod <principal> +res,-res` resolves `<principal>` as either a user or a user
  group and updates direct access for that principal.
- `mod <group> activate` and `mod <group> deactivate` are errors.
- `activate` and `deactivate` remain user-only operations.
- The new command name is exactly `usergroup`; no alias is planned for v1.
- `wgman usergroup <group>` with no membership operations is read-only,
  lists the group's users, and requires a clean full `check` result for
  consistency with other commands.
- `wgman usergroup <group> [+user,-user...]` requires a clean full `check`
  result before modifying `db.yaml`.
- Membership operations are comma-separated, use `+` to add users and `-` to
  remove users, and must reference existing users.
- A missing user group is created only when the operation list contains at
  least one `+user`.
- Removing users from a non-existing user group is an error and must not create
  the group.
- `usergroup` supports `--dry-run`; dry-run prints planned DB membership and
  live ipset changes without writing `db.yaml` or applying live changes.
- `usergroup` applies changes like access-only `mod`: write `db.yaml` first,
  then apply ipset deltas.

### Live state and comments

- User group changes alter live state through expected ipset deltas only; they
  do not create or remove WireGuard peers.
- Ipset comments continue to use only the username and access target, for
  example `alice -> mail@sandbox tcp/25`; user group source is not included in
  live firewall comments.

### Documentation and templates

- `docs/SPEC.md`, `README.md`, and sample config templates should be updated
  after implementation to describe the final user group behavior.
- The sample `share/etc/wireguard/wgman/db.yaml` should include a minimal
  `user-groups: {}` section rather than a demonstrative group example.
