# WGMAN

`wgman` is a command-line tool for managing WireGuard users and their access to internal VM resources through WireGuard peers and ipset-based firewall rules.

**Briefly**: the tool allows controlling named WireGuard users and their access to internal resources based on a simple YAML config as well as list current WireGuard status in a friendlier form (e.g. with user names and access lists). See the available commands below. Detailed behavior is specified in [docs/SPEC.md](docs/SPEC.md).

```yaml
  users:
    admin:
      ip: 10.8.0.5
      pub: ADMIN_PUB_KEY...
    alice:
      ip: 10.8.0.10
      pub: ALICE_PUB_KEY...
      comment: laptop replacement scheduled
    bob:
      ip: 10.8.0.15
      pub: BOB_PUB_KEY...
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

## Development Prerequisites

- Go installed in the development environment.
- Network access during initial setup to download Go module dependencies.
- For real host smoke tests only: Linux with WireGuard tools, `ipset`, firewall rules, and root access.

The only external Go module dependency is:

```sh
go get gopkg.in/yaml.v3@v3.0.1
go mod tidy
```

## Build

Development build:

```sh
make build
```

Release-style build:

```sh
go build -trimpath -ldflags="-s -w" -o wgman .
```

The result should be a single executable file named `wgman`.

## Run Unit Tests

Run the normal test suite:

```sh
make test
```

Format and vet before handing off changes:

```sh
make check
```

If `staticcheck` is installed:

```sh
staticcheck ./...
```

Unit tests should not require WireGuard, `ipset`, firewall tools, or root access. System interactions should be tested through fake adapters.

## Install On Target System

Build the binary, then install it on the target Linux host:

```sh
make build
sudo make install
```

This installs:

- `/usr/local/sbin/wgman`
- template config files in `/etc/wireguard/wgman`

Existing config files are not overwritten. Template sources live in [share/etc/wireguard/wgman](share/etc/wireguard/wgman).

See [docs/SPEC.md](docs/SPEC.md) for the expected file structure.

## WireGuard Firewall Hook Template

The repository includes an optional `iptables` hook template at [share/usr/local/sbin/wgman-firewall-hook.template](share/usr/local/sbin/wgman-firewall-hook.template). It is not installed by `make install`.

Review and edit the variables at the top of the template before installing it, especially:

- `IPTABLES` and `IPSET`
- `CONFIG_FILE`, if your `config.yaml` is not in `/etc/wireguard/wgman/config.yaml`
- `WG_IFACE`, if you want to override the interface from `config.yaml`
- `VM_IFACE`; use an `iptables` interface wildcard such as `virbr+` if the
  same rules should apply to several VM bridges like `virbr0` and `virbr1`
- `SET_ALL`, `SET_IP_MATRIX`, and `SET_PORT_MATRIX`, if you want to override the managed set names from `config.yaml`
- `CHAIN_INP` and `CHAIN_FWD`

Install it manually after review:

```sh
sudo install -m 0755 share/usr/local/sbin/wgman-firewall-hook.template /usr/local/sbin/wgman-firewall-hook
```

Before enabling the WireGuard hooks, create and populate the managed ipsets:

```sh
sudo wgman init-ipsets
sudo wgman deploy --dry-run
sudo wgman deploy
```

Then add the hook commands to the WireGuard interface config:

```ini
PostUp = /usr/local/sbin/wgman init-ipsets && /usr/local/sbin/wgman-firewall-hook up && /usr/local/sbin/wgman deploy --yes
PreDown = /usr/local/sbin/wgman init-ipsets --flush && /usr/local/sbin/wgman-firewall-hook down
```

The script manages only its dedicated `iptables` chains and parent jump rules. It allows full-VM TCP/ICMP forwarding through the IP matrix set, and TCP/UDP service forwarding through the port matrix set. Resource entries do not grant ICMP. It does not create ipsets, populate ipsets, install packages, or configure firewall persistence. Unmatched packets return to the host's existing `INPUT` or `FORWARD` policy. Limited-user host services such as DNS are not enabled by default. The `up` action rebuilds the owned chains, `down` removes them, and `reassert` moves the existing parent jump rules back to the top if another tool inserts higher-priority rules later:

```sh
sudo /usr/local/sbin/wgman-firewall-hook reassert
```

To inspect the effective hook variables after reading `config.yaml`:

```sh
/usr/local/sbin/wgman-firewall-hook print
```

The template is IPv4-only and uses `iptables`/`ipset`. Hosts using nftables or IPv6 should adapt the template manually.

## Basic Commands

Implemented commands:

Show help:

```sh
sudo wgman help
```

Check config, WireGuard peers, and ipset state consistency:

```sh
sudo wgman check
```

List users, user groups, resources, and optional user or user group access:

```sh
sudo wgman list
sudo wgman list alice
sudo wgman list developers
# Suppress interactive color output:
sudo wgman list --no-color
```

Show WireGuard peer status with user names:

```sh
sudo wgman show
# Suppress interactive color output:
sudo wgman show --no-color
```

```text
NAME       IP          ENDPOINT        RX        TX        LAST HANDSHAKE
admin      10.8.0.5    180.90.91.48    117.69Mb  320.32Mb  1m35s
alice      10.8.0.10   145.80.12.11    1.35Mb    6.88Mb    6h5m24s
bob~       10.8.0.15   -               -         -         -
```

Create the managed ipsets defined in `config.yaml`:

```sh
sudo wgman init-ipsets
```

Flush managed ipset contents, useful in WireGuard `PreDown` hooks:

```sh
sudo wgman init-ipsets --flush
```

Destroy managed ipsets manually after firewall rules are removed:

```sh
sudo wgman-firewall-hook down
sudo wgman init-ipsets --destroy
```

`--flush --destroy` flushes first, then destroys. Missing sets during flush or
destroy are treated as already-clean state.

The config must define all three managed sets:

```yaml
interface: wg0
sets:
  all: wg_allow_all
  ip_matrix: wg_allow_matrix
  port_matrix: wg_allow_matrix_ports
