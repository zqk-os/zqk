# Modular Pack Composition and Extensibility Architecture Guide

## Overview

ZQK is architected around **Modular Pack Composition**. Rather than bundling every domain capability directly into a monolithic core, the ZQK Kernel acts as a universal, typed knowledge substrate. Functionality—such as work tracking, continuous autonomous programming (CAP), governance, and ecosystem adapters—is partitioned into cohesive, self-contained units termed **Packs**.

This document outlines the architectural rules, directory structures, pack manifest contracts (`pack.yaml`), CLI command builder code generation (`pkg/cli/bldr_cli_cmd_v1`), registration mechanisms, and the crucial distinction between **Kernel Domain Packs** and **Swarm Orchestration Packs**.

---

## 1. Disambiguation: Kernel Domain Packs vs. Swarm Orchestration Packs

A frequent point of confusion is the dual usage of the word "Pack" in modern agentic architectures. In ZQK, these represent two entirely distinct layers:

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│ 1. RUNTIME SWARM ORCHESTRATION PACKS (Declarative Multi-Agent Manifests)               │
│ Location: examples/swarms/*/swarm.yaml, .zqk/swarms/, importable packs                │
│ Content:  YAML definitions of agent seats, persona assignments, and task choreographies│
│ Engine:   Executed by 'zqk swarm run' or 'zqk agent orchestrate'                       │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ 2. KERNEL DOMAIN PACKS (Compile-Time Go Code Modules)                                  │
│ Location: packs/<domain>/ (e.g., packs/agent/, packs/work/, packs/qa/, packs/code-eval/)│
│ Content:  Schemas, lifecycles, Go instance builders, and domain business logic         │
│ Engine:   Compiled into the static 'zqk' binary via composition root                   │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

| Dimension | Kernel Domain Packs (`packs/<domain>/`) | Swarm Orchestration Packs (`swarm.yaml`) |
| :--- | :--- | :--- |
| **Artifact Type** | Go source code, schemas, lifecycles, builders | Declarative YAML configuration |
| **Lifecycle** | Compile-time static linking into `zqk` (Go code) / Dynamic schema indexing (spec metadata) | Runtime interpretation by Swarm Engine |
| **Purpose** | Extends the Knowledge Kernel type system & behavior | Choreographs multi-agent teamwork & seats |
| **Modification** | Spec metadata: Dynamic loading without rebuild; Go code (builders/handlers): Recompilation required (`make`) | Dynamic; author and execute immediately |

> [!NOTE]
> **Dynamic Spec Loading vs. Go Binary Recompilation:** When a pack's declarative specification metadata (YAML schemas in `specs/objects/`, lifecycle state machines in `specs/lifecycles/`, and field definitions) is installed or registered (e.g. via `zqk kind-pack`), the Knowledge Kernel dynamically indexes and validates it without rebuilding the binary. However, new compiled Go code—such as custom command handlers, builders (`bldr_cli_cmd_v1`), or native kernel hooks—requires static recompilation (`make build`).

---

## 2. Philosophy & Categorization of Kernel Domain Packs

Kernel Domain Packs partition the system into cohesive, bounded contexts. They fall into three primary categories:

### A. Core Operational Packs (Shipped with Core Kernel)
Fundamental to all ZQK projects; required for basic functioning:
- **`packs/agent`**: The agent runtime plane. Defines `agent_instruction`, `agent_feed`, persona bindings, and autonomous worker loops. Every agent seat requires this pack.
- **`packs/work`**: The task & program management plane. Defines `goal`, `milestone`, `workstream`, `priority_plan`, and `backlog_item`.
- **`packs/qa`**: The verification plane. Defines `requirement`, `criteria`, `test_case`, `verification_matrix`, and Definition of Done gates.
- **`packs/workflow`**: Process automation, convergence sessions (`CVS-*`), and execution state machines.

### B. Analytical & Measurement Packs
Observability and time-series aggregation:
- **`packs/metric`**: Metric primitives, samplers, and rollups.
- **`packs/evolution`**: Schema migration, version evolution, and deprecation trackers.

### C. Specialized Extension Packs
Purpose-built capabilities that can be enabled, audited, or packaged independently:
- **`packs/code-eval`**: Automated codebase critique, evaluation rubrics, and benchmark grading. Unlike `packs/agent` (which orchestrates live work), `packs/code-eval` provides scoring lenses and audit harnesses.
- **Candidate Future Packs**:
  - `packs/incident`: Post-mortems, incident tracking, and remediation runbooks.
  - `packs/compliance`: Regulatory controls, SOC2/HIPAA evidence mapping.
  - `packs/finops`: LLM token budget management and cost allocation matrices.

---

## 3. Core Principles of Pack Composition

1. **Spec-Driven Knowledge Kernel**: The kernel interprets and validates objects dynamically based on declarative schemas (`.zqk/cli/specs/schemas/`). It does not hardcode domain types or vendor-specific data structures.
2. **Deterministic Code Generation**: High-level declarative command and schema specs are compiled into strongly-typed Go builder patterns using `bldr_cli_cmd_v1`. Code generation is a build-time tool; generated code resides with the pack that owns the spec.
3. **Unified Single Binary**: All registered packs compile into a single static binary (`cmd/zqk`) linked at the composition root. This eliminates runtime ABI incompatibilities, shared library hell, and external daemon version drifts.
4. **Isolated Namespaces and Clean Layering**: Packs operate in distinct namespaces (e.g., `zqk:kernel`, `work`, `cap`, `agent`) while maintaining referential integrity across the unified CAS object graph.

---

## 4. Anatomy of a Pack

A canonical pack resides within the `packs/` directory (or external module root) and follows this standardized layout:

```text
packs/<pack_name>/
├── pack.yaml                 # Pack manifest declaring identity, version, and dependencies
├── specs/                    # Declarative specifications
│   ├── objects/              # Object type definitions (*_spec.yaml)
│   ├── lifecycles/           # State machine definitions (*_lifecycle.yaml)
│   └── cli/                  # Command line interface specs (*_command.yaml)
├── pkg/                      # Internal pack implementation and domain logic
│   └── <domain>/             # Domain services, storage adapters, and controllers
└── cmd/                      # CLI registration entry points
```

### The Pack Manifest (`pack.yaml`)

Every pack must provide a declarative `pack.yaml` manifest adhering to the kernel pack schema:

```yaml
schema_version: "2.0.0"
name: "work"
version: "1.0.0"
description: "Core work tracking, backlog management, and priority program orchestration"
namespace: "work"
author: "ZQK Core Maintainers <inquiry@zqkos.com>"
dependencies:
  - name: "kernel"
    min_version: "1.0.0"
objects:
  - id: "backlog_item"
    spec: "specs/objects/backlog_item_spec.yaml"
    lifecycle: "specs/lifecycles/backlog_item_lifecycle.yaml"
  - id: "priority_plan"
    spec: "specs/objects/priority_plan_spec.yaml"
    lifecycle: "specs/lifecycles/priority_plan_lifecycle.yaml"
commands:
  - spec: "specs/cli/work_command.yaml"
```

---

## 5. Declarative CLI Command Specs & Builder Generation

CLI commands in ZQK are not authored using ad-hoc `cobra.Command` structures. Instead, they are defined declaratively in `.zqk/cli/specs/` and generated via the builder toolchain into `pkg/cli/bldr_cli_cmd_v1/`.

### Declarative Command Spec Example

```yaml
schema_version: "1.0.0"
command: "sync"
use: "sync [subcommand]"
short: "External issue and tracking synchronization"
long: "Synchronize local kernel objects bidirectionally with external project management platforms."
flags:
  - name: "dry-run"
    type: "bool"
    description: "Simulate synchronization without applying CAS mutations"
subcommands:
  - command: "github"
    use: "github [flags]"
    short: "Synchronize with GitHub Issues"
```

### The Builder Pattern (`bldr_cli_cmd_v1`)

The generator translates the YAML specification into idiomatic Go builders:

```go
package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSyncCommandBuilder builds the root cobra.Command for 'zqk sync'
func NewSyncCommandBuilder() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "External issue and tracking synchronization",
	}
	cmd.Flags().Bool("dry-run", false, "Simulate synchronization without applying CAS mutations")
	return cmd
}
```

This guarantees 100% compliance with CLI taxonomy standards, uniform `--help` output, deterministic flag parsing, and automatic AST auditing conformance.

---

## 6. Composition Root Registration

At the composition root (`cmd/zqk/root.go` and `cmd/zqk/main.go`), packs register their CLI trees, object types, and lifecycle state machines into the central registries:

```go
func init() {
	// Register commands generated from pack specifications
	rootCmd.AddCommand(sync.NewSyncCmd())
	rootCmd.AddCommand(object.NewObjectCmd())
	rootCmd.AddCommand(workflow.NewWorkflowCmd())
}
```

When new packs are incorporated into the system, developers execute `./scripts/build-cli-builders.sh` to update generated surfaces before compilation.

---

## 7. Adding a Custom Pack (Extensibility Walkthrough)

To create and integrate an extension pack:

1. **Initialize Directory**: Create `packs/<my-pack>/` with `pack.yaml`.
2. **Declare Schemas**: Author object specs and lifecycle transitions under `packs/<my-pack>/specs/`.
3. **Declare CLI Commands**: Add command specs in `.zqk/cli/specs/new/` or `packs/<my-pack>/specs/cli/`.
4. **Compile Builders**: Run `./bin/zqk dev bldr-generate` to synthesize `pkg/cli/bldr_cli_cmd_v1/`.
5. **Implement Command Handlers**: Wire the builder to internal execution logic using `cli.WithProcessor`.
6. **Register in Root**: Add the command builder into `cmd/zqk/main.go` or root command registry.

---

## Canonical References
- [Architecture Index](../INDEX.md)
- [Lifecycle State Machine Specification](LIFECYCLE_STATE_MACHINE.md)
- [Pack Composition Workspace](../../PACK-COMPOSITION.md)
