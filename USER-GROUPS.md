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
