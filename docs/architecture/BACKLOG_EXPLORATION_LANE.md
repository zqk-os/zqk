# Backlog Exploration Lane ("Boneyard")

This document defines a low-noise lane for ideas that are not yet strong candidates for active priority plans.

## Purpose

- Preserve ideas without dropping them.
- Prevent uncertain work from crowding current priority plans.
- Make ROI and integration readiness explicit before promotion.

## When to Use the Exploration Lane

Use this lane when one or more of the following is true:

- Immediate user/value impact is unclear.
- Integration cost with the current system is non-trivial and not justified yet.
- The feature is neglected/legacy and modern-system fit is uncertain.
- Required discovery work is still forming (problem definition not stable).

If usefulness is not obvious after a short, concrete review, default to Exploration Lane placement.

## Intake Template (Minimal)

Capture each item with:

- **Problem statement**: what pain or opportunity exists?
- **Hypothesized value**: who benefits, and how?
- **Integration surface**: systems/packages touched (CLI/spec/codegen/storage/etc.).
- **Estimated cost/risk**: rough effort + main risk.
- **Promotion trigger**: what evidence would justify moving it into an active plan?

## Decision Rules

1. **Current plan first**: Active priority plan work is not displaced by exploration items.
2. **Evidence over intuition**: Promotion requires at least one concrete signal (user demand, recurring operational pain, measurable efficiency gain, policy requirement, etc.).
3. **Timebox discovery**: Exploration analysis should be bounded and lightweight.
4. **No silent drift**: Exploration items are reviewed periodically, not ignored forever.

## Suggested Object Conventions

The following keeps this lane queryable without changing lifecycles:

- Keep item kind as `backlog_item`.
- Set `priority_plan_ref` only when promoted to active planning.
- Use a dedicated tag/label in notes/description such as `lane:exploration` (or equivalent project convention).
- Include an explicit `promotion_trigger` note in item details.

## Promotion Checklist (Exploration -> Active Plan)

- Value hypothesis is specific and testable.
- Integration path is known (spec-driven where applicable).
- Initial effort estimate is acceptable relative to current priorities.
- Owner and near-term milestone are identified.

If any are missing, keep it in Exploration Lane.

## Relationship to Existing Process

- Aligns with priority-plan discipline: current plan items first.
- Complements anti-pattern and DRY guidance by reducing context noise.
- Supports strategic clarity by separating "interesting" from "urgent."
