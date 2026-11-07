# Modular Language Bundles and Dynamic Process Packs Architecture

## Overview

Modern software engineering across polyglot repositories demands consistent enforcement of architectural invariants, AST hygiene policies, and verifiable static/dynamic quality gates. Rather than hardcoding language-specific linting rules, AST patterns, or test configurations into the core ZQK Knowledge Kernel binary, ZQK provides **Modular Language Bundles and Dynamic Process Packs**.

Language Bundles are portable, declarative packages encapsulating:
1. **Standards**: High-level engineering guidelines, code quality criteria, and severity classifications.
2. **AST Policies**: Declarative AST hygiene checks, forbidden invocation patterns, required docstrings, and struct invariants.
3. **Verification Gates**: Static and dynamic execution commands (e.g. `go test`, `cargo clippy`, `pytest`), execution environments, and CI enforcement toggles.
4. **Agent Directives**: Role-targeted operational rules mapped to autonomous subagents and IDE harnesses.
5. **Dependency Graphs**: Directed acyclic dependencies between foundational and specialized language bundles.

---

## 1. Architectural Topology & Component Layering

Modular Language Bundles decouple language-specific verification logic from kernel core execution:

```
┌────────────────────────────────────────────────────────────────────────┐
│                   CLI Command Plane: 'zqk bundle'                      │
│            (list, info <bundle>, add --file, remove <bundle>)          │
└────────────────────────────────────┬───────────────────────────────────┘
                                     │
                                     ▼
┌────────────────────────────────────────────────────────────────────────┐
│                Bundle Registry Engine (pkg/bundle)                     │
│  - Directory Isolation: .zqk/bundles/<name>/bundle.yaml                │
│  - Manifest Deserialization & Structural Validation                    │
│  - Path Traversal & Semantic Security Bounds Enforcement               │
│  - Cycle Detection & Self-Dependency Prevention                        │
└────────────────────────────────────┬───────────────────────────────────┘
                                     │
            ┌────────────────────────┼────────────────────────┐
            ▼                        ▼                        ▼
┌──────────────────────┐ ┌──────────────────────┐ ┌──────────────────────┐
│  AST Hygiene Engine  │ │  Verification Gates  │ │   Agent Directives   │
│ (Forbidden patterns, │ │ (Static / dynamic    │ │ (Persona bindings,   │
│  AST invariants)     │ │  CI gate execution)  │ │  IDE rules sync)     │
└──────────────────────┘ └──────────────────────┘ └──────────────────────┘
```

---

## 2. Manifest Schema (`bundle.yaml`)

Every bundle is defined by a canonical manifest satisfying the `LanguageBundleManifest` specification:

```yaml
schema_version: "1"
name: "golang-core"
language: "go"
version: "1.0.0"
description: "Core Go engineering standards, AST hygiene, and verification gates"

standards:
  - id: "GO-STD-001"
    title: "Explicit Error Handling"
    description: "Errors must be checked immediately and wrapped using errfmt"
    severity: "error"

ast_policies:
  - id: "AST-GO-NO-PANIC"
    rule_type: "forbidden_call"
    target_kind: "function_call"
    forbidden:
      - "panic"
      - "recover"

gates:
  - name: "static-analysis"
    command: "golangci-lint run ./..."
    enforce_ci: true
  - name: "unit-tests"
    command: "go test -v -race ./..."
    enforce_ci: true

agent_directives:
  - target_role: "Software Engineer"
    directive: "Always verify AST hygiene before marking backlog items shovel-ready"
```

---

## 3. Security Invariants & Fail-Closed Boundaries

The bundle subsystem strictly enforces runtime and filesystem security guarantees:

1. **Path Traversal Protection**:
   Bundle names containing relative directory traversal tokens (e.g. `../`, `..\\`) or forward/backward slashes are rejected with `ErrInvalidBundleName` to prevent arbitrary file deletion or escape from `.zqk/bundles/`.
2. **Self-Dependency and Circular Dependency Guards**:
   Manifests declaring a dependency on themselves or cycles are rejected at installation time.
3. **Atomic Installation and Uninstallation**:
   Manifests are written atomically into isolated workspace directories (`.zqk/bundles/<name>/bundle.yaml`), preventing partial state corruption.
4. **Strict Schema Validation**:
   Bundles lacking mandatory schema versions, names, languages, or semver-compliant version strings are rejected prior to persistence.

---

## 4. CLI Ergonomics & Operations

The `zqk bundle` command suite provides first-class management of installed packs:

- **List Bundles**:
  ```bash
  zqk bundle list
  zqk bundle list -f json
  ```
- **Inspect Manifest**:
  ```bash
  zqk bundle info golang-core
  zqk bundle info golang-core -f yaml
  ```
- **Install Pack**:
  ```bash
  zqk bundle add --file ./examples/bundles/golang-core.yaml
  ```
- **Uninstall Pack**:
  ```bash
  zqk bundle remove golang-core
  ```

---

## 5. Downstream Consumer Repository Integration

In consumer repositories (e.g., microservices, libraries, or multi-repo swarms), language bundles can be installed into `.zqk/bundles/`. During CI runs and continuous autonomous programming (CAP) execution loops, `zqk test` and `zqk system check` evaluate active bundle gates, ensuring complete compliance with organizational standards.
