# Review 3: Phases 0-9

Review scope: current Go implementation after Phase 9, checked against [docs/SPEC.md](../SPEC.md), [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md), [PROGRESS.md](PROGRESS.md), and prior findings in [REVIEW-2.md](REVIEW-2.md).

## Findings

### High: `create` and `remove` can leave hard WireGuard drift after partial failures

References: [cmd_create.go](../../cmd_create.go:172), [cmd_create.go](../../cmd_create.go:178), [cmd_create.go](../../cmd_create.go:182), [cmd_remove.go](../../cmd_remove.go:73), [cmd_remove.go](../../cmd_remove.go:78), [cmd_remove.go](../../cmd_remove.go:82), [cmd_deploy.go](../../cmd_deploy.go:67)

Both commands write `db.yaml` before all live system operations complete. The comments say later failures are visible as drift and can be reconciled with `deploy`, but that is only true for ipset-only mismatches. WireGuard peer mismatches are hard errors, and `deploy` refuses hard errors.

Examples:

- `create` writes the new user to `db.yaml`, then client config writing or `WGSetPeer` can fail. The next `check` sees a db user missing from WireGuard peers as a hard error, and `deploy` will not add the peer.
- `remove` deletes the user from `db.yaml`, then `IPSetDel` or `WGDelPeer` can fail. The next `check` sees an extra WireGuard peer as a hard error, and `deploy` will not remove the peer.

Before real-host use, decide and implement a recovery policy. Good options are compensating rollback on any post-DB failure, or a dedicated repair/reconcile path for WireGuard peers. Add failure-injection tests for `WGSetPeer`, `WGDelPeer`, and client config write failures.

### Medium: `--dry-run` is accepted by `create` but ignored

References: [cli.go](../../cli.go:26), [cli.go](../../cli.go:80), [cmd_create.go](../../cmd_create.go:93), [docs/SPEC.md](../SPEC.md:81)

The spec says `--dry-run` is supported by `deploy`, `remove`, and `mod`, not `create`. Because the flag is parsed globally, `wgman create --dry-run alice` currently proceeds with a real create. That is a footgun: the global help says dry-run shows planned changes without applying them.

For Phase 10, either reject `--dry-run` on unsupported commands with exit code 2, or implement dry-run for `create`. Rejection is simpler and aligns with the current spec.

### Low: README still marks implemented commands as not implemented

Reference: [README.md](../../README.md:169)

The README still says `create`, `mod`, and `remove` are "planned by the spec but not implemented yet". Phase 9 has implemented all three. This belongs in Phase 10 documentation alignment.

### Low: `SaveDBAtomic` is atomic but not durable

Reference: [config.go](../../config.go:156)

`SaveDBAtomic` writes a same-directory temp file and renames it over `db.yaml`, which is the right basic atomic pattern. It does not `fsync` the file or directory before/after rename. That is probably acceptable for now, but Review-2 explicitly said "fsync where practical". If the tool is intended for real host administration, add file sync and best-effort directory sync on Unix targets, with tests kept platform-tolerant.

## Review-2 Follow-Up Status

- Parsed ipset set-name validation: implemented in `validateAllAccessIPSet` and `validateMatrixIPSet`, with regression tests for wrong `create` set names.
- Duplicate access entry validation: implemented in `LoadDB`, with config test coverage.
- Atomic DB writer: implemented as `SaveDBAtomic`, with deterministic YAML output tests. It still lacks fsync durability, noted above as a low-priority hardening item.

## Direction Assessment

The implementation is broadly on track. `mod`, `create`, and `remove` follow the spec shape: they call `Check`, refuse dirty state, plan changes before writing, use the shared ipset delta machinery, keep private keys out of `db.yaml`, and have focused tests.

The main remaining risk is operational recovery after partial write-command failures. That should be addressed before Phase 11 real-host smoke checks, and preferably before Phase 10 polish declares the CLI coherent.

## Suggested Follow-Up Work Before Phase 10 Completion

1. Add failure-injection support to `fakeSystem` for `WGSetPeer` and `WGDelPeer`, plus a test hook for client config write failure.
2. Implement a consistency-preserving failure policy for `create` and `remove`; do not leave the normal recovery path blocked by WireGuard hard errors.
3. Reject unsupported global flags per command, especially `create --dry-run`, or implement `create --dry-run` explicitly.
4. Update README command status now that `create`, `mod`, and `remove` are implemented.
5. Consider adding fsyncs to `SaveDBAtomic` as a final hardening step.

## Verification

Executed with Go caches redirected to writable temp directories:

```powershell
$env:GOCACHE='C:\Users\at\AppData\Local\Temp\wgman-gocache'
$env:GOMODCACHE='C:\Users\at\AppData\Local\Temp\wgman-gomodcache'
go test ./...
go vet ./...
go build -o $env:TEMP\wgman-review3.exe .
gofmt -l .
bash -n share/usr/local/sbin/wgman-firewall-hook.template
```

Results:

- `go test ./...` passed.
- `go vet ./...` passed.
- `go build` passed.
- `gofmt -l .` printed no files.
- `bash -n` on the firewall hook template passed.
