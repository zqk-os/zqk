# Pre-Public-Beta Launch: Curriculum & Screen Map

## The Concept: Bi-Directional Engulfment
To achieve scale across a mesh of 1,000,000 ZQK kernels, we must align the **User Perspective** (The Visual Map) with the **Code Perspective** (The AST/Graph). When the user's expected visual outcome matches the kernel's internal behavioral checksum, the mesh achieves harmony. This document serves as the curriculum for our launch team and the baseline behavioral map.

---

## Part 1: The Visual Screen Map (User Journey)

These "screens" represent the exact terminal output states the user experiences. The launch team must memorize these states to quickly orient themselves when supporting a partner.

### Screen 1: The Ignition (`wake.sh`)
**Trigger:** `curl -sL https://zqk.ai/wake | bash`
**Visual Expected Outcome:**
```text
=== ZQK: Waking the Organism ===
[1/4] Checking dependencies...
✓ Go version is go1.26 (satisfies 1.26 requirement)
✓ Inside local ZQK repository.
```
**Kernel Alignment:** The kernel is validating host primitives before attempting mutation.

### Screen 2: The Guardrail (State Protection)
**Trigger:** Existing `.zqk/` directory found.
**Visual Expected Outcome:**
```text
[2/4] Verifying state integrity...
⚠️  Existing ZQK state detected.
To prevent schema corruption, please choose an action:
  [M]igrate : Keep existing state and proceed.
  [W]ipe    : Delete existing state and start fresh.
  [A]bort   : Cancel installation.
```
**Kernel Alignment:** Idempotent safety. The organism refuses to overwrite its own memory without explicit human-in-the-loop consent.

### Screen 3: The First Breath (Organism Awake)
**Trigger:** Successful installation and scheduler daemon boot.
**Visual Expected Outcome:**
```text
   ____  ____  __  _
  |__  |/ __ \| |/ /
    / /| |  | | ' / 
   / /_| |__| | . \ 
  /____|\___\_\_|\_\
    ORGANISM AWAKE

🚀 The ZQK Beta is now active.
🧠 Backend: file
⚙️  Scheduler PID: 47492
```
**Kernel Alignment:** The `file` backend overrides are successfully injected, and the telemetry loop is active.

### Screen 4: The Tiered Help Menu (Orientation)
**Trigger:** `./zqk --help`
**Visual Expected Outcome:**
```text
ZQK - Zen Quantum Kernel for AI + human hybrid teams

Everyday Commands:
  object      Manage Knowledge Kernel objects
  scheduler   Manage the ZQK scheduler and job queues

Integrations & Automation:
  automation  Run automated workflows
  mcp         Model Context Protocol server

Administration:
  internal    Developer & Administration tools
```
**Kernel Alignment:** The command tree successfully registered the dynamic kind subcommands and grouped them by cognitive load.

### Screen 5: The Sales Feedback Loop (Anomaly Reporting)
**Trigger:** Partner encounters an issue; Sales Team executes the failsafe.
**Visual Expected Outcome:**
```text
$ ./zqk system snapshot --reason "Partner X encountered graph timeout"
✓ System state snapshotted to .zqk-state/snapshots/snap_1780288...
✓ Ready for transmission to core.
```
**Kernel Alignment:** The system cleanly halts operations and packages its entire state vector for engineering review without panicking the partner.

---

## Part 2: Sales Team Curriculum ("Calm Under Fire")

**Rule 1: The Organism Never "Breaks", It "Halts to Protect."**
If a partner hits an error, the sales team must reframe it: "ZQK detected an anomaly and halted to protect your state." 

**Rule 2: Capture the State, Don't Guess.**
Do not attempt to debug live on a partner call. Instruct the partner to run `zqk system check` and `zqk system snapshot`. Send the snapshot back to the engineering team. 

**Rule 3: Trust the Screen Map.**
Use the visual map above. If the terminal output deviates from the Screen Map, you immediately know where the drift occurred.

---
*Drafted by the Strategy Architect & Visionary Exec - May 2026*