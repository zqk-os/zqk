# Module 1: Daily Orientation & The Workflow Suite

Welcome to **Module 1** of the Zen Quantum Kernel (ZQK) Masterclass. This tutorial covers the daily orientation routine that human operators and autonomous agents run every morning to orient their workday in milliseconds.

---

## 1. The Morning Dilemma

In traditional development workflows, engineers and AI assistants burn precious morning hours digging through stale Kanban boards, outdated Slack threads, and unprioritized issue trackers. For autonomous agents, the absence of a deterministic entry point leads directly to hallucinated goals and drift.

ZQK replaces this uncertainty with the **Three-Step Orientation Triad**:

```mermaid
flowchart LR
    A["Step 1: whats-next"] -->|Discover Active Plan| B["Step 2: pplan current"]
    B -->|Lock Milestone Invariants| C["Step 3: vds evaluate"]
    C -->|Verify Done-Gates| D["Safe Autonomous Execution"]
```

---

## 2. The Three-Step Orientation Triad

### Step 1: Runway Discovery (`zqk workflow whats-next`)
Inspect the active runway flight board. In milliseconds, ZQK queries the Knowledge Kernel and returns the active priority plan, groomed backlog items, and ambient steward guidance.

```bash
# Human-facing flight board table:
zqk workflow whats-next

# Machine-facing structured JSON tokens for autonomous agent ingest:
zqk workflow whats-next --format json
```

**Expected JSON Response:**
```json
{
  "schema": "zqk_whats_next_v1",
  "priority_plan": {
    "id": "PRI-1793959074191907000-fc210840",
    "title": "ASMM Go Media Studio Dynamic Stereo Multiband Resonant Body & Acoustic Formant Filter Studio Engine",
    "status": "grooming",
    "backlog_counts": {
      "originated": 1,
      "in_progress": 0,
      "complete": 0
    }
  },
  "kernel_steward": {
    "lead_priority_plan": "PRI-1793959074191907000-fc210840",
    "align_score": 98.2,
    "next_action": "execute whats-next priority_plan"
  }
}
```

### Step 2: Milestone Invariant Lock (`zqk workflow pplan current`)
Lock your working session to the active milestone invariants and compiler contracts before making code edits. This guarantees that your work aligns with the active priority plan's branch specification and acceptance criteria.

```bash
zqk workflow pplan current
```

### Step 3: Verification Done-Gate Audit (`zqk vds evaluate`)
Enforce continuous verification before work is claimed. The Verifiable Decomposition Spine (VDS) checks:
1. Zero race conditions (`go test -race`).
2. Zero unbound criteria.
3. Cryptographically sound lineage back to root architectural goals.

```bash
zqk vds evaluate --strict
```

---

## 3. Continuous Parity: Humans and Machine Agents

Across the entire ZQK CLI, stdout automatically adapts to the consumer:
- **Interactive Human View:** High-visibility tabular flight boards with ANSI color highlights and clear summaries.
- **Machine Agent View:** Token-efficient, strictly typed JSON or YAML streams (`--format json`, `--format yaml`) suitable for LLM ingestion without extraneous punctuation or unbudgeted token consumption.

---

## 4. Video Showcase & Theater

- **High-Definition Video:** `assets/productions/masterclass_v01_daily_orientation.mp4` (1080p60 Manim Animation)
- **Captions & Transcript:** `assets/productions/masterclass_v01_daily_orientation.srt`
- **Rubric Audit Verification:** Passed 100.0% across all 5 pillars of `POL-MEDIA-INSPECTION-RUBRIC-001`.

---

## 5. Next Steps

In **Module 2**, we dive into minting architectural blueprints and scaling the 5-layer cascade from draft plane to immutable CAS storage.