```

Preview access reconciliation:

```sh
sudo wgman deploy --dry-run
```

Apply reconciliation from `db.yaml` to WireGuard peers and managed ipsets:

```sh
sudo wgman deploy
```

Skip confirmation prompts where supported:

```sh
sudo wgman deploy --yes
```

`--dry-run` is supported by `deploy`, `remove`, `mod`, and `usergroup`. It is
rejected for commands such as `create` and `init-ipsets` so planned-change
output is not confused with real changes.

Create a user with an auto-assigned IP:

```sh
sudo wgman create alice
# Equivalent alias:
sudo wgman add alice
```

Create a user with a stored operator comment:

```sh
sudo wgman create -c "laptop replacement scheduled" alice
```

Create a user with an explicit IP and full-VM access:

```sh
sudo wgman create alice 10.1.0.10 sandbox,mailvm
```

Create or modify access to a port-limited resource:

```yaml
resources:
  ssh@sandbox:
    vm: sandbox
    ports: 22
  web@sandbox:
    vm: sandbox
    ports: [80, 443, tcp:8080]
  dns@mailvm:
    vm: mailvm
    ports: udp:53
```

```sh
sudo wgman create alice ssh@sandbox
sudo wgman mod alice +web@sandbox,-ssh@sandbox
```

Modify resource access:

```sh
sudo wgman mod alice +mailvm,-sandbox
sudo wgman mod developers +mailvm
```

Manage user group membership:

```sh
sudo wgman usergroup developers
sudo wgman usergroup developers +alice,-bob
sudo wgman usergroup --dry-run developers +alice
```

If user group does not exist it is created.

User group access and direct user access merge for each user. In `wgman list
alice`, access inherited only from user groups is annotated with the group name,
for example `mailvm (developers)`.

Deactivate or reactivate a user without deleting their DB record:

```sh
sudo wgman mod alice deactivate
sudo wgman mod alice activate
```

Remove a user:

```sh
sudo wgman remove alice
# Equivalent alias:
sudo wgman del alice
```

Removing a user also removes that user from all user groups.

Use an alternate config directory for testing or staging:

```sh
sudo wgman --config-dir ./fixtures check
```

## Real Host Smoke Test

Use a non-production Linux host or VM first.

```sh
sudo wgman init-ipsets
sudo wgman check
sudo wgman list
sudo wgman show
sudo wgman deploy --dry-run
```

Only after the dry run looks correct, test a controlled `deploy`, `mod`, or `create` operation.

Do not run initial smoke tests against production firewall state.
