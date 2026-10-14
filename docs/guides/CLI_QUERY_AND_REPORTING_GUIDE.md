# CLI Query & Reporting Command Engine Guide

## Executive Summary

The ZQK CLI Query and Reporting Command Engines provide real-time declarative graph querying and AI-augmented predictive telemetry across the persistent Knowledge Kernel:

- **`zqk query` (alias `zqk zparql`)**: High-performance, in-memory graph pattern matching engine executing ZPARQL queries (`MATCH ... WHERE ... RETURN ...`). Includes an interactive REPL console (`--interactive` or `repl`) and ASCII graph path visualizer (`--visualize` or `--format visual`).
- **`zqk reports`**: Real-time project telemetry engine calculating Project Confidence Score (**PCS**), Effort Distribution Discrepancy (**EDD**), and Dependencies & Blockers (**D&B**), enhanced with Git commit telemetry and quick lifecycle triage reports (`quick`).

---

## 1. Declarative Graph Queries (`zqk query`)

### Overview & Grammar
The `query` command evaluates Cypher-like ZPARQL queries against indexed objects and relational edges in the Knowledge Kernel (`.zqk/process/`).

```bash
# Basic match on specific object kind
zqk query "MATCH (b:backlog_item) RETURN b.id, b.status LIMIT 10;"

# Relationship path traversal
zqk query "MATCH (p:priority_plan)-[:items]->(b:backlog_item) RETURN p.title, b.id;"

# Multi-hop relationship traversal
zqk query "MATCH (g:goal)-[:requirement_refs]->(r:requirement)-[:criteria_refs]->(c:criteria) RETURN g.id, r.id, c.id;"

# Wildcard node matching across all indexed kinds
zqk query "MATCH (n) WHERE n.status == 'in_progress' RETURN n.kind, n.id;"
```

### Interactive REPL Console (`zqk query --interactive`)
Starting the console enters an interactive prompt (`zparql>`) with auto-indexing and meta-commands:

| Meta-Command | Description |
| :--- | :--- |
| `.help`, `.h` | Displays the interactive REPL guide and syntax examples |
| `.kinds` | Lists all indexed object kinds with total node counts |
| `.tables`, `.edges` | Lists all indexed graph relationships and common traversal edges |
| `.schema [kind]` | Inspects field definitions, types, and required markers for any kind |
| `.format [table\|json\|visual]` | Sets default output format for executed queries |
| `.visualize [on\|off]` | Toggles ASCII graph path visualization mode |
| `.refresh`, `.reload` | Re-indexes the Knowledge Kernel storage graph from disk |
| `.clear` | Clears the multi-line query input buffer |
| `.exit`, `.quit`, `.q` | Exits the interactive console session |

### Graph Edge Visualizer (`--visualize`)
Visualizing query results outputs hierarchical graph trees showing incoming, outgoing, and undirected relationships:

```text
=== ZPARQL Graph Path Visualization ===

[1] Path:
  ● (p:priority_plan PRI-1791956057434882000-1650245d)
      ───[:items]──► (b:backlog_item BLI-1791956047783734500-343f3063)
          ───[:criteria_refs]──► (c:criteria CRIT-1791956047783734000-9dabbbcd)
      └─ Properties: p.title=Cycle 68, b.status=in_progress

(Visualized 1 path(s))
```

---

## 2. Predictive AI Metrics & Reports (`zqk reports`)

### Project Confidence Score (`zqk reports pcs`)
Measures overall project health and delivery momentum on a normalized scale (0–100):

- **>= 80 (Excellent)**: High delivery velocity, healthy commit frequency, minimal blockers.
- **60–79 (Good)**: Project on track with minor non-blocking risks.
- **40–59 (Fair)**: Velocity deceleration or elevated open technical debt; monitor closely.
- **20–39 (Poor)**: Significant delivery impediments; action required.
- **< 20 (Critical)**: High failure risk; immediate TPM intervention mandated.

```bash
# Tabular summary output
zqk reports pcs

# Structured machine-readable output
zqk reports pcs --format json
zqk reports pcs --format yaml --include-commit-data
```

### Effort Distribution Discrepancy (`zqk reports edd`)
Evaluates variance between estimated effort and actual delivered reality:

- **Positive EDD (> 5%)**: Underestimation (work items took more effort/commits than projected).
- **Near Zero (-5% to +5%)**: High estimation accuracy and reliable scoping.
- **Negative EDD (< -5%)**: Overestimation (work items required less effort than reserved).

```bash
zqk reports edd --format json
```

### Dependencies & Blockers (`zqk reports blockers`)
Aggregates code-level dependencies, architectural co-changes, stale file anomalies, and active risk blockers:

```bash
zqk reports blockers
```

### Quick Lifecycle Reports (`zqk reports quick`)
Fast triage commands for immediate operational health:

```bash
# Unanswered questions
zqk reports quick --preset questions

# Overdue milestones
zqk reports quick --preset milestones-overdue
```
