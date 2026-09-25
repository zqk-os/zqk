---
name: community-functional-craftsman
description: Functional, highly readable, maintainable Go code. Zero magic literals, DRY abstractions, fluent builders/predicates, robust concurrency and memory hygiene.
---

# Elite Functional & Readable Code Craftsmanship

## Core Objective
Produce Go code that reads like clear English prose, eliminates visual noise, maximizes maintainability, and prevents defect propagation. Turn ugly, defensive type assertions and nested conditional death-chains into elegant, declarative, functional pipelines.

---

## 1. Zero Magic Literals & Absolute DRY (Don't Repeat Yourself)
- **Single Source of Truth**: NEVER hardcode a string literal, field key, or magic number more than once in the codebase.
- **Constant Hoisting**: Every identifier, status string, error message prefix, parameter name, configuration key, or numeric threshold MUST be bound to an exported package constant or domain-level enum.
- **Domain Key Re-use**: Reuse existing keys from canonical definitions (e.g. `objects.FieldKey*`, `objects.ObjectStatus*`, `objects.Kind*`). If a key or constant is missing, define it once in the appropriate constants file and import it.

---

## 2. Eliminate Defensive Type-Assertion Clutter (KernelObjectInspector / Fluent Wrappers)
- **Forbidden Anti-Pattern**:
  ```go
  // UNACCEPTABLE: Ugly, noisy, repetitive type casting
  if st, _ := item[objects.FieldKeyStatus].(string); st != "in_progress" {
      item[objects.FieldKeyStatus] = "in_progress"
  }
  ```
- **Mandated Ergonomic & Functional Pattern**:
  ```go
  // ACCEPTABLE: Declarative, readable, self-documenting
  if !koi.IsStatus(item, objects.ObjectStatusInProgress) {
      koi.SetStatus(item, objects.ObjectStatusInProgress)
  }
  // OR fluent wrapper:
  if koi.Wrap(item).NotInProgress() {
      koi.Wrap(item).SetStatus(objects.ObjectStatusInProgress)
  }
  ```
- Any repetitive access to maps (`map[string]any`, `map[string]string`) or deeply nested structs MUST be wrapped in safe, nil-tolerant helper functions (e.g. `koi.Status()`, `koi.ID()`, `koi.GetString()`, `koi.GetStringSlice()`).

---

## 3. Kill the "If / Else If / Else" Death Chains: Functional Predicates & Builders
- **Linear Sentences Over Nested Trees**: Nesting past 2 levels is an architectural defect. Refactor branch labyrinths into early returns, guard clauses, or functional predicates.
- **Predicates & Higher-Order Filters**:
  Instead of manual iterative loops with complex conditional checks:
  ```go
  // UNACCEPTABLE
  var eligible []Item
  for _, it := range items {
      if it != nil && it.Status == "ready" && it.Priority <= 2 {
          eligible = append(eligible, it)
      }
  }
  ```
  Adopt functional combinators:
  ```go
  // ACCEPTABLE
  eligible := slices.Filter(items, func(it Item) bool {
      return it != nil && it.IsReady() && it.IsHighPriority()
  })
  ```
- **Fluent Builder Chains**: When configuring objects, composing pipelines, or executing validations, use fluent method chains:
  ```go
  pipeline.ForBacklogItem(item).
      Require(HasReadyCriteria).
      Require(HasBoundTestCases).
      OnFailure(LogTDDHold).
      Execute(TransitionToInProgress)
  ```

---

## 4. Optionals, Result Types, & Error Encapsulation
- Avoid returning `(value, bool)` or `(value, error)` where the caller must write 5 lines of defensive checks just to discard or re-wrap.
- Use explicit helper constructors that encapsulate default fallback values:
  `koi.GetStringOr(item, objects.FieldKeyTitle, "Untitled")`
  `koi.GetIntOr(item, "retry_count", 3)`
- Keep errors actionable, structured, and contextual without obscuring the happy-path narrative of the function.

---

## 5. Concurrency & Memory Best Practices
- **Explicit Ownership & Lifecycles**:
  - Always pair resource acquisition (`os.Open`, `net.Dial`, channel creation, lock acquisition) with `defer` immediately on the next line.
  - Never launch un-tracked or unbounded goroutines. Use `sync.WaitGroup`, errgroups, or bounded worker pools with cancellation propagation via `context.Context`.
- **Thread Safety by Encapsulation**:
  - Encapsulate sync primitives (`sync.RWMutex`, `sync.Mutex`) behind methods; never expose raw locks across package boundaries.
  - Read-heavy caches must use `RWMutex` with scoped read-locks (`RLock()` / `RUnlock()`).
- **Memoization & Allocation Hygiene**:
  - Pre-allocate slice capacity when length is known: `make([]T, 0, len(source))`.
  - Cache expensive parsing or compilation (e.g. regex compilation, schema validation models) in `sync.Once` or package-level caches.
