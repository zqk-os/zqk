# Continuous Verification Matrix: Dimension SECOBS

## Overview
- **Code**: `SECOBS`
- **Name**: Zero-PII Security and Structured Observability
- **Applicable File Classes**: `go_prod`, `go_test`, `script`, `config_file`

## Evaluation Standards & Check Rules

| Rule ID | Category | Check Description | Severity | Allowed Scope |
| :--- | :--- | :--- | :--- | :--- |
| `SECOBS-001` | Secret Leakage | Plaintext API tokens, private keys, passwords, or bearer tokens in code. | Critical | Forbidden (use secret managers or env vars) |
| `SECOBS-002` | PII Exposure | Logging email addresses, usernames, home directories, or IP addresses. | High | Must be anonymized / redacted |
| `SECOBS-003` | Structured Logging | Emitting unparsed printf strings instead of structured key-value log fields. | Medium | Use structured logger with context |
| `SECOBS-004` | Audit Emission | State mutations occurring without change journal or audit log events. | High | Mandatory audit logging |
