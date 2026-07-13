# WGMAN - wireguard and resource access manager

## General goal and considerations

The tools is supposed to be a convenient wrapper around wireguard to enable adding/viewing allowed users by name and also control access of those users to specific internal resources (virtual machines) on the server. The problems to be solved:

1) Standard `wg show` command does show only ip address and hashes of the clients - which is not human friendly
2) Adding a new user requires creation a configuration file for him. It can be automated
3) The server have several VMs. There are `accept` firewall rules that control forwarding from the wg0 interface to VMs via ipsets. Full-VM access is represented by a `hash:net,net` ipset that maps source VPN IP to destination VM IP/CIDR. Port-limited access is represented by a `hash:ip,port,ip` ipset that maps source VPN IP, destination protocol/port, and destination VM IP. The tool should give a possibility to control the access in a convenient matrix form. As well as control the consistency after reboots or VPN user deletion. In addition, there is a special FW rule (and ipset) which allows any access for the given VPN client (admin user).

## Configuration files

Configuration files are supposed to live in `/etc/wireguard/wgman`. There are 3 config files:

`config.yaml` - holds global parameters to the program, in particular, such that are not impacted by VPN user changes and VMs changes.

```yaml
  interface: wg0 # managed sireguard interface
  sets:
    all: wg_allow_all # the set that allows access to all resources
    ip_matrix: wg_allow_matrix # the net,net set, that maps client IP to VM ip
    port_matrix: wg_allow_matrix_ports # the ip,port,ip set, that maps client IP to VM service ports
```

`db.yaml` - the database of users, user groups, VMs, port-limited resources, and access matrix. This file will be modified by the `wgman` and may also be modified manually by admin.

- `users` section list users, every one contains `ip` and `pub` (public key) params. The ip is mostly for the human readability of the file. Wgman must user pub keys for its validations. A user may also include optional `comment` metadata and optional `inactive: true`; missing `inactive` means the user is active.
- `user-groups` section is optional and lists named, flat user groups. Missing means no groups. Empty groups are valid. Group members must be existing users.
- `vms` section - list of VM names and the corresponding ip addresses
- `resources` section - optional list of named services. Each resource references a VM and one or more TCP/UDP ports. Unprefixed ports mean TCP. A resource may include optional `comment` metadata.
- The `access` matrix declares VMs or resources accessible to a user or user group. VM entries are added to `sets.ip_matrix`; resource entries are added to `sets.port_matrix`. If the effective access entry = `*`, the user must be added to the `sets.all` (admin). Direct user access and all access from groups containing that user merge; `*` is dominant and suppresses redundant VM/resource entries.

```yaml
  users:
    admin:
      ip: 10.8.0.5
      pub: 1j67823bghdhskfj6734gy324234
    alice:
      ip: 10.8.0.10
      pub: 1j67823bghdhskfj6734gyg4564645
      comment: laptop replacement scheduled
    bob:
      ip: 10.8.0.15
      pub: 1j67823bghdhskfj6734gyg5345645
      inactive: true

  user-groups:
    admins:
      - admin
    developers:
      - alice

  vms:
    sandbox: 192.168.122.100
    mailvm: 192.168.122.101

  resources:
    ssh@sandbox:
      vm: sandbox
      ports: 22
      comment: access to SSH
    dns@mailvm:
      vm: mailvm
      ports:
        - udp:53
        - tcp:53

  access:
    admins:
      - "*"
    developers:
      - ssh@sandbox
    alice:
      - sandbox
    bob:
      - sandbox
      - mailvm
```

`user.conf.template` - is a template of the user VPN confguration file. It holds place holders `$CLIENT_PRIVATE_KEY`, `$CLIENT_VPN_IP`, `$SERVER_PUBLIC_KEY` that should be replaced on user creation, when the specific config is generated.

```ini
[Interface]
PrivateKey = $CLIENT_PRIVATE_KEY
Address = $CLIENT_VPN_IP/32

[Peer]
PublicKey = $SERVER_PUBLIC_KEY
AllowedIPs = 10.1.0.0/24, 192.168.122.0/24
Endpoint = myserver.com:55555
PersistentKeepalive = 20
```

