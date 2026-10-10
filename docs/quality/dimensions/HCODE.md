# Continuous Verification Matrix: Dimension HCODE

## Overview
- **Code**: `HCODE`
- **Name**: Hardcoded Logic and Literal Eradication
- **Persona Owner**: [Hardcoding Eradication Czar](file:///.zqk/process/personas/PER-HARDCODING-ERADICATION-CZAR.yaml)
- **Policy Invariant**: `POL-CODE-1791593579304410000-e749bb5f`
- **Applicable File Classes**: `go_prod`, `go_test`, `script`, `config_file`

## Evaluation Standards & Check Rules

| Rule ID | Category | Check Description | Severity | Allowed Scope |
| :--- | :--- | :--- | :--- | :--- |
| `HCODE-001` | Absolute Paths | Forbidden user directories (`/Users/*`, `/home/*`, `/tmp/*` outside test sandbox). | Critical | Zero tolerance in prod/test |
| `HCODE-002` | Release Versions | Hardcoded version numbers (e.g. `2.9.4`, `0.1.0-beta.20`) embedded in source. | Critical | Zero tolerance in prod code |
| `HCODE-003` | Raw Permissions | Magic octal integer literals (`0755`, `0644`, `0700`) used directly. | High | Prohibited (use `paths.*` or `fileutil.*`) |
| `HCODE-004` | Fixed Ports & IPs | Hardcoded `localhost`, `127.0.0.1`, `:8080`, `:9000` bindings. | High | Configurable via flags/env |
| `HCODE-005` | String Literals | Duplicate string literals appearing $\ge 2$ times across the repository. | High | Must extract to shared package constant |

## Decoupled Blind Evaluation & Deduplication Inventory
1. **Blind Sensor Contract**: Evaluating agents parse source files and record all string literals, numeric literals, and path references as raw structured findings (`file`, `line`, `literal_value`, `category`).
2. **Invisible Policy Engine Scoring**: The verification engine aggregates findings into the repository-wide **Global String Literal Inventory**.
   - Single occurrences ($N=1$) are tolerated for localized logging and unique errors.
   - Multiple occurrences ($N \ge 2$) fail closed and mandate constant extraction.
