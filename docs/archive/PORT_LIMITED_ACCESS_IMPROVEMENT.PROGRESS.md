Implemented `PORT_LIMITED_ACCESS_IMPROVEMENT.md`.

What changed:
- Replaced `sets.matrix` with required `sets.ip_matrix`.
- Added required `sets.port_matrix` using `hash:ip,port,ip`.
- Added `resources` support in `db.yaml`, including `@` resource names, VM/resource namespace validation, port parsing, TCP defaulting, UDP support, and deterministic YAML writes.
- Extended check/deploy/create/remove/mod reconciliation to manage all three ipsets.
- Updated `init-ipsets` to create `all`, `ip_matrix`, and `port_matrix`.
- Updated `list` to show a `Resources:` section.
- Updated firewall hook template with `SET_IP_MATRIX`, `SET_PORT_MATRIX`, and TCP/UDP port-matrix rules.
- Updated `docs/SPEC.md`, `docs/NOTES.md`, `README.md`, bundled sample config/db files, and the improvement doc typo.

Verification:
- `go test ./...` passes.
- `git diff --check` passes.

Linux-specific follow-up to double-check on a real target host: validate the generated `iptables` rules against the installed `ipset`/`iptables` versions, especially `hash:ip,port,ip` with `--match-set "$SET_PORT_MATRIX" src,dst,dst` for both TCP and UDP. Unit tests cover command generation and reconciliation logic, but not kernel-level packet matching.