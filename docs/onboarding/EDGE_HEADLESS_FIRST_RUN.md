# Edge & Headless First-Run Guide

**Audience:** Operators on remote servers, appliances, DGX/GPU clusters, SSH-only hosts, or local SLM runtimes without desktop IDE agents.  
**Related Guides:** [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md), [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md).

## Goal

Treat ZQK as the **control plane**: init → seat → thin `AGENTS.md` → sync report. Do **not** write IDE rule forests (`.cursorrules`, …) unless you pass `--all-vendors`. Do **not** invent organ binaries (`zqk-neuron`, …) into the community default seed until market probes close.

## Sequence

```bash
# After system init on the appliance / host
zqk system agent-onboard --headless --format json

# Or auto Vector B when no IDE markers are present:
zqk system agent-onboard --format json
```

| Stage | Vector B behavior |
|-------|-------------------|
| **detect** | Reports IDE markers (usually empty) + **edge_signals** (`SPECIALIZATION_TIER`, NVIDIA/DGX/SSH hints, optional `.zqk/agent-runtime/headless_edge.json`) + open **market_probe** questions |
| **seat** | Same default seating pack as community |
| **prime_workspace** | **Only** `.agents/AGENTS.md` (unless `--all-vendors`) |
| **prime_kernel** | `.zqk/agent-runtime/agent_workspace_sync.json` |
| **smoke** | AGENTS.md + sync report |

Optional marker (operator-declared, not required):

```bash
mkdir -p .zqk/agent-runtime
echo '{"schema":"zqk_headless_edge_v1","note":"appliance control plane"}' > .zqk/agent-runtime/headless_edge.json
```

Specialization (when meaningful on the node):

```bash
export ZQK_SPECIALIZATION_TIER=neuron   # or muscle|heart|lung — see pkg/specialization
```

## Market probes (open until evidence)

Returned under `market_probe_open` in `agent-onboard` JSON. Canonical list also in the strategy doc. **Do not** ship organ-named community binaries until human ACK on these signals (`BLI-AGENT-ONBOARD-VEC-B-001`).

## Wake / interrupt

Prefer scheduler ticks and feed/`delivery_mode=notify` — not Terminal paste. See mesh wake policies when multi-agent seats exist.

## After Setup

1. `zqk workflow whats-next --format json`
2. [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md) — First-run setup, CLI tools, and kernel verification.
3. [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md) — Agent operating directives and seating configuration.
