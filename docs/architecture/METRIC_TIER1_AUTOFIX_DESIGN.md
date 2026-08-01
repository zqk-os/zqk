# Metric Tier-1 Auto-Fix Design

**Purpose:** Define semantics and safe defaults for resolving Tier-1 instance_validation issues on internal metric kinds (e.g. `audit_aggregation_metric`) so auto-fix can be extended in a rule- and spec-driven way without guessing.

**Related:** [AUTO_FIX_PATTERN.md](../process/architecture/AUTO_FIX_PATTERN.md), [SPEC_BASED_AUTO_FIXER_PLAN.md](../process/architecture/SPEC_BASED_AUTO_FIXER_PLAN.md), system-check baseline, "Store only what's necessary" (LESSONS_LEARNED).

---

## 1. Scope

- **Kinds:** `audit_aggregation_metric` (and any other internal metric kinds that report Tier-1 missing required fields).
- **Issue pattern:** `instance_validation` — "Field X is required" with `auto_fixable: true`.
- **Current state:** ~482 Tier-1 issues; many are `audit_aggregation_metric` missing: `first_seen`, `aggregation_window_start`, `aggregation_window_end`, `schema_version`, `metric_type`, `event_type_counts`.

---

## 2. Field-by-Field Decisions (audit_aggregation_metric)

| Field | Required | Safe default? | Recommendation |
|-------|----------|---------------|----------------|
| **schema_version** | Yes | Yes | Use spec default `"2.0.0"` (spec is the source of truth). Add to spec `validation.default` or apply in auto-fixer when kind is audit_aggregation_metric and field missing. |
| **metric_type** | Yes | Yes | Spec checklist already says `default: system`; value must be `"system"`. Safe to set `"system"` for this kind. |
| **event_type_counts** | Yes | Yes (sentinel) | Use empty object `{}` to mean "no breakdown available." Spec validation is "object with string keys and integer values"; empty object is valid. Prefer regeneration when audit stream is available so real counts can be filled. |
| **aggregation_window_start** | Yes | No | Must be ISO 8601; "auto-populated from oldest event in window." No safe generic default. **Options:** (a) Regenerate from audit stream for that window, or (b) Leave as manual / auto_fix_rule that sets a placeholder only when product approves (e.g. use metric `created_at` if present). |
| **aggregation_window_end** | Yes | No | Same as start; "auto-populated from newest event in window." Regeneration or approved placeholder only. |
| **first_seen** | Yes (base_metric) | No | "Auto-populated on first collection." No safe generic default. **Options:** (a) Use object `created_at` if available and valid ISO 8601, or (b) Regeneration, or (c) Leave for manual fix. |

---

## 3. Regeneration Policy

- **When regeneration is allowed:** For internal metrics that are derived entirely from system data (e.g. audit event aggregation), product/owner may approve "regenerate from raw audit stream" for a given metric or batch. That implies: re-run aggregation for the same time window and overwrite the metric with the result (so all required fields are populated correctly).
- **When not to auto-fill:** Do not invent timestamps or counts that are not derived from real data; that would violate "store only what's necessary" and could distort baselines.

---

## 4. Implementation Order

1. **Spec defaults (low risk):** Add or use defaults for `schema_version` and `metric_type` for `audit_aggregation_metric` in the auto-fixer path (or in spec so spec-based auto-fixer picks them up).
2. **Sentinel default for event_type_counts:** If auto-fixer applies to "missing required field" for `event_type_counts`, use `{}` when no stream-derived value is available; document that `{}` means "no breakdown."
3. **Window / first_seen:** Do not add generic defaults. Prefer: (a) implement regeneration path (aggregation job re-run for window), or (b) add an explicit `auto_fix_rule` object (via CLI) that sets a placeholder (e.g. from metric created_at) only after product approval. Add tests that assert "auto-fixable → fix applied → no Tier-1" for the safe defaults only.

---

## 5. Success Criteria

- After applying safe defaults (schema_version, metric_type, event_type_counts = {} where appropriate), re-run `zqk system check` and confirm Tier-1 count drops for audit_aggregation_metric.
- Remaining Tier-1 for aggregation_window_* and first_seen are either: (a) resolved by regeneration, or (b) documented as "manual or approved rule only" until regeneration or approved placeholder is implemented.

---

## 6. References

- `docs/architecture/_internal/object_specs/audit_aggregation_metric.yaml`
- `docs/architecture/_internal/object_specs/base_metric.yaml` (first_seen)
- `cmd/zqk/system/spec_auto_fixer*.go`, `auto_fix_rule_loader.go`
- System-check baseline: `.zqk/logs/system-check-*-final.json`
