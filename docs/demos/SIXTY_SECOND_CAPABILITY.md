# 60-Second Capability Demo (Buyer / Procurement)

**Audience:** Procurement, security reviewers, and engineers evaluating open-core ZQK.  
**Goal:** Prove durable kernel capability in ~60 seconds of wall clock — not a slide deck.  
**License ACK:** Community kernel is [Apache License 2.0](../../LICENSE); Enterprise fleet mesh is separately licensed (see README **Apache / Enterprise delineation ACK**).

This is the short path. Full narrative demos live in [docs/demos/README.md](./README.md). First-run depth: [Community First-Run](../onboarding/COMMUNITY_FIRST_RUN.md).

---

## Preconditions (~10s)

```sh
# Binary on PATH (Homebrew, release, or local build)
zqk --version

# Or from a clone:
./bin/zqk --version
```

No `GITHUB_TOKEN` / `GH_TOKEN` required for this smoke.

---

## 60-second script (copy/paste)

```sh
# T0 — empty greenfield (~5s)
rm -rf /tmp/zqk-60s-demo && mkdir /tmp/zqk-60s-demo && cd /tmp/zqk-60s-demo

# T1 — init kernel (~5–15s)
zqk init

# T2 — mission discovery without prompt reconstruction (~1–3s)
zqk workflow whats-next

# T3 — object graph is real (list kinds / counts) (~1–3s)
zqk object count

# T4 — optional interactive tour (same product surface as first-run)
# zqk test-drive
```

**Pass signal:** `whats-next` returns structured mission/plan context (not an empty chat reconstruction), and `object count` shows kernel kinds after `init`.

**Timed stranger path (CI / unattended):** from a clone of the public candidate / community tree:

```sh
./scripts/open-core/ttfa-unauth-smoke.sh
```

That script unsets GitHub tokens, installs from local source when needed, times `zqk init` + `zqk workflow whats-next`, and fails if wall clock exceeds 120s (procurement bar: under 2 minutes; interactive demo targets ~60s).

---

## What you just proved

| Claim | Evidence in this run |
| :--- | :--- |
| Local-first kernel | `zqk init` wrote project state under `.zqk/` with no cloud account |
| Task truth ≠ prompt memory | `whats-next` reads the kernel graph |
| Open-core boundary | You exercised **community** single-cell CLI only — no Enterprise fleet mesh APIs |
| Apache surface | Commands above ship in the Apache-2.0 community tree (`LICENSE` / `NOTICE`) |

---

## Next 2 minutes (optional)

```sh
zqk ui -w                 # Visual Web Studio
zqk mcp ensure            # Wire Cursor / Claude Code
zqk system agent-onboard  # Detect IDE / seat agent
# zqk do   # Studio CAP loop — not required for stranger 60s proof
```

Aligns with Community First-Run Quickstart (2–3 commands).

---

## Related

- [README Quick Start (< 2 min)](../../README.md#quick-start--2-min)
- [Show HN / launch narrative](./SHOW_HN.md)
- [Interactive demo suite](./README.md)
