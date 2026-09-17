# Field State Tracking Principles

## Core Principle

**If a field is used to track or understand the possible state of an object, that field must have a finite set of possible values in order to be reliable.**

This doesn't mean that the entire field must remain static—just that there needs to be a consistently applied rule that guarantees a finite set of values for the state-tracking portion of the field.

## Examples

### Dynamic Fields with Finite State Bits

A `title` field can be dynamic and change frequently, as long as there is an established convention for:
- **Which bits** (character positions, prefixes, suffixes, or patterns) encode state information
- **Where those bits are located** within the field value
- **The finite list of possible values** for those state-encoding bits

**Example: Status in Title Convention**
```
Title format: "[STATUS] Free-form description"

Status bits (characters 1-12): Finite set: ["[PLANNED]", "[ACTIVE]", "[BLOCKED]", "[DONE]"]
Free bits (characters 13+): Can change freely without impacting state validation
```

In this convention:
- The status prefix (positions 1-12) is **sacred**—must be one of the finite set
- The rest of the title (positions 13+) is **free**—can change without affecting state tracking
- Validation can extract status from the sacred bits reliably
- Query matching can use the status bits for state-aware searches

### Fields Used for State Tracking

Fields commonly used for state tracking should have well-defined, finite value sets:

1. **Status Fields**: Enum with defined lifecycle states
2. **Category Fields**: Enum or controlled vocabulary
3. **Tags**: Controlled vocabulary or tag categories (even if dynamic, the categories are finite)
4. **Priority/Type Fields**: Enum with defined levels/types

## Implications for Auto-Fix and Validation

### Query Hint Generation

When generating query hints for auto-fix (e.g., finding matching milestones), we rely on fields that must have reliable, finite value sets:

- **Status**: Must be from a finite enum (e.g., `not_started`, `in_progress`, `blocked`, `complete`)
- **Category**: Must be from a finite set (e.g., `feature`, `bug`, `enhancement`)
- **Tags**: Can be dynamic, but should use a controlled vocabulary or tag categories
- **Title**: If used for matching, should follow a convention where state-encoding bits are finite

### Pattern Matching and Resolution

When resolving placeholders in fix commands:
- Extract state-encoding bits from fields using established conventions
- Match against finite value sets, not free-form text
- Handle dynamic portions of fields separately from state-encoding portions

### Validation Rules

Validation rules should:
- Define finite value sets for state-tracking fields
- Document conventions for extracting state from dynamic fields (if applicable)
- Enforce that state-encoding bits remain within the finite set

## Best Practices

1. **Define Finite Value Sets Explicitly**
   - Use enums for status, category, priority fields
   - Document controlled vocabularies for tags
   - Specify conventions for state-encoding in dynamic fields (like title)

2. **Separate State Bits from Free Bits**
   - Clearly identify which portions of a field encode state
   - Document the location/pattern of state-encoding bits
   - Allow free bits to change without state validation impact

3. **Use Consistent Conventions**
   - Apply the same convention across similar object kinds
   - Document conventions in object specifications
   - Validate adherence to conventions in validation rules

4. **Query and Matching Logic**
   - Extract state-encoding bits before matching
   - Match against finite value sets, not raw field values
   - Handle conventions consistently in resolution logic

## Related Documentation

- [Efficient Data Processing](./EFFICIENT_DATA_PROCESSING.md) - Optimization principles for data processing pipelines
- Object Specifications - Define finite value sets for fields (enums, controlled vocabularies)
- Validation Rules - Enforce finite value sets and conventions
