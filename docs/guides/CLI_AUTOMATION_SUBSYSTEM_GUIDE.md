# CLI Automation Subsystem & Lint Bypass Coordination Guide

## Overview

The ZQK Automation Subsystem (`cmd/zqk/automation`) provides essential automated housekeeping, documentation registration synchronization, and lint bypass audit coordination for continuous integration and agentic development workflows. It bridges filesystem artifacts, Git staging state, and the Knowledge Kernel without manual intervention.

The subsystem delivers three primary operational capabilities:
1. **Hierarchical Command Orchestration**: Provides the parent `zqk automation` command tree hosting extensible automated subcommands.
2. **Docman Registration Synchronization (`docman-sync`)**: Scans repository Markdown documentation files (globally or restricted to Git staged/modified files) and registers them with the Knowledge Kernel's documentation registry (`pkg/docman`), supporting `--check-only`, `--git-aware`, `--update-existing`, and `--dry-run` modes.
3. **Lint Bypass Audit Coordination (`lint-bypass-audit`)**: Intercepts and audits situations where developers or automated agents bypass standard linting checks (e.g., in break-glass scenarios or emergency hotfixes), capturing committer identity, commit message, and touched files as structured audit events in the Knowledge Kernel.

---

## Architectural Taxonomy & Core Components

```
cmd/zqk/automation/
├── automation.go                   # Parent 'automation' command definition & hierarchy
├── automation_constants.go         # Command names, flag literals, and default configuration
├── docman_sync.go                  # 'docman-sync' command execution & lifecycle
├── docman_sync_helpers.go          # Markdown file discovery, Git status parsing, and registry calls
├── lint_bypass.go                  # 'lint-bypass-audit' command flags, validation, & execution
├── lint_bypass_helpers.go          # Git author/committer detection, commit message & staged file parsing
└── lint_bypass_coordination.go     # Coordinator bridge linking lint checks to kernel audit loggers
```

---

## 1. Docman Synchronization Engine (`docman-sync`)

The `docman-sync` command ensures that repository documentation is continuously indexed and accessible to agents, tools, and operators through the Knowledge Kernel.

### Command Syntax

```bash
zqk automation docman-sync [flags]
```

### Supported Flags

| Flag | Type | Description |
|------|------|-------------|
| `--check-only` | bool | Verification mode; exits with non-zero status if unregistered documentation files are detected. |
| `--git-aware` | bool | Restricts scanning to Markdown files tracked, modified, or staged in Git rather than scanning the entire tree. |
| `--update-existing` | bool | Updates metadata and content hash of existing documentation entries in the Knowledge Kernel. |
| `--dry-run` | bool | Simulates synchronization without persisting any mutations to `.zqk/process/doc_entries`. |

### Execution Workflow

```
[zqk automation docman-sync]
             │
             ▼
   Is --git-aware set?
     ├── Yes ──► Run `git status` / `git ls-files` to isolate touched .md files
     └── No  ──► Walk repository filesystem discovering all .md documents
             │
             ▼
   Filter against ignored paths (vendor, node_modules, .zqk/cache)
             │
             ▼
   Compare discovered docs against Kernel Registry (`pkg/docman`)
             │
             ▼
   Is --check-only set?
     ├── Yes ──► If unregistered files exist, fail with exit code 1
     └── No  ──► Persist new or updated doc_entry objects (unless --dry-run)
```

---

## 2. Lint Bypass Audit Coordination (`lint-bypass-audit`)

When emergency operational velocity requires bypassing pre-commit or CI linting checks, the system enforces non-repudiation by recording a tamper-evident audit event in the Knowledge Kernel.

### Command Syntax

```bash
zqk automation lint-bypass-audit [flags]
```

### Supported Flags

| Flag | Type | Description |
|------|------|-------------|
| `--git-user` | string | Committer/author name. Auto-detected from Git config if omitted. |
| `--git-email` | string | Committer/author email. Auto-detected from Git config if omitted. |
| `--commit-message` | string | Commit message associated with the bypass. Auto-detected from staged commit if omitted. |
| `--files` | string | Comma-separated list of files bypassed. Auto-detected from staged files if omitted. |
| `--auto-detect` | bool | Controls automatic Git context resolution (default: `true`). When set to `false`, explicit flags are mandatory. |
| `--dry-run` | bool | Previews the generated audit event payload without writing to the audit journal. |

### Invariant & Validation Rules

1. **Mandatory Identity**: Committer name and email must resolve either via Git configuration (`user.name`, `user.email`) or explicit CLI flags. Missing identity triggers immediate fail-closed error.
2. **Context Resolution**: When `--auto-detect=false`, `--git-user`, `--git-email`, and `--commit-message` are strictly enforced as required arguments.
3. **Audit Immutability**: Recorded bypass events are written to the Knowledge Kernel audit trail with nanosecond timestamps, Git commit references, and affected file manifests.

---

## 3. Verification & Testing Discipline

The automation subsystem maintains $\ge 80.0\%$ statement test coverage enforced across three verification tiers:

1. **Command Structure & Hierarchy Verification (`automation_test.go`)**: Validates Cobra command tree integrity, flag binding, and standard help output.
2. **Functional & Edge Case Verification (`docman_sync_test.go`, `lint_bypass_test.go`)**: Validates behavior across clean repositories, dirty Git working trees, missing Git configurations, and dry-run execution paths.
3. **Integration Scenarios (`docman_sync_integration_test.go`, `lint_bypass_coordination_test.go`)**: Uses isolated temporary testkit projects (`testkit.PrepareIsolatedTempProject`) with real Git repositories to verify end-to-end execution without polluting developer workspace state.

---

## 4. Integration with CI/CD and Agentic Loops

- **Pre-Commit Hooks**: The git shim (`cmd/zqk-shim`) triggers `docman-sync --check-only` and `lint-bypass-audit` during commit phases.
- **Continuous Integration**: The `CI/Lint & Format` workflow verifies documentation synchronization across PR branches using `zqk automation docman-sync --check-only`.
- **Autonomous Swarms**: Subagents query documentation through the unified doc registry populated by the automation subsystem.
