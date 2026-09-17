# CLI Error Guard Options

**Last Verified:** 2026-08-31


This document explores options for **maintainability, readability, fluidity, and consistency** of the common "if condition then return error" pattern in CLI RunE handlers, and how they align with existing pipeline/builders in the codebase.

## Current Pattern

Today we use explicit if-blocks and `EnhanceError`:

```go
proc, err := cli.NewProcessor(cmd)
if err != nil {
    return cli.EnhanceError(cmd, fmt.Errorf("failed to create processor: %w", err))
}
// ...
if len(args) < 1 {
    return cli.EnhanceError(cmd, fmt.Errorf("object ID is required"))
}
```

This is clear but repetitive. We have fluent builders elsewhere (e.g. `CommandBuilder`, `HelpBuilder`, `ChainBuilder`) that reduce boilerplate via method chaining.

## Option 1: Guard Fluent Helper (Implemented)

**Location:** `internal/cli/guard.go`

A small fluent API that chains a command, optional error, optional requirement, and optional wrap format, then returns an enhanced error (or nil).

### Usage

**Error with wrap (replace if err + EnhanceError + fmt.Errorf):**

```go
proc, err := cli.NewProcessor(cmd)
if err != nil {
    return cli.Guard(cmd).Err(err).Wrapf("failed to create processor: %w").Return()
}
```

**Require a condition (single line, no if):**

```go
return cli.Guard(cmd).Require(len(args) >= 1, "object ID is required").Return()
```

**Plain error (no wrap):**

```go
return cli.Guard(cmd).Err(err).Return()
```

**Chain Require then Err (first failure wins):**

```go
return cli.Guard(cmd).
    Require(len(args) >= 1, "object ID is required").
    Err(procErr).Wrapf("failed to create processor: %w").
    Return()
```

(Note: chaining multiple `Err` from different steps usually requires separate Guard chains per step, since each step depends on the previous succeeding.)

### Benefits

- **Consistency** with existing builder style (chain then final `Return()`).
- **Less repetition**: no manual `fmt.Errorf` + `EnhanceError` in the handler.
- **Require(cond, msg)** gives a one-line, no-if validation return.
- **Single place** for “wrap + enhance” behavior.

### When to use

- Use **Guard** when you want a short, chainable way to “return enhanced error” or “require condition or return”.
- Keep **explicit if + EnhanceError** when you need extra logic in the error path (e.g. logging, different messages by branch).

---

## Option 2: Pipeline-Style “Then” Builders (Future)

Existing builders (e.g. spec builders, command builders) construct *data* or *commands*. A pipeline that encodes “do step; on error return” would need a different shape: each step returns `(value, error)` and the pipeline stops on first error.

Conceptually:

```go
// Not implemented – illustration only
result, err := cli.Pipeline(cmd).
    Then(createProcessor).           // (proc, err)
    Then(proc.Storage().Read).       // (obj, err)
    ReturnEnhancingErrors()
```

Challenges in Go:

- RunE is linear; we don’t have a single “pipeline result” type for all commands.
- Steps often need the *previous* value (e.g. `proc` from step 1), so a generic pipeline would need generics or `any` and type assertions.
- Existing RunE handlers are already sequential; a pipeline would need to be adopted gradually and might only pay off in the longest RunEs.

**Recommendation:** Keep Guard for local “check then return” patterns; consider a pipeline only if we see many long RunEs with the same “step1; step2; step3; return on first error” structure and want a shared abstraction.

---

## Option 3: Inline Helpers (No Chaining)

Minimal helpers without a builder:

```go
// ReturnEnhance(cmd, err) returns nil if err == nil, else EnhanceError(cmd, err)
return cli.ReturnEnhance(cmd, err)

// ReturnEnhancef(cmd, err, format, args...) wraps then enhances
return cli.ReturnEnhancef(cmd, err, "failed to create processor: %w")
```

Pros: very simple, no new types. Cons: no `Require(cond, msg)`, no chaining, and “first failure wins” across multiple checks would still be manual.

Guard subsumes this (e.g. `Guard(cmd).Err(err).Return()` or `Guard(cmd).Err(err).Wrapf("...").Return()`) and adds Require and a consistent style.

---

## Consistency and Policy

1. **Use one style per file:** Prefer either Guard or explicit if + EnhanceError in a given RunE; avoid mixing both for the same kind of check.
2. **Logging:** When you log before returning (e.g. `proc.Logger().LogError(...)`), keep the log call and then `return cli.Guard(cmd).Err(err).Wrapf("...").Return()` (or the equivalent if-block).
3. **Documentation:** `docs/CLI_HANDLER_PATTERNS.md` and this file are the source of truth for when to use Guard vs explicit if and for pipeline options.

---

## Summary

| Approach           | Use when                                           |
|--------------------|----------------------------------------------------|
| **Guard**          | You want a short, chainable “return enhanced” or “require or return” and are fine with a small fluent API. |
| **Explicit if**    | You need custom logic in the error path or prefer maximum explicitness. |
| **Pipeline (future)** | We introduce a shared “RunE pipeline” and have many long, similar RunEs. |

Guard is implemented and used in at least one command (e.g. `object get`) as a reference; adopt it elsewhere where it improves readability without hiding important behavior.
