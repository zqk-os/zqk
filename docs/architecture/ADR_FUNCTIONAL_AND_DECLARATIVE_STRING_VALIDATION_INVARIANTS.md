# ADR-0053: Declarative and Functional String Validation Invariants

## Status
Accepted

## Context
Across complex systems, imperative and procedural validation patterns such as:
```go
if strings.TrimSpace(value) == "" {
    return fmt.Errorf("field %s cannot be empty", fieldName)
}
```
recur repeatedly throughout command handlers, data parsing layers, and API adapters. This pattern introduces unnecessary boilerplate, increases cognitive overhead, and is prone to inconsistent formatting and omission of whitespace trimming.

To maintain clean, concise, and functional Go code, input validation must transition from procedural conditional checks to declarative, zero-allocation helper functions. Furthermore, the repository verification engine requires an automated static analysis rule to detect and flag legacy imperative patterns.

## Decision
1. **Declarative Validation Functions (`pkg/strutil` & `pkg/stringutil`)**:
   - Provide `ErrorOnBlank(val, format string, args ...any) error`:
     - Checks if trimmed string is empty.
     - Formats error message with optional format arguments if blank; returns `nil` if non-blank.
     - Zero allocations on the non-blank happy path.
   - Provide `RequireNonBlank(val, name string) error`:
     - Emits standardized `<name> cannot be blank` error.
   - Provide pure predicates `IsBlank(s string) bool` and `IsNonBlank(s string) bool`.
   - Expose identical signatures in `pkg/stringutil` as forwarding aliases to `pkg/strutil`.

2. **Automated Verification Rule in `zqk-vet`**:
   - Implement `CheckImperativeBlankValidation` AST checker under `hygiene/anti_pattern_blank_check`.
   - Scan Go ASTs for `strings.TrimSpace(...) == ""` conditions returning error constructors (`fmt.Errorf`, `errors.New`, `errfmt.Newf`).
   - Flag occurrences as actionable warnings during local `zqk-vet` checks and pre-push verification gates.

3. **Incremental Migration**:
   - Establish warning-level severity during initial rollout to prevent breaking existing build workflows.
   - Gradually migrate legacy procedural checks to functional calls during normal development cycles.

## Consequences
- **Positive**:
  - Consistent error formatting and mandatory whitespace trimming across all validation paths.
  - Reduced boilerplate and declarative code structure.
  - Proactive static analysis guidance preventing anti-pattern recurrence.
- **Negative**:
  - Requires developer awareness of the helper API across teams and autonomous subagent seats.
