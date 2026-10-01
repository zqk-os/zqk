# ZQK Agent Skills Architecture: Evolutionary Breeding & Generative Mutation

This package tree (`pkg/skills`) implements autonomous agent skill maintenance, fitness-driven evolution, and generative synthesis within the ZQK Knowledge Kernel.

---

## 1. Architectural Overview

Agent skills in ZQK are first-class kernel objects (`kind: agent_skill`) backed by executable or procedural Markdown instructions (`SKILL.md`) or Go code. Over the lifecycle of swarm operations, skills are evaluated by automated execution monitors (`maturation_report`, `metrics_feedback`, and VDS validation passes).

```
   ┌─────────────────────────────────────────────────────────────────────────────┐
   │                            Kernel Event Mesh                                │
   └──────────────────────────────────────┬──────────────────────────────────────┘
                                          │ EventTypeObjectMutated (maturation_report)
                                          ▼
                         ┌─────────────────────────────────┐
                         │   breeding.Listener (Ambient)   │
                         └────────────────┬────────────────┘
                                          │ Triggers on mutation
                                          ▼
                         ┌─────────────────────────────────┐
                         │         breeding.Engine         │
                         │  - Scans agent_skill objects    │
                         │  - Evaluates fitness_score      │
                         │  - Identifies fitness < T       │
                         └────────────────┬────────────────┘
                                          │ Passes originalFilePath + feedback
                                          ▼
                         ┌─────────────────────────────────┐
                         │     breeding.MutationProvider   │
                         │     (breeding.LLMSkillMutator)  │
                         │  - Synthesizes improved skill   │
                         │  - Fallback: deterministic tag  │
                         └────────────────┬────────────────┘
                                          │ Creates new fork
                                          ▼
                         ┌─────────────────────────────────┐
                         │  New agent_skill Object Minted  │
                         └─────────────────────────────────┘
```

---

## 2. Package Decomposition

### `pkg/skills/breeding`
- **`Engine`**: Scans all registered `agent_skill` objects and inspects their linked `maturation_report` objects. If any skill exhibits a `fitness_score` strictly lower than the configured threshold (`fitnessThreshold`, e.g. `0.70`), it accumulates feedback from both `maturation_report` and `metrics_feedback` and invokes the `MutationProvider`.
- **`MutationProvider`**: Interface defining `MutateSkill(ctx context.Context, originalFilePath string, feedback []string) (string, error)`.
- **`LLMSkillMutator`**: Production implementation of `MutationProvider`. Employs ZQK's core `llm.Client` to optimize skill instructions and code. In airgapped or offline mode, falls back to a deterministic, timestamped fork preserving audit feedback.
- **`Listener`**: Event-driven subscriber to the ZQK kernel event router. Intercepts `EventTypeObjectMutated` events for `maturation_report` kinds, allowing the breeding loop to operate autonomously in the background without polling.

### `pkg/skills/mutation`
- **`Engine`**: Generative multi-parent skill synthesis. Merges multiple parent skill implementations into a unified child skill, performs semantic validation via `SemanticEngine`, and generates technical documentation.
- **`SkillSynthesisClient` (`llm_client.go`)**: Concrete adapter connecting ZQK's unified LLM client (`pkg/llm.Client`) to generative skill synthesis. Provides customizable system prompts (`DefaultSystemPromptCode`, `DefaultSystemPromptDocs`) and sanitizes markdown fence wrappers from language model completions to return pristine, compilable Go source.

---

## 3. Operational Invocations

- **Manual Optimization**: `zqk ops level-up <agent_skill_id>` evaluates and reports optimization targets for a given skill.
- **Autonomous Evolution**: `zqk system evolve` triggers autonomous skill recombination and evolutionary convergence across active swarm seats.
- **Cryptographic Sealing**: `zqk system seal-skill <skill_name>` signs and validates skill immutability via cryptographic digest headers in `SKILL.md`.
