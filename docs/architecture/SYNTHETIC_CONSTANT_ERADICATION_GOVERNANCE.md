# Synthetic Constant Eradication & Code Readability Governance

## Overview

This architecture document defines governance policies, invariant rules, and enforcement mechanisms against synthetic, auto-generated pseudo-constants (e.g., `ConstMagic[0-9a-fA-F]{8}`) across the ZQK codebase, resolving Technical Debt `TDE-F-CQ-002`.

## Background & Problem Statement

Prior to this specification, automated refactoring and constant extraction tools populated over 3,500 synthetic pseudo-constants (such as `ConstMagic437c4f93`, `ConstMagic129864b4`, `ConstMagicExtracted_*`) across validation, storage, and CLI packages.

This mechanical indirection introduced severe architectural and maintainability detriments:
1. **Semantic Obfuscation**: Constants with non-descriptive hash suffixes obscured the meaning and intent of error messages, regex patterns, and string literals.
2. **Impaired Grepability and Navigability**: Developers and static analyzers could not grep for user-visible error strings, log formats, or enum values directly in the source code.
3. **AST Bloat and Compiler Overhead**: 900+ line artificial constant tables (`constants_internal.go`, `constants_internal_messages.go`) inflated package graphs without providing abstraction benefits or multi-site reuse.
4. **False Abstraction**: Single-use formatting strings and test assertion descriptions were separated from their call sites, harming local readability.

## Architectural Governance: Inlining & Naming Policies

### 1. Inlining of Single-Use Literals
Single-use error messages, test failure descriptions, log format strings, and local regex patterns must remain as readable inline string literals or contextual format strings at their respective call sites.

### 2. Strict Criteria for Named Constants
Named constants are strictly reserved for:
- **Domain Enumerations**: Defined status strings, kind names, and category values (e.g., `pkg/objects`, `pkg/kindnames`).
- **Standard Protocol Headers & Keys**: HTTP headers, IPC message types, and metadata attributes.
- **Shared Configuration Keys**: Configuration property names shared across multiple distinct subsystems.

Every named constant must have a clear, descriptive, and human-readable identifier conveying its domain purpose (e.g., `MetricNameAsyncValidator`, `ErrMsgCreateCacheDir`).

### 3. Absolute Prohibition of Opaque Identifiers
Identifiers matching the following synthetic patterns are strictly prohibited from entering the repository:
- `ConstMagic*`
- `ConstMagicExtracted_*`
- Any variable, constant, or function name containing mechanical MD5/SHA hash segments or opaque hex fingerprints.

## Automated Verification & Enforcement

The repository enforces synthetic constant eradication through a Three-Fold Proof:

1. **Static Floor (`TestConstMagicEradication_StaticFloor`)**: Scans all source and test files across the subsystem to assert zero occurrences of `ConstMagic` identifiers.
2. **Operational Proof (`TestConstMagicEradication_OperationalProof`)**: Validates that all inlined literals function correctly in runtime validation, diagnostics, and test suites.
3. **Negative Boundary (`TestConstMagicEradication_NegativeBoundary` & ASTAuditor)**: The AST structural auditor (`pkg/validation/qa/ast_audit.go`) inspects constant declarations and flags any synthetic pseudo-constant as a blocking `code_quality_debt` violation.
