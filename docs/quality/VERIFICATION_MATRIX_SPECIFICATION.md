# Continuous Source Verification Matrix (CSVM) Specification

## 1. Executive Summary & Core Philosophy
The **Continuous Source Verification Matrix (CSVM)** provides universal, file-level quality and architectural invariant tracking across all source code and project artifacts in ZQK.

Unlike transient, prompt-promiscuous chat harnesses that forget criteria across iterations, the CSVM couples:
1. **Content-Addressable Verification (`evidence_sha`)**: Every file is tracked by its SHA-256 hash. Once a file passes an evaluation dimension, its status is stamped and cached. Clean files achieve **$O(1)$ instant skips (0 ms, 0 tokens)** on future runs until their content hash changes.
2. **Standard Evaluation Dimension Codes**: Homogeneous check suites codified directly in the Knowledge Kernel:
   - `HCODE`: Hardcoded Logic, Magic Literals, Raw Permissions, Fixed Paths, and Static Versions.
   - `EFFPERF`: Efficiency, Complexity, Resource Hygiene, Context Deadlines.
   - `ERRHYG`: Error Hygiene, Wrapping, and Fail-Closed Resilience.
   - `CONCURR`: Named Goroutines (`goroutinelabels`), Lifecycle Termination, Deadlock Safety.
   - `SECOBS`: Zero-PII, Secret Eradication, and Structured Logging.
   - `DOCSIG`: API Contracts and 100% Exported Symbol GoDoc Coverage.
3. **Programmatic Writer Gating**: The matrix ledger cannot be manually edited by agents or users. All updates must be stamped programmatically via `zqk matrix stamp` or evaluated via `zqk matrix verify`, verified against the target file's content hash.
4. **Decoupled 3-Party Quality Loop**:
   - **Blind Sensor (Evaluator Agent / Czar)**: Scans source files and extracts raw structured findings (line, count, category, literal value) without knowing acceptance thresholds.
   - **Policy Engine (Central Matrix)**: Invisibly scores findings against authoritative thresholds, maintains the **Global String Literal Inventory**, and computes Diamond Scores (1 to 5 diamonds).
   - **Fixer Agent**: Remediates flagged violations using structured sensor findings, refactoring code and submitting it for re-evaluation.

---

## 2. Kernel Policy & Persona Traceability

| Dimension / Policy | Kernel Object ID | Enforcement | Status |
| :--- | :--- | :--- | :--- |
| **Hardcoding Eradication Czar** | `PER-HARDCODING-ERADICATION-CZAR` | Adversarial Sensor | Approved |
| **HCODE Dimension** | `POL-CODE-1791593579304410000-e749bb5f` | Automated / Critical | Active |
| **EFFPERF Dimension** | `POL-CODE-1791593697272400000-9b0f833b` | Automated / High | Active |
| **ERRHYG Dimension** | `POL-CODE-1791593699894406000-cd6f25fd` | Automated / High | Active |
| **CONCURR Dimension** | `POL-CODE-1791593703788100000-9f2dff69` | Automated / Critical | Active |
| **SECOBS Dimension** | `POL-CODE-1791593707127249000-965eda9e` | Automated / Critical | Active |
| **DOCSIG Dimension** | `POL-CODE-1791593709986529000-029390e6` | Automated / Medium | Active |
| **Programmatic Writer Policy** | `POL-CODE-1791593713535339000-77a9d462` | Automated / Critical | Active |
| **Blind Evaluation Policy** | `POL-CODE-1791593716794368000-efa51192` | Automated / High | Active |

---

## 3. Global String Literal Deduplication Engine
To eliminate rampant hardcoding while accommodating legitimate, unique log statements and messages:
1. Every string literal found in source files is indexed alphabetically in `.zqk/literal_inventory.json`.
2. **Tolerance Rule**: Exactly **1 unique occurrence** of a string literal is tolerated globally across the codebase.
3. **Violation Threshold**: If an identical string literal appears **$\ge 2$ times** across different call sites or files without being declared as a shared package constant, it triggers an immediate deduplication failure.

---

## 4. Diamond Scoring Rubric (1-5 Diamonds)
- **◆◆◆◆◆ (5 Diamonds - Flawless)**: 0 violations, clean invariant adherence, optimal documentation.
- **◆◆◆◆◇ (4 Diamonds - Minor Polish)**: 0 critical/high violations; $\le 2$ minor notices (e.g. single-use log string without deduplication threat).
- **◆◆◆◇◇ (3 Diamonds - Pass with Remediation Backlog)**: Minor issues present, non-blocking for local development, blocking for production release.
- **◆◆◇◇◇ (2 Diamonds - Failing)**: 1 or more high violations (e.g. raw permissions, missing context timeout).
- **◆◇◇◇◇ (1 Diamond - Critical Failure)**: Critical violations (e.g. hardcoded release version, naked goroutine, hardcoded secret, swallowed error).