## WGMAN invocation

The tool is supposed to running from root (check it at the beginning, return error it is not so).

The tool is supposed to live in `/usr/local/sbin` - it must be one file executable.

The tool is invoked as `wgman <cmd> [params...]`. Commands description follows. A special `help` (or `-h` flag) command must show the list and short explnation of commands and their params.

Global flags:

- `--yes` - skip interactive confirmation prompts.
- `--dry-run` - show planned changes without applying them. Supported by `deploy`, `remove`, `mod`, and `usergroup`.
- `--flush` - flush managed ipsets. Supported by `init-ipsets`.
- `--destroy` - destroy managed ipsets. Supported by `init-ipsets`.
- `--no-color` - suppress colorized terminal output.

### Check

`wgman check`

- read wg - `wg show <config.interface> dump`
- read config files
  - check duplicate users
  - check duplicate or invalid user groups
  - check duplicate vms
  - check all user ips are in wg interface subnet
  - check access owners are known users or user groups, and VMs/resources defined in access section are present in `vms` or `resources` section
  - check resources reference known VMs and valid TCP/UDP ports
  - other reasonable consistency checks
- check all wg users (hashes) are in file users (hash) and that active users' ips are the same
- check all active file users (hashes) are in wg; if an active user's WireGuard peer is missing, report restorable drift that `deploy` can reconcile
- inactive file users are expected to be absent from WireGuard and managed ipsets; if an inactive user's WireGuard peer or managed ipset entries are still present, report removable drift
- read ip sets defined in `config.sets` - `ipset list <setname> -o save`
- check `all`, `ip_matrix`, and `port_matrix`

For the output of `wg` consult the tool man page. But the idea is that the `dump` command outputs the data in concise machine readable way, where first output line lists, in particular, server public key. And the follwoing lines output users information: publick key, ip, endpoint, traffic ...

```text
4O11873947598347598374985739875935438947583=    pG57639475674957674958674957698475965976948=    55555   off
5N78634586384658734563874875638465876378465=    (none)  175.43.19.18:54444    10.1.0.9/32    1782924438      116250608       317169928       off
```

For the output of `ipset list` consult the tool man page. But the sample is below.

```text
create wg_allow_matrix hash:net,net family inet hashsize 1024 maxelem 65536 comment bucketsize 12 initval 0x12345678
add wg_allow_matrix 10.1.0.5,192.168.122.51 comment "test"
create wg_allow_matrix_ports hash:ip,port,ip family inet hashsize 1024 maxelem 65536 comment bucketsize 12 initval 0x12345678
add wg_allow_matrix_ports 10.1.0.5,tcp:22,192.168.122.51 comment "alice -> ssh@sandbox tcp/22"
```

Also obtain the ip address of the `<config.interface>` - find the best practice. It will be needed to validate user ip addresses, create configs, etc.

If the check is successful - return success message. If not successful - return deviations.

Internally, `check` must be a routine, that detects inconsistancies in the config files and current wg and ipsets confguration. The results may be used to inform the users (like this `check` command), block further processing, or apply the differences to the actual system state. The internal result must distinguish hard errors from ipset drift, and must return prepared ipset deltas that callers can report or apply.

At the end of each command clearly and concisely report the result.

## List

`wgman list [filter]`
`wgman list -r <resource-or-vm>`

