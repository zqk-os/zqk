# MNT — Code Quality & Maintainability Evaluation (code-eval / PRI-CODE_EVAL)

Evaluator: specialist_evaluator
Scope: `pkg/audit`, `pkg/interactionpolicy` (and AST census of `pkg/...`)
Method: evidence-driven read of live package surfaces + AST hits (see task context)

## 1. Architecture & Package Boundaries

**Findings**
- `pkg/audit` (policy engine + event stream) and `pkg/interactionpolicy` (event
  catalog evaluation, ambience detection) are cohesive single-purpose packages.
  Good boundary: audit exposes a `Policy` interface + engine, no domain leakage.
- `Evaluate(map[string]any, string) Result` in `pkg/interactionpolicy/evaluate.go`
  accepts an untyped `map[string]any` event payload. This is the weakest typing
  seam in the packages inspected: callers must know the key schema by convention
  only. **Severity: Medium.**
  Recommendation: introduce a typed event struct (e.g. `type Event struct {
  Type, Persona, ... string }`) and keep the map variant as a transitional
  adapter that decodes into it.
- `ambience.go` functions also take `[]map[string]any` snapshots
  (`DetectStaleInProgress`, `DetectStaleAgentTasks`). Same recommendation: typed
  input struct (`TaskSnapshot`) rather than raw maps; `stringSliceFromAny` is a
  symptom of the untyped seam, not a defect in itself.

**Verdict:** Boundaries sound; primary risk is the untyped cross-package data
contract (map[string]any) rather than layering violations.

## 2. Code Craftsmanship

Findings (by file, from AST/read evidence):
1. `pkg/interactionpolicy/catalog.go` — `stepsForEvent` is table-driven over an
   event catalog; pattern is consistent (`Step` structs, `catalogEntry`). Good.
2. `pkg/interactionpolicy/ambient_drive.go:47-66` — `Hint*` functions are
   one-liner string templates sharing no constants for repeated literals.
   DRY opportunity, **Medium**. Implemented in this run: literals extracted to
   `hint_constants.go` + tests.
3. `pkg/audit/stream.go` — `NewAuditStream(*ast.Ellipsis)` and
   `Subscribe(...) *ast.ChanType` surface Go AST types through the public API.
   This is a **High** maintainability smell in production code: the API is
   anchored to `go/ast`, which prevents the caller from using normal Go channels
   and implies a code-generation/proxy artifact was committed as a public surface.
   Recommendation: regenerate or re-abstract the stream API behind
   `<-chan AuditRecord`. (Flagged; not fixed here — would touch external callers.)
4. `pkg/audit/policies.go` — registry + fail-fast validate loop is clean;
   `PolicyViolationError` carries a message; `Name()`-keyed registration.
   No observed leak (no goroutines started by the engine itself). **Low** —
   consider `RegisterPolicy` returning an explicit error on duplicate names
   instead of silent overwrite (fail-closed consistency with
   POL-DEFAULT-60a14e239d552b9e).

## 3. Resource Hygiene & Goroutine Safety (POL-DEFAULT-7c873b7213847d78)

- `pkg/audit/stream.go` exposes `Subscribe(ctx)`: test
  `TestAuditStream_UnsubscribeOnCancel` demonstrates cancel-driven
  unsubscription, i.e. subscriber lifecycle is ctx-bound. Positive evidence.
- No unbounded channels or detached goroutines observed in the inspected
  packages. **Pass (scoped to pkg/audit, pkg/interactionpolicy).**

## 4. Test Coverage & TDD Signals (POL-DEFAULT-f746cccc03ce5694)

- `pkg/audit`: `policies_test.go` (mockPolicy, Validate), `stream_test.go`
  (publish/subscribe + unsubscribe-on-cancel). Core behaviors covered.
- `pkg/interactionpolicy`: `evaluate_test.go` (default catalog, persona filter,
  unknown event), `ambience_test.go`, `ambient_drive_test.go` (8+ cases incl.
  edge: missing align, draft plane, shaping order). Healthy table-driven style.
- Gap: no observed test for `stringSliceFromAny` malformed inputs
  (nil / non-[] / nested). **Medium.** Covered partially by this run's new
  tests for the hint constants module only.
- Mocks are minimal interface implementations (good) rather than heavy
  frameworks.

## 5. Failure / Invariant Mode (POL-DEFAULT-60a14e239d552b9e)

- `PolicyEngine.Validate` returns first violation error — caller-enforced
  fail-closed depends on callers checking the error; the engine itself does not
  panic. Acceptable for a validator, but a sentinel `ErrNoPolicies` or
  explicit "empty engine rejects" policy would tighten the invariant. **Low.**
- `Evaluate(...).Result` in `interactionpolicy` appears to return a struct
  rather than an error; unknown events are handled via
  `TestEvaluate_UnknownEvent` — verify the returned `Result` is consumed as
  fail-closed (denied) by CLI call sites. **Low / verify.**

## 6. Severity Summary

| Sev  | Issue                                                        | Where                    | Action          |
|------|--------------------------------------------------------------|--------------------------|-----------------|
| High | Public API exposing `go/ast` types (`NewAuditStream`, `Subscribe`) | pkg/audit/stream.go      | Follow-up task  |
| Med  | Untyped `map[string]any` event/record seams                  | interactionpolicy, audit | Refactor plan   |
| Med  | Duplicated string literals in Hint* builders                 | ambient_drive.go         | **Fixed (this run)** |
| Med  | No tests for malformed `stringSliceFromAny` input            | ambience.go              | Backlog (BLI)   |
| Low  | Duplicate policy registration overwrites silently            | audit/policies.go        | Backlog (BLI)   |
| Low  | Empty policy engine semantics unspecified                     | audit/policies.go        | ADR / policy    |

## 7. Remediation implemented in this run

- `pkg/interactionpolicy/hint_constants.go`: extracted repeated hint
  string literals into exported constants (DRY, single source of truth for
  feed strings).
- `pkg/interactionpolicy/hint_constants_test.go`: verifies constants are
  non-empty, pairwise distinct, and stable (no accidental shared values).

## 8. Recommendation & Next Steps

1. **Plan-scoped branch / PR**: land constants refactor behind
   `go build` + `go test ./pkg/interactionpolicy/...` gating (TDD policy).
2. File follow-up BLIs:
   - `pkg/audit/stream.go` public API should not expose `go/ast` types.
   - Typed event/snapshot structs to replace `map[string]any` seams.
   - Test hardening for `stringSliceFromAny` malformed inputs.
   - `RegisterPolicy` duplicate-name handling (deny or error, not silent
     overwrite).
3. Do not treat this summary as closed — per Flywheel policy
   (POL-DEFAULT-ec4baffc4448367c), the above follow-ups should be picked up
   as separate tasks in the same plan, not deferred to manual triage.
