# WGMAN

`wgman` is a planned command-line tool for managing WireGuard users and their access to internal VM resources through WireGuard peers and ipset-based firewall rules.

Detailed behavior is specified in [docs/SPEC.md](docs/SPEC.md). The implementation plan is in [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md).

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
go build -o wgman .
```

Release-style build:

```sh
go build -trimpath -ldflags="-s -w" -o wgman .
```

The result should be a single executable file named `wgman`.

## Run Unit Tests

Run the normal test suite:

```sh
go test ./...
```

Format and vet before handing off changes:

```sh
gofmt -w .
go test ./...
go vet ./...
```

If `staticcheck` is installed:

```sh
staticcheck ./...
```

Unit tests should not require WireGuard, `ipset`, firewall tools, or root access. System interactions should be tested through fake adapters.

## Install On Target System

Build the binary, then install it on the target Linux host:

```sh
go build -trimpath -ldflags="-s -w" -o wgman .
sudo install -m 0755 wgman /usr/local/sbin/wgman
```

Create the configuration directory:

```sh
sudo mkdir -p /etc/wireguard/wgman
```

Place these files in `/etc/wireguard/wgman`:

- `config.yaml`
- `db.yaml`
- `user.conf.template`

See [docs/SPEC.md](docs/SPEC.md) for the expected file structure.

## Basic Commands

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

Preview access reconciliation:

```sh
sudo wgman deploy --dry-run
```

Apply access reconciliation from `db.yaml` to managed ipsets:

```sh
sudo wgman deploy
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

Skip confirmation prompts where supported:

```sh
sudo wgman deploy --yes
```

Use an alternate config directory for testing or staging:

```sh
sudo wgman --config-dir ./fixtures check
```

## Real Host Smoke Test

Use a non-production Linux host or VM first.

```sh
sudo wgman check
sudo wgman list
sudo wgman show
sudo wgman deploy --dry-run
```

Only after the dry run looks correct, test a controlled `deploy`, `mod`, or `create` operation.

Do not run initial smoke tests against production firewall state.