- calls the `check` internally for the state and config validation. If fails - return with errors same a `check`.
- list users, their ips, and merged effective access summary in the form `(vm1,vm2)` or `(none)`
- list user groups in compact form, VMs, and resources; each resource shows its referenced VM and ports
- if filter is a user, output that user's merged effective access. Access inherited only from user groups is annotated with the sorted source group names, e.g. `mailvm (developers)` or `mailvm (devs,ops)`.
- if filter is a user group, output direct configured access for that group only.
- if `-r <resource-or-vm>` is provided, output users with effective access to that exact VM/resource target. The target must be an existing VM or resource name, except `*` is also accepted and lists all-access/admin users only. `-r` cannot be combined with the positional user/user-group filter.
- VM/resource filters are exact access-target lookups in the shared VM/resource namespace. For example, `wgman list -r sandbox` lists users with effective `sandbox` or `*` access, but not users with only `ssh@sandbox`; `wgman list -r ssh@sandbox` lists users with effective `ssh@sandbox` or `*`, but not users with only full-VM `sandbox` access.
- `list -r` includes inactive users, using the same `~` suffix and color behavior as the normal user list.
- `list -r` does not list user groups as rows. If a user has access to the requested target through `*` all-access, annotate the user with `(*)` except when the requested target is `*` itself. If a user has access to the requested target only through user group membership, annotate the user with the sorted source group names, e.g. `alice (developers)` or `alice (devs,ops)`. Direct user access to the target, including direct `*`, suppresses the group annotation.
- if a valid `-r` target has no matching users, print `(none)` and succeed.
- when stdout is interactive, `none` access markers are grey and `*` access markers are red; `--no-color` suppresses this

## Show

`wgman show`

This is a convenient representation of `wg show <interface>`, essentially with user names instead of hashes.

- calls the `check` internally for the state and config validation.
- outputs: `username [ip] endpoint in out lasthandshake`
  - inactive users are shown with `~` appended to the username, e.g. `alice~`
  - endpoint without port
  - in/out bytes in human readable format e.g. `2.07Mb`
  - lasthandshake in format like `2d23h48m40s`
- colorizes `RX`, `TX`, and `LAST HANDSHAKE` values when stdout is an interactive terminal, unless `--no-color` is passed
  - inactive usernames are grey and still keep the `~` suffix
  - non-zero traffic unit suffixes (`B`, `Kb`, `Mb`, `Gb`) are dim cyan
  - zero-byte traffic (`0B`) is grey
  - `never` is grey
  - day and minute components are dim cyan; hour and second components remain uncolored
  - redirected output remains plain text with no ANSI escape sequences

## Create user

`wgman create [-c "comment text"] <name> [ip] [res1,res2...]`

- calls the `check` internally for the state and config validation.
- refuse to run if `check` detects any hard errors or ipset drift
- check, if the user is not already created and does not conflict with any user group name
- if the second arg is present it is an IP - use it as client ip. Otherwise, generate ip from the interface ip range (use max available IP among the users + 1, error on failure)
- generate wireguard private and public keys for the new user (check `wg` man page)
- check next arg (after the IP if it was there), it may be a comma separated (no space) list of VMs/resources to add access to. If the list is present, check that all access targets are in the config (error otherwise)
- if `-c "comment text"` is provided, store the trimmed text as the user's optional `comment` metadata in `db.yaml`; reject empty comments
- generate config file: take the template, replace the variables, save as `<user>.vpn.conf` in the current dir
- add to user to the `db.yaml`, add his accesses of they were given (otherwize no new access entries)
- update system state (deploy)
  - `wg set <config.interface> peer <client-pulic-key> allowed-ips <client-ip>`
  - add to corresponding ipsets, if relevant - `ipset add <set> <client-ip>` for admin (`*`) access, `ipset add <ip_matrix> <client-ip>,<vm-ip> comment <comment>` for full VM access, or `ipset add <port_matrix> <client-ip>,<protocol>:<port>,<vm-ip> comment <comment>` for resource access. VM comments use `<username> -> <vmname>`; resource comments use `<username> -> <resource> <protocol>/<port>`.

Internally, the "deploy" part must be coded as a routine, that applies changes to the system state. It can be reused in `remove`, `mod` and `deploy` command.

`create` command should have command line alias - `add`.

## Remove user

`wgman remove <name>`

- calls the `check` internally for the state and config validation.
- refuse to run if `check` detects any hard errors or ipset drift
- check, if the user exists
- issue a warning to confirm if the user XXX (ip), with access to x,y,z must be deleted
- update `db.yaml`: remove user, his access entries, and his membership from all user groups
- update system state (deploy)
  - remove corresponding ipset entries: `ipset del <set> <client-ip>` or `ipset del <set> <client-ip>,<vm-ip>`
  - remove wg entry `wg set <config.interface> peer <client-pulic-key> remove`

