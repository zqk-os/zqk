# Adaptive Agent Feedback and Progressive Disambiguation Engine

## Overview
This specification details the architecture for progressive failure disambiguation, negative constraint injection, pluggable diagnostic extractors, and loop stagnation protection within the ZQK Knowledge Operating System (KOS).

The primary architectural principle of this system is **public surface agnosticism**: external CLI flags, data schemas, and API contracts remain strictly generic and configurable, avoiding hardcoding project-specific or language-specific heuristics into the public runtime interface.

---

## 1. Structured Failure Attribution
When an agent or task execution fails, runtime signals are normalized into a project-agnostic `FailureEnvelope`:
- **Attempt**: 1-indexed attempt counter.
- **ExitCode**: Process exit code or subsystem status.
- **ErrorMessage**: Cleaned, bounded error summary.
- **Category**: Agnostic error taxonomy (`success`, `timeout`, `execution_failure`, `constraint_violation`, `context_cancelled`, `verification_failed`, `unknown`).
- **FailingConstraints**: Invariant or validation constraints that remained unsatisfied.
- **ExecutedActions**: Sequences of executed tools or actions preceding the failure.
- **Signature**: Deterministic, timestamp-invariant cryptographic hash of the failure's structural characteristics, allowing exact delta comparison across retry attempts.

---

## 2. Progressive Disambiguation & Negative Constraints
Rather than blind re-execution where identical attempts yield low probability of delta, subsequent attempts ($N > 1$) receive synthesized contextual enrichment:
- **Negative Constraints**: Identifies actions that produced failures in prior attempts and instructs the agent to avoid repeating the same paths without addressing the underlying constraint.
- **Diagnostic Differential**: Computes the entropy delta between attempt $N-1$ and attempt $N$. If the signature shifted, it isolates what changed; if identical, it signals zero new evidence.
- **Actionable Guidance**: Highlights unfulfilled constraints and directs the agent to pivot to alternative implementations.
- **Bounded Context**: Injected guidance is character- and token-bounded (`MaxPromptBytes`) to prevent runaway prompt bloating.

---

## 3. Pluggable Diagnostic Extractor Framework
Subsystems and language tooling can provide rich diagnostics without tightly coupling to the core kernel:
- **`Extractor` Interface**: Side-effect-free, context-aware diagnostic extraction contract (`Name()`, `Extract(ctx)`).
- **`DiagnosticExtractorRegistry`**: Deterministic, fail-closed registry executing extractors in stable, sorted order.
- **`WithTimeout` Wrapper**: Bounds individual extractor runtimes to ensure non-responsive extractors cannot block the execution loop.
- **`DiagnosticArtifact`**: Generic container (`SizeBytes`, `MIMEType`, `Payload`) for structured diagnostic outputs.

---

## 4. Adaptive State Checkpointing & Loop Stagnation Protection
To prevent thrashing or burning compute when an agent makes no progress:
- **Zero-Delta Detection**: Detects when consecutive retries produce identical failure signatures with zero introduced entropy.
- **Configurable Stagnation Policy**:
  - `MaxStagnantAttempts`: Maximum tolerable zero-delta attempts before escalation.
  - `ZeroDeltaFailFast`: Fail-fast trigger recommending strategy pivots on the first zero-entropy retry.
  - `MaxIdenticalSignatures`: Upper bound on repeated identical failures across the full retry history.
- **Actionable Recommendations**: Emits clear directives (`continue`, `pivot_strategy`, `escalate_to_supervisor`, `halt_loop`) to safeguard autonomous loops.
