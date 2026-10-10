# Continuous Verification Matrix: Dimension ERRHYG

## Overview
- **Code**: `ERRHYG`
- **Name**: Error Hygiene, Wrapping, and Fail-Closed Resilience
- **Applicable File Classes**: `go_prod`, `go_test`, `script`

## Evaluation Standards & Check Rules

| Rule ID | Category | Check Description | Severity | Allowed Scope |
| :--- | :--- | :--- | :--- | :--- |
| `ERRHYG-001` | Swallowed Errors | Explicitly ignoring fallible return values (`_ = doSomething()`). | Critical | Zero tolerance on mutations/IO |
| `ERRHYG-002` | Naked Errors | Using raw `errors.New` or unformatted `fmt.Errorf` without causal context. | High | Must use `errfmt.Wrap` or domain errors |
| `ERRHYG-003` | Fail-Closed Gating | Proceeding on partial failure states without explicit transactional rollback. | Critical | Fail-closed invariants mandatory |
| `ERRHYG-004` | Error Constants | Using magic string matching for errors (`strings.Contains(err.Error(), ...)`) instead of `errors.Is`. | Medium | Use typed errors or `errors.Is` |