Reuse the `deploy` routine to update the state.

`remove` has command line alias - `del`.

## Modify access to resources

`wgman mod <name> <+res1,-res2...|activate|deactivate>`

- calls the `check` internally for the state and config validation.
- refuse to run if `check` detects any hard errors or ipset drift
- check, if the name exists as a user or user group
- read the arg that follows name. If it is `activate` or `deactivate`, the name must be a user; toggle the user's inactive state and apply live WireGuard/ipset changes immediately. User groups cannot be activated or deactivated. Otherwise it must be a list of existing VMs/resources, separated by commas (no space), prefixed by `+` or `-`
- `+/-` represent intended direct access change - add or remove the VM/resource from the user's or user group's access list
- update system state (deploy) - update the relevant ipsets
- `deactivate` writes `inactive: true`, preserves the user record/comment/access list, deletes relevant managed ipset entries, and removes the WireGuard peer
- `activate` omits the `inactive` field from written YAML, preserves the user record/comment/access list, adds the WireGuard peer, and adds relevant managed ipset entries

Reuse the `deploy` routine to update the state.

## User groups

`wgman usergroup <group> [+user1,-user2...]`

- calls the `check` internally for the state and config validation.
- refuse to run if `check` detects any hard errors or ipset drift
- if the user list is not given, list users in the given group. If the group does not exist, fail with a clear error.
- if the user list is given:
  - the user list is comma-separated with no spaces and each user is prefixed with `+` or `-`
  - all referenced users must exist
  - a missing user group is created only when at least one `+user` operation is present
  - removing users from a missing user group is an error
  - update `db.yaml`
  - update system state by applying the relevant effective ipset deltas
- `--dry-run` reports the planned membership and ipset changes without writing `db.yaml` or applying live changes.

## Deploy

- calls the `check` internally for the state and config validation. In case of this command, the difference if supposed to be applied to the system state (the file state supposed to be intended).
- user list is not supposed to be changed. Unknown WireGuard peers and peer IP mismatches are errors, but missing active-user peers are planned for addition and inactive users that still exist live are planned for removal.
- report the planned updates from the deltas returned by internal `check`, confirm with the user
- update system state (deploy) - add missing active users' WireGuard peers where planned, update the relevant ipsets by applying deltas only, then remove inactive users' WireGuard peers where planned

Reuse the `deploy` routine to update the state.

## Interview findings

The following decisions were agreed during planning and should guide implementation.

### State validation and drift

- Internal `check` returns two categories of findings:
  - hard errors: invalid config/schema, duplicate IPs, invalid names, unknown access targets in `access`, invalid resource definitions, users outside the WireGuard interface subnet, WireGuard peer mismatches, missing configured ipsets, and similar issues that make the intended state unsafe or ambiguous;
  - ipset drift: missing or extra entries in the configured access ipsets compared with `db.yaml`.
- `check` reports hard errors and ipset drift. Any finding makes the command fail.
- `create`, `remove`, `mod`, and `usergroup` must refuse to run if `check` reports either hard errors or ipset drift. Direct changes to `db.yaml` should be applied through a clean state.
- `deploy` may run when the only detected problems are ipset drift or restorable WireGuard peer drift for known DB users. It must treat `db.yaml` as the intended access state and reconcile WireGuard peers and configured ipsets to it.
- Internal `check` must prepare concrete ipset deltas so command logic can either report them or apply them.
- Internal `check` must prepare concrete WireGuard peer add deltas for active users missing live and removal deltas for inactive users that still exist live.
- The configured `sets.all`, `sets.ip_matrix`, and `sets.port_matrix` are fully owned by `wgman`. Entries in these sets that are not represented by `db.yaml` are safe for `deploy` to delete. Manual firewall exceptions should use separate ipsets/rules.
- Inactive users remain in `db.yaml` but are excluded from expected live WireGuard peers and managed ipsets.
- Expected ipset state is computed from effective per-user access after expanding user groups.
- `deploy` applies deltas only: add missing active-user WireGuard peers, add missing expected ipset entries, delete unexpected ipset entries, and remove inactive users' live WireGuard peers. It must not flush/rebuild whole ipsets unless a future explicit option is added.

