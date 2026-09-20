---
name: zqk-cellular-membrane
description: >-
  Navigates the ZQK Cellular Membrane and Privileged Writer Mode B boundary. Enforces safe object mutations via the Semantic Intake Reasoner (zqk intake), prevents unauthorized direct file writes to .zqk/process/, and handles UNIX domain socket fail-closed barriers.
---

# ZQK Cellular Membrane & Privileged Writer Skill

> **Kernel Object Ref:** `ASK-1789810877924085000-7bef60c0`  
> **Specification Ref:** `TSP-CELLULAR-MEMBRANE-MODE-B-001`  
> **Runbook Ref:** `docs/architecture/CELLULAR_MEMBRANE_MODE_B_CONFIGURATION.md`

**Your primary directive is preserving the data-plane security boundary of the ZQK Knowledge Kernel in Mode B environments.**

In locked-down or multi-agent swarm environments, the repository enforces **Mode B (Cellular Membrane Lockdown)**. Direct filesystem mutations to `.zqk/process/` or `.zqk/streams/` are blocked fail-closed. All modifications must be mediated by the `PrivilegedWriterDaemon` via UNIX domain socket or the Semantic Intake Receptor (`zqk intake`).

---

## 1. Operating Modes & Boundary Detection

Before attempting any object or state modifications, identify which mode the host environment is running:

1. **Check for Socket Rendezvous:**
   ```bash
   test -S /tmp/zqk-privileged-writer.sock && echo "Mode B: Cellular Membrane Active" || echo "Mode A: Standalone"
   ```
2. **Check Service Status:**
   ```bash
   zqk scheduler service status com.zqk.privileged-writer
   ```

- **Mode A (Developer Standalone):** Client tools write directly to disk; standard CLI commands (`zqk object create`, `zqk object update`) perform local I/O.
- **Mode B (Lockdown):** Direct disk write is revoked. Direct `touch` or file editing in `.zqk/process/` will fail with POSIX `Permission denied` or `ErrDirectWriteProhibitedInModeB`. All writes route over `/tmp/zqk-privileged-writer.sock`.

---

## 2. Core Agent Directives

### Directive 1: Never Attempt Direct Disk Edits on `.zqk/process/`
- **Prohibited:** Never use `write_to_file` or `replace_file_content` to edit YAML files in `.zqk/process/`. This causes hash mismatches, corrupts the CAS index, and will be blocked by the OS under Mode B.
- **Mandated:** Always use CLI commands (`zqk object ...`), the Semantic Intake Receptor (`zqk intake`), or MCP kernel tools (`mcp_kernel_put`).

### Directive 2: Ingest Free-Form Intent via `zqk intake`
When capturing new requirements, backlog items, or technical specs, leverage the Semantic Intake Reasoner:
```bash
zqk intake "<narrative description of feature or fix>" \
    --kind <requirement|backlog_item|technical_spec> \
    --priority <p0|p1|p2|p3>
```
Or via the tray:
```bash
zqk tray run intake "<narrative description>"
```
**Why this matters:**
- The Semantic Intake Reasoner automatically synthesizes mandatory fields (`description`, `title`, timestamps, initial statuses).
- Validated payloads pass directly through the Privileged Writer socket into CAS storage, skipping the preliminary Draft Plane (`.zqk/object_drafts/`) and eliminating manual promotion steps.

### Directive 3: Enforce Mandatory Descriptions on All Objects
- Any object missing a non-empty `description` field will be rejected fail-closed at the membrane boundary (POL-DOC-001, REQ-1789334232564133000-02789aa2).
- When authoring templates or YAML inputs for `zqk object create`:
  ```yaml
  title: "Clear, concise action-oriented title"
  description: "Detailed explanation of operational scope, motivation, and verification criteria."
  ```
  Do NOT specify manual `status` on creation—allow the lifecycle state machine to assign the initial valid status or use `--promote`.

### Directive 4: Socket Failure & Escalation Protocol
If the privileged writer socket `/tmp/zqk-privileged-writer.sock` exists but the daemon is unresponsive:
1. Do **NOT** attempt to delete the socket file or bypass permissions.
2. Check daemon health:
   ```bash
   zqk scheduler service status com.zqk.privileged-writer
   ```
3. If degraded, issue a restart:
   ```bash
   zqk scheduler service restart com.zqk.privileged-writer
   ```
4. If still unreachable, report the failure and signal escalation rather than attempting insecure fallbacks.
