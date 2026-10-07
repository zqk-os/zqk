---
name: community-cli-ergonomics
description: CLI command taxonomy, ergonomics, findability, and responsive console feedback.
---

# CLI Ergonomics, Command Taxonomy & Responsive Feedback

## Objective
Govern the command surface area of the ZQK CLI binary to guarantee intuitive navigation, complete discoverability, strict taxonomy adherence, and zero-stall console feedback for human operators and autonomous agents.

## Core Invariants

### 1. Canonical Command Taxonomy (`zqk <domain> <verb>`)
- Every root verb must belong to an approved domain taxonomy group (Everyday, Integrations, Advanced, Administration).
- Subcommands must follow consistent action verbs (`list`, `get`, `create`, `update`, `delete`, `check`, `run`, `validate`, `show`).
- Never introduce ambiguous or phantom command names.

### 2. Help & Discoverability Standard
- Every command and flag must define a concise, descriptive `--help` entry with runnable copy-pasteable examples.
- Unknown or misspelled commands must fail closed with exit code 1 and provide fuzzy-matching "Did you mean ...?" suggestions.

### 3. Responsive Console Feedback (Zero-Stall Experience)
- Any command or operation running longer than 500ms must emit real-time status:
  - Interactive TTY: ANSI spinner or progress indicator via `pkg/cli/ux` with active step description and elapsed timer.
  - Non-Interactive / JSON: Suppress ANSI codes and emit streaming JSON or structured log entries.
- Users and agents must never be left in an ambiguous or silent wait state.