### Key material and generated configs

- `db.yaml` must not store client private keys.
- `create` generates the client private key and writes it only into the generated `<user>.vpn.conf` file in the current directory.
- If the generated client config is lost, recovery is manual or the user must be recreated.

### Create command parsing

- `create <name> [ip] [res1,res2...]` parses the second argument as an IP address if it is valid IPv4 or IPv4 CIDR input.
- `create -c "comment text" <name> [ip] [res1,res2...]` and the `add` alias store the comment in `db.yaml`. Empty or whitespace-only comments are rejected with a usage error.
- If the second argument is not an IP address, parse it as the comma-separated access list and auto-assign the user IP.
- Stored user IPs and WireGuard allowed IPs should be normalized to plain IPv4 addresses without `/32`.

### Confirmation and dry-run behavior

- `remove` and `deploy` prompt by default.
- `create`, `mod`, and `usergroup` do not prompt after validation.
- Global `--yes` skips confirmations for automation.
- Global `--dry-run` is supported by `deploy`, `remove`, `mod`, and `usergroup`; it reports planned changes without applying them. For `deploy`, dry-run includes planned WireGuard peer additions and removals.

### Configuration format and dependencies

- Keep YAML for `config.yaml` and `db.yaml` because readability for manual admin edits is preferred.
- A lightweight, actively maintained, safe YAML parser dependency is acceptable.
- Do not store or support arbitrary YAML object tags. Only plain mappings, lists, strings, and scalar values are expected.
- `config.yaml` requires `sets.all`, `sets.ip_matrix`, and `sets.port_matrix`. The old `sets.matrix` key is not accepted.

### Host and environment assumptions

- Target Linux hosts with WireGuard tools, `iptables`, and `ipset` already installed.
- `wgman` does not install packages.
- `wgman` does not create firewall rules, create ipsets, or configure persistence across reboot in v1.
- `check` verifies that the configured ipsets exist and reports clear errors if they do not.
- `init-ipsets` creates all three managed sets: `hash:ip`, `hash:net,net`, and `hash:ip,port,ip`.

## Init ipsets

`wgman init-ipsets [--flush] [--destroy]`

- with no flags, creates all three managed sets using idempotent creation:
  - `sets.all` as `hash:ip`
  - `sets.ip_matrix` as `hash:net,net`
  - `sets.port_matrix` as `hash:ip,port,ip`
- with `--flush`, flushes all entries from the three managed sets.
- with `--destroy`, destroys the three managed sets.
- with `--flush --destroy`, flushes first, then destroys.
- missing sets during `--flush` or `--destroy` are treated as already clean and do not fail the command.
- destroy errors caused by sets still being referenced by firewall rules are errors; run the firewall hook `down` action before manual destroy.
- this command loads `config.yaml` only and does not run the full `check`, so it can be used during WireGuard startup and teardown.

### Names

- User and VM names must match `^[A-Za-z0-9_-]+$`.
- User group names must match `^[A-Za-z0-9_-]+$`.
- Resource names must match `^[A-Za-z0-9_@-]+$`, allowing names such as `ssh@sandbox`.
- Names are case-sensitive for lookup.
- Creating a user or user group name that differs only by case from an existing user or user group name must be rejected to avoid operator confusion. VMs and resources share one access-target namespace; exact and case-only conflicts between VM and resource names must be rejected.
- These restrictions keep generated filenames such as `<user>.vpn.conf` predictable.

## Unit testing strategy

The implementation should keep system command execution and file I/O behind small boundaries so the critical logic can be tested without requiring root, WireGuard, or ipset on the test host.

Recommended unit test coverage:

