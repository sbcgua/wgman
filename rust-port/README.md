# wgman-rs

Rust port of `wgman`, a WireGuard user and resource-access manager. The Rust
binary is named `wgman-rs` and keeps the Go tool's default config directory:
`/etc/wireguard/wgman`.

## Build And Check

```sh
make build
make check
```

`make check` runs formatting, clippy with warnings denied, and the normal Rust
test suite. The normal suite uses fake system adapters and does not require
root, WireGuard, ipset, iptables, or live network interfaces.

## Usage

```sh
wgman-rs help
wgman-rs --config-dir /etc/wireguard/wgman check
wgman-rs --config-dir /etc/wireguard/wgman list
wgman-rs --config-dir /etc/wireguard/wgman show
wgman-rs --config-dir /etc/wireguard/wgman deploy --dry-run
```

Mutation commands (`init-ipsets`, `deploy`, `create`/`add`, `remove`, and
`mod`) are intended to run as root on Linux hosts with WireGuard and ipset
installed. The port does not install system packages or firewall rules.
