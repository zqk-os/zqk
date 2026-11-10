# Automated Evaluation Failure Remediation Synthesizer & Defect Auto-Minting Gate

## 1. Overview & Problem Statement
During autonomous multi-model evaluation and continuous convergence sessions, scenario failures provide rich diagnostic telemetry. Without automated objectification, failures either require manual human triage or risk being dropped across session resets. Furthermore, uncoordinated retry loops risk spamming the persistent Knowledge Kernel with duplicative defect tickets for the same underlying failure mode.

The **Automated Evaluation Failure Remediation Synthesizer** bridges the runtime evaluation engine (`pkg/eval`) and the persistent Knowledge Kernel CAS (`pkg/objects`). When a scenario evaluation fails or violates quality thresholds, the synthesizer deterministically extracts the failure signature, checks for existing active remediations, and automatically mints an idempotent `technical_debt` or remediation object in the Knowledge Kernel.

## 2. Architectural Topology

```
+-------------------------------------------------------------+
|                     pkg/eval Subsystem                      |
|                                                             |
|   +-----------------------+      +----------------------+   |
|   |  BatchScenarioRunner  | ---> |     AutoEvalGate     |   |
|   +-----------------------+      +----------------------+   |
|               |                             |               |
|               v (failed outcomes)           v (violations)  |
|   +-----------------------------------------------------+   |
|   |             RemediationSynthesizer                  |   |
|   |  - Normalized Fingerprinting (SHA-256)              |   |
|   |  - Idempotency & Deduplication Filter               |   |
|   |  - Target Kind & Severity Classification            |   |
|   +-----------------------------------------------------+   |
+------------------------------|------------------------------+
                               |
                               v (auto-mint)
+-------------------------------------------------------------+
|              Knowledge Kernel Persistent CAS                |
|                                                             |
|   +----------------------+      +-----------------------+   |
|   | technical_debt (CAS) |      |   remediation_task    |   |
|   +----------------------+      +-----------------------+   |
+-------------------------------------------------------------+
```

## 3. Data Invariants & Schema Contract

### 3.1 Failure Fingerprint
Every evaluation failure is mapped to a deterministic SHA-256 fingerprint:
$$\text{Fingerprint} = \text{SHA256}(\text{ScenarioID} \parallel \text{ModelName} \parallel \text{NormalizedErrorMessage})$$
Normalized error messages strip transient timestamps, ephemeral memory addresses, and session IDs to guarantee reproducibility across multiple test runs.

### 3.2 Synthesizer Payload Invariants
A minted remediation object MUST satisfy:
1. `title`: Standardized descriptor prefixed by `[EVAL-FAILURE] <ScenarioID>: <ShortFailureReason>`.
2. `severity`: Deterministically mapped from failure category:
   - Boundary violation / panic: `P0` / `critical`
   - Assertion mismatch: `P1` / `high`
   - Latency / soft timeout: `P2` / `medium`
3. `remediation_payload`: Enclosed metadata containing `scenario_id`, `model_name`, `error_signature`, and `fingerprint`.
4. `code_location`: Bound to the scenario test path or target code location.

## 4. Idempotency & Deduplication Gate
Prior to object creation, the synthesizer checks the active working set for existing remediations sharing the same failure fingerprint:
- If an open `technical_debt` or `backlog_item` already exists with status `originated`, `planned`, or `in_progress`, the synthesizer records telemetry but skips duplicate creation.
- If a prior remediation is marked `resolved` or `complete`, a new failure triggers a reopened or regression-flagged remediation.

## 5. Verification & Three-Fold Proof
1. **Static Floor Invariant**: Validates remediation schema structure, required fields, and non-empty failure fingerprints.
2. **Operational Dynamic Proof**: Executes synthetic failed scenario run and verifies automated minting of valid CAS object.
3. **Negative Adversarial Boundary**: Ingests duplicate, malformed, and empty failure traces; proves zero duplicate object minting and fail-closed error containment.
