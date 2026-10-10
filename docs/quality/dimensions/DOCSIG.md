# Continuous Verification Matrix: Dimension DOCSIG

## Overview
- **Code**: `DOCSIG`
- **Name**: API Signatures, Documentation, and Contracts
- **Applicable File Classes**: `go_prod`

## Evaluation Standards & Check Rules

| Rule ID | Category | Check Description | Severity | Allowed Scope |
| :--- | :--- | :--- | :--- | :--- |
| `DOCSIG-001` | Exported Symbols | Exported types, interfaces, functions, and methods without GoDoc comments. | High | 100% GoDoc coverage required |
| `DOCSIG-002` | Contract Drift | GoDoc comments referencing non-existent parameters, flags, or return values. | Medium | Documentation must match code |
| `DOCSIG-003` | Stale Examples | Code examples in documentation failing execution or compile checks. | High | All examples must be verified |
