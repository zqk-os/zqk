# How-To: Create and Template Objects

Learn how to safely generate and materialize typed Knowledge Kernel objects without violating CLI-first constraints.

---

## The Cardinal Rule: CLI-First
**NEVER edit or create YAML files in `.zqk/process/` directly.** All objects must be created via the `zqk` CLI so that Content-Addressable Storage (CAS) hashes, indexes, and write-ahead logs (WAL) remain in sync.

---

## Step 1: Generate a Spec Template
Use `zqk object template` to produce a valid, commented YAML template for any system kind:

```bash
# Output template to stdout
zqk object template backlog_item

# Save template to a scratch file
zqk object template backlog_item > /tmp/new_bli.yaml
```

---

## Step 2: Edit Your Template
Open `/tmp/new_bli.yaml` in your editor and provide the necessary fields:
- `id`: Unique identifier (e.g. `BLI-MY-FEATURE-001`)
- `title`: Concise summary
- `description`: Narrative description
- `priority`: `critical`, `high`, `medium`, or `low`
- `priority_tier`: `P0`, `P1`, `P2`, or `P3`

---

## Step 3: Materialize the Object
Create the object using the CLI:

```bash
zqk object create backlog_item --file /tmp/new_bli.yaml
```
The CLI validates the schema, computes the SHA-256 content hash, stores the blob in CAS (`.zqk/process/backlog/<hash>.yaml`), updates the in-memory index, and cleans up the temporary file automatically.

---

## Step 4: Link Dependencies & References
Do not edit reference arrays directly. Use `zqk object ref add`:

```bash
# Link backlog items to requirements, criteria, and personas
zqk object ref add BLI-MY-FEATURE-001 REQ-001 CRIT-001 PER-ORCH-ALPHA

# Link Epics to upstream goals, child requirements, and execution workstreams
zqk object ref add EPC-MY-INITIATIVE-001 GOAL-001 REQ-001 REQ-002 WS-CORE-001
```