- Parse `wg show <interface> dump` output:
  - parse interface/server line and peer lines;
  - normalize peer allowed IPs from `/32` to plain IPv4;
  - handle `(none)` endpoint and absent/latest-handshake fields;
  - reject malformed rows with clear errors.
- Parse `ipset list <setname> -o save` output:
  - parse `hash:net` admin entries and `hash:net,net` matrix entries;
  - parse and validate `hash:ip,port,ip` port-matrix entries;
  - parse entries with and without comments;
  - ignore `create` metadata except for validating expected set type where useful;
  - reject malformed managed entries.
- Validate config and database structures:
  - required keys in `config.yaml` and `db.yaml`;
  - valid user, VM, and resource names;
  - valid user group names and known group members;
  - duplicate/case-conflicting names and duplicate IPs;
  - access owners referencing unknown users/user groups;
  - access entries referencing unknown VMs/resources;
  - resources referencing known VMs and valid unique ports;
  - `*` access handled only as all-access/admin.
- Check routine:
  - clean state returns no hard errors and no deltas;
  - config-only problems return hard errors;
  - WireGuard peer IP mismatch returns hard errors;
  - missing active peers and present inactive peers return restorable peer deltas;
  - missing/extra ipset entries return drift deltas;
  - hard errors and drift are clearly separated for callers.
- Delta/deploy planning:
  - compute expected ipset state from `db.yaml`;
  - expand user group access into effective per-user access;
  - generate port-matrix entries for resource access;
  - generate minimal add/delete deltas;
  - treat configured ipsets as fully owned by `wgman`;
  - ensure deploy applies deltas only, without flush/rebuild behavior.
- Command-level decision logic:
  - `create`, `remove`, and `mod` refuse to proceed on hard errors or ipset drift;
  - `deploy` proceeds only when there are no hard errors and applies/report deltas;
  - `--dry-run` reports planned changes without executing system updates;
  - `--yes` bypasses confirmation prompts only where prompts exist.
- User creation helpers:
  - optional IP/access argument parsing;
  - auto-IP allocation from the interface subnet;
  - template substitution for generated client config;
  - no private key is written to `db.yaml`.

Tests should prefer table-driven cases with small fixture strings for external command output. Integration tests against real `wg`, `ipset`, and root-only behavior are optional and should not be required for the normal test suite.

## Postimplementation Improvement session #1

1. user record in the DB may have optional `comment` field. it can be either edited in yaml by the user, or added during the `create` command with `-c "comment text"` flag
2. user record may have optional flag `inactive` (boolean). If `true`: the deploy should remove the user from wg and the relevant ipsets. Yet the user record itself stays.
3. There also should be an command line option to toggle inactive state. `wgman mod <user> activate/deactivate`. It will not conflict with adding vms, as those always start from `+` or `-`. The `activate` should remove the `inactive` flag at all for readability. The command applies the changes immediately.
4. some eyecandy stuff: let's add colors to `wgman show` to latest handshake field. If the field = `never`, color it in grey. If there is time, then days and minutes should be colored in some relatively dim yet distinguishable color (cyan?). The seconds and hours should stay as is. The color should be suppressable with `--no-color` option

## Postimplementation Improvement #2 - Port-Limited VM Access

See [PORT_LIMITED_ACCESS_IMPROVEMENT.md](./archive/PORT_LIMITED_ACCESS_IMPROVEMENT.md). Also integrated in the text above.

## Postimplementation Improvement session #3

- Colors in `list` command (Respect the --no-color flag):
  - format user names similar to `show`, which is: add `~` for inactive ones and color them grey.
  - color `tcp:` and `udp:` prefixes in pink and cyan respectively and Group tcp and udp ports together.
  - color VMs in resources in the user output in different colors (yellow and blue)
- `cmd_check`: color status of final messages OK and FAILED in green and red respectively. Respect --no-color flag.
- `wgman-firewall-hook`: extract interface and sets from yaml by default

## Postimplementation Improvement #4 - User groups support

- See [USER-GROUPS.md](./archive/USER-GROUPS.md). Also integrated in the text above.
- added `del` alias to `remove` command
- added --flush and --destroy features to `init-ipsets`
