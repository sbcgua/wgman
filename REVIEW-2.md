# Review 2: Phases 0-6

Review scope: current Go implementation after phases 0-6, checked against [docs/SPEC.md](docs/SPEC.md), [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md), [PROGRESS.md](PROGRESS.md), and prior findings in [REVIEW-1.md](REVIEW-1.md).

## Findings

### Medium: live ipset create-line name is not validated against the configured set

References: [check.go](check.go:220), [check.go](check.go:240), [parse_ipset.go](parse_ipset.go:19), [check_test.go](check_test.go:325)

Review-1 required strict ipset parsing/checking before deploy could trust deltas. Most of that was implemented: `ParseIPSet` now records set name/type, rejects `add` lines before `create`, rejects duplicate `create` lines, rejects `add` lines for a different set than the parsed `create`, and `check` validates set types and entry shapes.

One gap remains: `validateAllAccessIPSet` and `validateMatrixIPSet` receive the configured set name but only check `parsed.SetType` and entries. They do not check `parsed.SetName == setname`. A fake or unexpected command output like:

```text
create other_set hash:ip family inet
add other_set 10.8.0.5
```

would be accepted while checking `wg_allow_all`, because the parser only verifies internal consistency between `create other_set` and `add other_set`. Real `ipset list <setname>` should normally not return another set, but the review requirement was to validate live set name before trusting deploy plans. Add this hard error and a regression test.

### Low: duplicate access entries in `db.yaml` are not reported

References: [config.go](config.go:128), [validate.go](validate.go:34), [check.go](check.go:150)

`access` lists are validated for known users/VMs and invalid `*` mixing, but duplicate entries such as `alice: [sandbox, sandbox]` are accepted. `computeExpectedIPSets` collapses duplicates into a map, so check/deploy may report a clean state while the intended database still contains redundant access entries.

This is not a deploy safety bug, but it matters before `mod` starts writing `db.yaml`: the command should not preserve or create duplicate access values, and offline validation should preferably reject duplicates so manual edits stay clean.

## Review-1 Follow-Up Status

- Strict ipset parsing/checking: mostly implemented. Remaining gap: configured set name is not compared with the parsed `create` line name.
- Command dependency injection: implemented via `App` with `SystemAdapter`, stdin, stdout, stderr, and clock hook.
- Positional argument validation: implemented for `check`, `show`, `list`, `init-ipsets`, and `deploy`.
- IPv4-only validation: implemented for user and VM IPs through `To4() != nil`.
- Stderr preservation for read commands: implemented for `WGDump`, `IPSetList`, and `WGPubKey` with `CombinedOutput()`.

## Direction Assessment

The implementation direction is sound. The central shape matches the spec and plan: YAML loading is typed and strict, validation/parsing is mostly pure, system access is behind `SystemAdapter`, command-level code is injectable, `check` separates hard errors from ipset drift, and `deploy` applies minimal ipset deltas with dry-run and confirmation support.

I would not start Phase 7 until the medium finding is fixed. It is small, but it sits exactly in the surface area Phase 7 will build on: trusted ipset state.

## Suggested Follow-Up Work Before Phase 7

1. Add parsed set-name validation in `validateAllAccessIPSet` and `validateMatrixIPSet`; include tests for wrong `create` set names.
2. Add duplicate access-entry validation and make the future `mod` writer normalize access lists deterministically.
3. Add an atomic `db.yaml` write helper before `mod`: write to a temp file in the config dir, fsync where practical, then rename. Tests should prove dry-run does not write, failed validation does not write, and successful writes produce deterministic YAML.

## Verification

Executed with Go caches redirected to writable temp directories:

```sh
$env:GOCACHE='C:\Users\at\AppData\Local\Temp\wgman-gocache'
$env:GOMODCACHE='C:\Users\at\AppData\Local\Temp\wgman-gomodcache'
go test ./...
go vet ./...
go build -o $env:TEMP\wgman-review.exe .
gofmt -l .
```

Results:

- `go test ./...` passed.
- `go vet ./...` passed.
- `go build` passed.
- `gofmt -l .` printed no files.
