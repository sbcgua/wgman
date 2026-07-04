# WGMAN

`wgman` is a planned command-line tool for managing WireGuard users and their access to internal VM resources through WireGuard peers and ipset-based firewall rules.

Detailed behavior is specified in [docs/SPEC.md](docs/SPEC.md). The implementation plan is in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

## Development Prerequisites

- Go installed in the development environment.
- Network access during initial setup to download Go module dependencies.
- For real host smoke tests only: Linux with WireGuard tools, `ipset`, firewall rules, and root access.

The main runtime dependency planned for the Go implementation is:

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
- `WG_IFACE`
- `VM_IFACE`
- `SET_ALL` and `SET_MATRIX`
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
PostUp = /usr/local/sbin/wgman-firewall-hook up
PreDown = /usr/local/sbin/wgman-firewall-hook down
```

The script manages only its dedicated `iptables` chains and parent jump rules. It does not create ipsets, populate ipsets, install packages, or configure firewall persistence. Unmatched packets return to the host's existing `INPUT` or `FORWARD` policy. Limited-user host services such as DNS are not enabled by default. The `up` action rebuilds the owned chains, `down` removes them, and `reassert` moves the existing parent jump rules back to the top if another tool inserts higher-priority rules later:

```sh
sudo /usr/local/sbin/wgman-firewall-hook reassert
```

The template is IPv4-only and uses `iptables`/`ipset`. Hosts using nftables or IPv6 should adapt the template manually.

## Basic Commands

Implemented commands at the current stage:

Show help:

```sh
sudo wgman help
```

Check config, WireGuard peers, and ipset state:

```sh
sudo wgman check
```

List users, resources, and optional user access:

```sh
sudo wgman list
sudo wgman list alice
```

Show WireGuard peer status with user names:

```sh
sudo wgman show
```

Create the managed ipsets defined in `config.yaml`:

```sh
sudo wgman init-ipsets
```

Preview access reconciliation:

```sh
sudo wgman deploy --dry-run
```

Apply access reconciliation from `db.yaml` to managed ipsets:

```sh
sudo wgman deploy
```

Skip confirmation prompts where supported:

```sh
sudo wgman deploy --yes
```

Create a user with an auto-assigned IP:

```sh
sudo wgman create alice
```

Create a user with an explicit IP and resource access:

```sh
sudo wgman create alice 10.1.0.10 sandbox,mailvm
```

Modify resource access:

```sh
sudo wgman mod alice +mailvm,-sandbox
```

Remove a user:

```sh
sudo wgman remove alice
```

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
