# Polyglot `zgrep` AST Search, Agent Compliance & Pilot ROI Guide

> **Audience:** Engineering leads, AI coding practitioners, and design-partner teams running Cursor, Claude Code, Windsurf, Cline, or Antigravity on TypeScript/JavaScript, Python, Rust, and Go repositories.

---

## 1. 60-Second Zero-Bloat Setup (`zqk init --lite`)

You do not need to adopt the full Knowledge Kernel ontology on Day 1. Land with sub-15ms polyglot `zgrep` and compliance telemetry first:

```bash
# Install ZQK (Homebrew or direct installer)
brew install zqk-os/tap/zqk

# Zero-bloat zgrep-first setup (creates .zqk/{cache,logs,state}, IDE rules, trigram index;
# does NOT seed .zqk/process CAS objects)
zqk init --lite

# Or wire multi-host MCP configs AND PreToolUse compliance hooks in one command:
zqk init --lite --with-mcp --with-hooks
```

When your team is ready to expand into Git-versioned Requirements, Criteria, Verification Done-Gates, and multi-agent swarms, upgrade in place with one command:

```bash
zqk init --merge
```

---

## 2. Polyglot `zgrep` AST Structural Queries

`zqk grep` (alias `zgrep`) runs 100% in-process in pure Go with zero external `ripgrep`, CGo, or Tree-Sitter dependencies. It supports structural AST queries (`--ast`) across **TypeScript/JS** (`.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`, `.cjs`), **Python** (`.py`, `.pyi`), **Rust** (`.rs`), and **Go** (`.go`):

| Goal | Command |
|---|---|
| List all classes/structs across a polyglot repo | `zqk grep --ast --kind struct` |
| List all interfaces/traits across TS, Rust & Go | `zqk grep --ast --kind interface` |
| Find methods belonging to a class, struct, or Rust `impl` | `zqk grep --ast --recv OrderService` |
| Find functions/methods matching a symbol name | `zqk grep --ast --kind func "Validate"` |
| Token-budgeted JSON output for AI coding agents | `zqk grep "payment" --max-tokens 2000 -f json` |
| Rebuild persistent trigram index cache | `zqk grep --reindex` |

---

## 3. Agent Decision Rubric: When to Use `zgrep` vs. Standard Reads

`zqk system agent-onboard` primes every AI coding host with a clear decision rubric so agents avoid dumping entire directories into their context window:

- **USE `zgrep` (`zqk grep`)** for:
  - Cross-file codebase discovery and symbol lookup (`--ast --kind`, `--ast --recv`).
  - Regex or literal searches across repositories where raw `grep`/`rg`/`find` would overflow token budgets.
  - Structured, token-budgeted agent responses (`--max-tokens 2000 -f json`).
- **USE standard file reads** for:
  - Reading a single file whose exact path you already know.
  - Targeted line-range inspection after `zgrep` has returned the exact file and line numbers.

When a pre-command hook intercepts raw `grep`, `rg`, `git grep`, or repo-wide `find`, ZQK emits an actionable steering nudge and records a compliance observation to `.zqk/logs/zgrep_compliance.jsonl`.

---

## 4. Measuring Day-0 vs. Day-25 Pilot ROI (`zqk grep --roi`)

Inspect your repository's `zgrep` compliance ratio and estimated context tokens saved at any time:

```bash
# Human-readable scorecard
zqk grep --roi

# Executive Markdown table for Slack, Notion, or Day-25 pilot renewal reviews
zqk grep --roi -f markdown

# Machine-readable JSON / YAML for dashboards
zqk grep --roi -f json
zqk grep --roi -f yaml

# Fail-closed CI / PR compliance threshold gate (exits non-zero if below 80%)
zqk grep --roi --min-compliance 80

# Rolling 7-day window across multiple repositories (ideal for Week-4 vs. Week-1 reviews)
zqk grep --roi --repos ./svc-api,./svc-billing --since 7d -f markdown --min-compliance 80
```

For drop-in GitHub Actions PR enforcement with automatic `GITHUB_STEP_SUMMARY` scorecards, copy [`docs/guides/examples/zgrep-compliance-gate.yml`](./examples/zgrep-compliance-gate.yml) into `.github/workflows/`.

Example output:

```text
zgrep Pilot ROI Scorecard (/path/to/repo)
------------------------------------------------------------
Total Search Invocations:      142
zgrep Compliant Invocations:   128
Raw Shell Search Intercepted:  14
zgrep Compliance Ratio:        90.1%
Estimated Tokens Saved:        816000
Telemetry Log:                 /path/to/repo/.zqk/logs/zgrep_compliance.jsonl
```

---

## 5. Related References

- [Community First-Run Walkthrough](../onboarding/COMMUNITY_FIRST_RUN.md)
- [Verification Data Spec & Evidence Guide](./VERIFICATION_DATA_SPEC_AND_EVIDENCE_GUIDE.md)
- [Kernel Object Administration Guide](./KERNEL_OBJECT_ADMINISTRATION.md)
