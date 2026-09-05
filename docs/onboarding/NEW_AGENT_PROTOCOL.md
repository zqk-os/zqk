# New Agent Protocol

## 1. Introduction

To avoid "split-brain" syndrome and ensure that all intelligent agents operating within the ZQK ecosystem are fully integrated, we adhere to the "Kernel First" principle. All new agents must be registered and initialized through the ZQK knowledge kernel before they are considered operational.

**First contact (workspace):** Before or immediately after creating a persona, run **`zqk system agent-onboard`** so vendor directives and default seating stay aligned with the kernel. See [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md) and [`../strategy/open-core/AGENT_ONBOARDING_SEQUENCE.md`](../strategy/open-core/AGENT_ONBOARDING_SEQUENCE.md).

## 2. Core Principles

- **Kernel First:** An agent does not exist until its persona and assessment rating are mapped into the knowledge graph.
- **Traceability:** Every action an agent takes must be traceable back to its origin and the goal it is attempting to fulfill. This requires the agent identity and capabilities to be formally defined.
- **Continuous Improvement:** By establishing a unified assessment rating for each persona, we can track performance over time, making adjustments and fine-tuning until optimal performance is achieved.
- **No vendor brain as source of truth:** Regenerable rules files (`.cursorrules`, `AGENTS.md`, …) come from `agent-onboard` / `generate-agent-configs`, not hand-edited lore that drifts from CAS.

## 3. The `zqk agent new` Command

To streamline the onboarding process and guarantee adherence to this protocol, all new agents must be created using the `zqk agent new` command.

This command utilizes the ZQK persistence bundle functionality to execute a "one-shot" creation of all required system objects.

### What the Command Does:

When `zqk agent new <persona_name>` is executed, it atomically generates and persists the following objects as a single bundle:

1.  **Persona Object:** Defines the agent identity, role, and overarching purpose.
2.  **Assessment Rating Object:** Initializes a baseline performance rating to track the agent effectiveness and guide future fine-tuning.
3.  **Agent Skill Objects (Placeholders):** Generates initial placeholders for the specific skills the agent requires, linking them directly to the persona.

## 4. Enforcement

No agent should be deployed or granted execution capabilities without first having its foundational objects successfully persisted in the kernel via this protocol. Any un-mapped agents will be isolated from system resources.

Default community seating (`PER-DEFAULT-*`) is seeded by **`zqk system init`** and repairable via **`zqk system seed-default-agent-seating`** or the seat stage of **`zqk system agent-onboard`**.
