# WGMAN — Handoff Document

**Date:** 2026-07-01  
**Status:** Phases 0 and 1 complete; `go test ./...` passes; binary builds and runs.

---

## What Has Been Done

### Phase 0 — Bootstrap (complete)

- Go module initialised (`module wgman`, Go 1.26.4).
- `gopkg.in/yaml.v3 v3.0.1` added as the only external dependency.
- Source files created:
  - [main.go](main.go) — `main()` delegates to `run(os.Args[1:])`.
  - [cli.go](cli.go) — command routing; `help`/`-h`/no-args prints usage; `check` dispatched; all other commands return "unknown command".
  - [model.go](model.go) — `Config`, `ConfigSets`, `DB`, `UserEntry`, `IpsetDeltaOp`, `CheckResult` types (with `OK()` and `Clean()` helpers).
  - [system.go](system.go) — `SystemAdapter` interface (all external command and OS calls).
  - [validate.go](validate.go) — `ValidateOffline(cfg, db)` pure function.
  - [config.go](config.go) — `LoadConfig`, `LoadDB` with `yaml.Decoder.KnownFields(true)`; all structural validation.
- Test scaffolding in [testhelpers_test.go](testhelpers_test.go): `testHelper`, `makeTempDir`, `writeFile`, `assertNoError`, `assertError`, `fakeSystem` (implements `SystemAdapter`).

### Phase 1 — Config and Database Validation (complete)

`LoadConfig` / `LoadDB` enforce:
- Required fields (`interface`, `sets.all`, `sets.matrix`; user `ip`/`pub`).
- `yaml.Decoder.KnownFields(true)` — unknown YAML fields are rejected.
- Name pattern `^[A-Za-z0-9_-]+$` for users and VMs.
- Case-conflicting names rejected (fold check).
- Duplicate IPs and duplicate public keys rejected.
- `access` entries validated against known users and VMs.

`ValidateOffline` additionally checks:
- User and VM IP values are valid IPv4 addresses.
- `"*"` in an access list must be the sole entry (no mixing with VM names).

`wgman check --config-dir <dir>` is wired up and reports:
- Hard errors if validation fails (exit 1).
- "offline validation passed" message if clean (exit 0; live WireGuard/ipset checks not yet implemented).

Testdata fixture at [testdata/valid-offline/](testdata/valid-offline/) (config.yaml + db.yaml).

Test files:
- [config_test.go](config_test.go) — ~20 table-driven cases covering the full validation matrix.
- [cli_test.go](cli_test.go) — CLI smoke tests including `check --config-dir testdata/valid-offline`.

---

## What Comes Next

Proceed from **Phase 2** in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

**Phase 2 — Parse Live System Output:**
- Pure parsers for `wg show <iface> dump` output → `WGDump` struct.
- Pure parsers for `ipset list <setname> -o save` output → ipset entry slices.
- Interface address/subnet helper.
- Real `SystemAdapter` implementation in `system.go` using `exec.Command`.
- All parsers tested with inline fixture strings (no WireGuard/ipset required).

**Phase 3 — Full `wgman check`:**
- Wire live system calls into the check path.
- Compare live WireGuard peers against `db.yaml` (by public key and IP).
- Compare live ipset entries against expected state derived from `db.yaml`.
- Populate `CheckResult.Drift` and `CheckResult.Deltas`.
- User-facing output and exit codes.

Subsequent phases (4–10) are fully described in [IMPLEMENTATION_PLAN.v2.md](IMPLEMENTATION_PLAN.v2.md).

---

## Key Conventions Established

| Topic | Decision |
|---|---|
| Config dir default | `/etc/wireguard/wgman` (`defaultConfigDir` in `config.go`) |
| Name regex | `^[A-Za-z0-9_-]+$` (`nameRe` in `config.go`) |
| Case folding | simple ASCII byte-shift (`caseFold` in `config.go`) |
| Unknown YAML fields | rejected via `KnownFields(true)` |
| `"*"` in access | admin/all-access; must be sole entry |
| IPs in db.yaml | plain IPv4, no `/32` suffix |
| Private keys | never written to `db.yaml` |
| System calls | only via `SystemAdapter`; `exec.Command` lives in `system.go` |
| Test fakes | `fakeSystem` in `testhelpers_test.go` |
| Testdata | `testdata/valid-offline/` |

---

## Suggested Skills

- **`grilling`** — use if new design questions arise before implementing Phases 2–3 (e.g. parser error formats, subnet detection command).
- **`handoff`** — use again if work needs to be paused after further phases.
