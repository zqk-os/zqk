# CEF S5 — System tranche-1 (bootstrap + systemcheck extracts)

**Last Verified:** 2026-08-31


**TRACK:** `BLI-CEF-ARCH-SYSTEM-TRANCHE1` / `REQ-CEF-ARCH-001` / `CRIT-CEF-ARCH-001A`  
**Date:** 2026-08-17

## Intent

Bounded decomposition of the `cmd/zqk/system` God package (F-ARCH-001): move domain logic out of the CLI package into a library home that already owns the concern.

## Tranche-1 extracts

### 1. Bootstrap source-fallback → `internal/bootstrap`

| Before | After |
|--------|--------|
| `cmd/zqk/system/bootstrap_extractor.go` (~372 LOC) | Thin CLI wrapper → `bootstrap.ExtractFiles` |

### 2. Check result types + violation resolver → `pkg/systemcheck`

| Before | After |
|--------|--------|
| `CheckResult` / `Issue` / snapshot metadata in `cmd/zqk/system` | `pkg/systemcheck` types + aliases in CLI package |
| `violation_resolver.go` (~264 LOC) | `pkg/systemcheck/violation_resolver.go` |

### 3. Registration / lifecycle validators → `pkg/systemcheck`

| Before | After |
|--------|--------|
| `checkRegistration` / `checkLifecycle*` in `check_impl_validators.go` | `systemcheck.CheckRegistration` / `CheckLifecycle` / `CheckLifecycleWithLoader` |
| CLI file retains thin wrappers + CLI-coupled instance/reference validators | |

## LOC evidence (`cmd/zqk/system` non-test `*.go`)

| Checkpoint | Non-test LOC |
|------------|--------------|
| S5 start baseline (2026-08-17) | **65741** |
| After bootstrap extract | **65384** (−357) |
| After systemcheck types/resolver | **65101** (−640 vs baseline) |
| After registration/lifecycle validators | **64921** (−820 vs baseline) |

Measure:

```bash
find cmd/zqk/system -name '*.go' ! -name '*_test.go' -print0 | xargs -0 wc -l | tail -1
```

## Acceptance vs CRIT-CEF-ARCH-001A

- **Package extracted:** yes — `internal/bootstrap` + `pkg/systemcheck` (types, resolver, registration/lifecycle validators).
- **LOC reduction measured vs baseline:** yes — table above (−820 LOC).
- Further God-package extracts (object ID cache, audit buffer, check output) remain as follow-on TRACK under F-ARCH-001; tranche-1 CRIT is satisfied by measured extracts above.

## Related

- `docs/architecture/CEF_EVENT_PATH_AND_GLOBALS.md` (sibling S5 BLI, complete)
- `internal/bootstrap/README.md`
- `pkg/systemcheck/doc.go`
