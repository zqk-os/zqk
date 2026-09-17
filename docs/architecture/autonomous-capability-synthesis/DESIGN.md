# Autonomous Capability Synthesis: OrchestrationManager Design

**Last Verified:** 2026-08-31


## Overview
The `OrchestrationManager` (OM) is the core actor responsible for transforming raw system intent into formalized ZQK objects (`Goal` or `Workstream`). It bridges policy negotiation and semantic memory.

## Architecture
OM operates as an event-driven actor.

1.  **Intent Reception**: Consumes raw intent signals (e.g., system diagnostics, user commands).
2.  **Policy Validation**: Invokes `PolicyNegotiator` to verify if the intent aligns with current ZQK governance.
3.  **Context Retrieval**: Queries `MemoryStore` for relevant semantic patterns and historical outcomes.
4.  **Synthesis**: Uses the `SpecBuilder` pattern to assemble a validated `Goal` or `Workstream` object.
5.  **Lifecycle Initiation**: Commits the new object into the ZQK Knowledge Kernel.

## Proposed Interface
```go
package orchestration

type OrchestrationManager interface {
    // ProcessIntent evaluates, synthesizes, and commits a new capability.
    ProcessIntent(ctx context.Context, intent RawIntent) (CapabilityRef, error)
}

type RawIntent struct {
    Signature string
    Payload   map[string]any
    Priority  int
}

type CapabilityRef struct {
    ID   string
    Kind string
}
```

## Sequence Logic
1. `Signal` -> `OrchestrationManager`
2. `OrchestrationManager` -> `PolicyNegotiator.Validate(Intent)`
3. `PolicyNegotiator` -> `Result` (Allow/Deny)
4. If Allowed:
   - `OrchestrationManager` -> `MemoryStore.GetContext(Intent.Signature)`
   - `OrchestrationManager` -> `SpecBuilder.Synthesize(Intent, Context)`
   - `OrchestrationManager` -> `Kernel.Commit(Object)`
5. `OrchestrationManager` -> `NotifyStatus(Success/Failure)`
