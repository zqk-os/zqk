# ZQK Documentation Index

Welcome to the ZQK (Zen Quantum Kernel) documentation portal. This index organizes documentation following Divio's standard documentation quadrant: **Tutorials**, **How-To Guides**, **Reference**, and **Explanation**.

---

## 🚀 Quick Start

| Resource | Description |
| :--- | :--- |
| **[5-Minute Quickstart Tutorial](./tutorials/quickstart.md)** | Install, build, and run your first ZQK session |
| **[CLI User Manual](./manual/CLI_REFERENCE.md)** | Comprehensive command-line reference and flag guide |
| **[Contributing Guidelines](../CONTRIBUTING.md)** | Community workflow, PR governance, and policies |

---

## 1. 🎓 Tutorials (Learning-Oriented)

Step-by-step learning tracks for new developers and agent operators:

| Guide | Description |
| :--- | :--- |
| **[5-Minute Quickstart](./tutorials/quickstart.md)** | Zero to running kernel and swarm session |
| **[First Agent Session](./tutorials/first-agent-session.md)** | Claiming backlog items and executing agent loops |
| **[Object Lifecycle](./tutorials/object-lifecycle.md)** | Navigating planned → in-progress → complete states |

---

## 2. 🛠️ How-To Guides (Problem-Oriented)

Practical recipes for common administrative and operational tasks:

| Guide | Description |
| :--- | :--- |
| **[Create and Template Objects](./howto/create-objects.md)** | Safe object generation via `zqk object template` |
| **[Workflow VDS Configuration](./howto/workflow-vds.md)** | Lifecycle preconditions and transition verification |
| **[Agent Admin Membrane](./howto/agent-admin-membrane.md)** | Seating, sandboxing, and atomic work claims |
| **[Process CAS Commits](./howto/process-cas-commits.md)** | Managing Git Content-Addressable Storage hashes |

---

## 3. 📖 Reference (Information-Oriented)

Exhaustive technical references, specs, and schemas:

| Resource | Description |
| :--- | :--- |
| **[Reference Portal Overview](./reference/README.md)** | Technical reference landing page and system specs index |
| **[CLI Reference Manual](./manual/CLI_REFERENCE.md)** | Complete guide to all commands, flags, and exit codes |
| **[System Objects Guide](./onboarding/SYSTEM_OBJECTS_GUIDE.md)** | Ontology, schemas, and fields for all 99 system kinds |
| **[Architecture Patterns](./architecture/ARCHITECTURE_PATTERNS.md)** | Project patterns, logging, error handling, and concurrency |

---

## 4. 💡 Explanation (Understanding-Oriented)

Architectural rationale and conceptual background:

| Concept | Description |
| :--- | :--- |
| **[Kernel vs. IDE Projections](./explanation/kernel-vs-ide.md)** | Why the kernel is the single source of truth |
| **[VDS State Machine Design](./explanation/vds-state-machine.md)** | Version-driven state graphs and auditor gates |
| **[The Agent Membrane Pattern](./explanation/agent-membrane.md)** | Separating cognitive coding from supervisor loops |

---

## 📂 Documentation Structure Map

```text
docs/
├── INDEX.md                      ← Central Divio portal index
├── getting-started.md            ← Quick start entry point
├── cli-reference.md              ← CLI reference shortcut
├── tutorials/
│   ├── quickstart.md             ← 5-minute onboarding tutorial
│   ├── first-agent-session.md    ← Multi-agent operational tutorial
│   └── object-lifecycle.md       ← FSM lifecycle progression
├── howto/
│   ├── create-objects.md         ← CLI-first object creation
│   ├── workflow-vds.md           ← Version-driven state rules
│   ├── agent-admin-membrane.md   ← Seating & sandboxing
│   └── process-cas-commits.md    ← CAS hash management
├── reference/
│   ├── README.md                 ← Technical reference landing page
│   └── CLI_REFERENCE.md          ← Reference pointer to CLI manual
├── manual/
│   └── CLI_REFERENCE.md          ← Comprehensive CLI user manual
└── explanation/
    ├── kernel-vs-ide.md          ← Kernel primacy architecture
    ├── vds-state-machine.md      ← Finite state machine design
    └── agent-membrane.md         ← Cognitive membrane boundaries
```
