# CRIT-DATACELL-001 — Strict nucleus boundary

**Last Verified:** 2026-08-31


**Criterion:** **`CRIT-DATACELL-001`** — *Data cell exposes coordinator membrane API only* (process object).

**Strict reading (no shortcuts):** Code outside the datacell **nucleus implementation** must not resolve CAS `.zqk/process/` paths or `stream_current/<kind>` layout by calling low-level helpers **[`CASEntityPrimaryDir`](../../pkg/datacell/paths.go)** or **[`StreamCurrentKindDir`](../../pkg/datacell/paths.go)** on arbitrary call sites. Those functions remain **internal** to `pkg/datacell` (see [`membrane_read_path.go`](../../pkg/datacell/membrane_read_path.go)).

## Required API for external packages

| Need | Use |
|------|-----|
| CAS entity directory under `.zqk/process/` | **`datacell.CellCASPrimaryDir(projectRoot, segment)`** or **`datacell.CASEntityMembraneReadPaths(projectRoot).CASEntityPrimaryDir(segment)`** |
| Stream overlay directory `stream_current/<kind>` | **`datacell.CellStreamOverlayKindDir(projectRoot, kind)`** or **`datacell.StreamMembraneReadPaths(projectRoot).StreamCurrentKindDir(kind)`** |

**Nucleus / maintenance:** `pkg/storage` and scheduler envelope handlers may still perform low-level I/O, but **path strings** for CAS segments and stream overlays must be obtained via the membrane adapters above (or shared helpers built on them) so layout stays consistent with **`zqk system data-cells` (PRUNED)** / [`DATA_CELL_MODEL.md`](./DATA_CELL_MODEL.md).

## Enforcement

From repo root:

```sh
./scripts/check-crit-datacell-001-boundary.sh
```

Fails if forbidden direct calls appear outside the allowlisted implementation files.
