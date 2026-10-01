# Fail-Closed Audit Test Suite (`failclosed_audit`)

## Overview

This directory contains standalone, executable reference invariant tests designed to evaluate whether critical state transition gates and resource lifecycles adhere strictly to the **Fail-Closed Safety Mandate** (`POL-DEFAULT-60a14e239d552b9e`) and **Goroutine Resource Hygiene** (`POL-DEFAULT-7c873b7213847d78`).

---

## Core Invariants Tested

### 1. Invariant I2: Fail-Closed Gate Enforcement (`audit_test.go`)
- **Rule**: If an operation returns an error, the subsequent state transition must **never commit**.
- **Negative Boundary**: A gate implementation that logs an error but proceeds to return `result, nil` or mutates success state is classified as **fail-open (unsafe)** and rejected.
- **Shadowing Check**: Asserts that error variables in nested `if`/`for` blocks do not shadow outer return errors, which would cause deferred cleanup handlers to evaluate stale error states.

### 2. Invariant I1: Goroutine Resource Hygiene & Joinability
- **Rule**: Every long-lived background goroutine spawned by non-test code must possess a reachable context cancellation or channel close path.
- **Verification**: Tests verify that cancel propagation halts background workers within bounded timeouts without leaking goroutines or deadlocking.

---

## Running the Invariant Audit

To execute the fail-closed audit suite independently:

```bash
cd examples/swarms/code-eval/failclosed_audit
go test -v ./...
```
