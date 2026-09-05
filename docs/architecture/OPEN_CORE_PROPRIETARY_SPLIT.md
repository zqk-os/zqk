# ZQK Architectural Split: Open-Core vs Commercial Enterprise Tiers

**Last Verified:** 2026-08-31


## Overview
This document formalizes the architectural boundary and distribution separation between the **ZQK Open-Core (Community Edition)** and **ZQK Commercial Enterprise (EE)** modules, satisfying [REDACTED-ID].

## 1. Module & Tier Boundaries

| Component / Module | Tier | License | Distribution Path |
| :--- | :--- | :--- | :--- |
| Core Kernel (`pkg/graph`, `pkg/storage`, `pkg/mcp`, `pkg/objects`, …) | Open-Core | **Apache 2.0** | `github.com/lanceman/zqk` |
| Community CLI (`cmd/zqk-community`, `cmd/zqk`, `cmd/zqk-shim`) | Open-Core | **Apache 2.0** | `bin/zqk-community` (`package-community.sh`) |
| Enterprise Admin / Control Plane (`cmd/zqk-admin`, `pkg/admin`) | Enterprise | **Commercial EE** (not Apache) | Proprietary EE path / future EE repo |
| Neural Swarm Operators (`cmd/zqk-neuron`, `cmd/zqk-muscle`, …) | Enterprise | **Commercial EE** (not Apache) | Proprietary EE path / future EE repo |

Root `LICENSE` is Apache 2.0 for Open-Core. Appendix A carves out Enterprise paths still present in this monorepo.

## 2. Packaging & Exclude Rules
- **Community Binaries:** `make zqk-community` & `./scripts/package-community.sh` compile ONLY open-core CLI command targets (`cmd/zqk-community`).
- **Bootstrap Scrub:** `internal/bootstrap/community_denylist.go` enforces strict exclusion of proprietary commands, AppleScript paste helpers, and internal CVS orchestrate specs during greenfield system initialization.

## 3. Public source vs studio (mandatory before public GitHub)

Binary-only releases are **not** sufficient for developer adoption/trust. Before making Open-Core source public:

1. Keep **this monorepo** as private **Studio** (live `docs/process`, `.zqk/`, agent mesh, `scripts/**`, build-only cmds).
2. Scrub a **local public-candidate fork** first (Go product tree: preferably **0** shell/Python helpers, hard cap **≤2**; omit build-ZQK-only surfaces). Promote that candidate to a **new** public remote only after hygiene gates pass.
3. Follow include/exclude inventories, script budget, Studio continuity map, and G1–G12 in:

**`docs/strategy/open-core/OPEN_CORE_PUBLIC_RELEASE_HYGIENE.md`** (HUMAN ACK’d 2026-07-29)
