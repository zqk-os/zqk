package system

// Code path analysis for fix command generation
//
// Entry Points:
// 1. checkKindObjects() -> checkObject() -> performAllChecks() -> checkInstanceValidationWithValidatorAndData()
// 2. checkIDs() -> checkObject() -> performAllChecks() -> checkInstanceValidationWithValidatorAndData()
//
// Code Path:
// checkKindObjects/checkIDs
//   -> checkObject()
//      -> performAllChecks()
//         -> checkInstanceValidationWithValidatorAndData()
//            -> validator.Validate() -> ValidationResult
//            -> convertValidationResultsToIssues()
//               -> generateFixCommand() <- THIS IS WHERE FIX COMMANDS ARE GENERATED
//
// Finite State Possibilities for generateFixCommand():
//
// State 1: Empty objID (FAIL-FAST)
//   Condition: objID == emptyValue
//   Action: Return "" immediately
//   Result: auto_fixable = false
//
// State 2: Lifecycle violation with valid precondition pattern
//   Conditions:
//     - rule == "lifecycle"
//     - strings.Contains(message, "Precondition not met")
//     - Pattern matches one of:
//       * milestone_refs patterns
//       * priority_plan_ref pattern
//       * workstream_refs patterns
//       * goal_refs patterns
//   Action: Generate fix command with query hint
//   Result: auto_fixable = true, FixCommand set
//
// State 3: Required field violation
//   Conditions:
//     - rule == "required"
//   Action: Generate generic fix command
//   Result: auto_fixable = true, FixCommand set (generic)
//
// State 4: Unknown/unsupported violation (FAIL-FAST)
//   Conditions:
//     - rule != "lifecycle" && rule != "required"
//     - OR rule == "lifecycle" but pattern doesn't match
//     - OR rule == "lifecycle" but message doesn't contain "Precondition not met"
//   Action: Return ""
//   Result: auto_fixable = false
//
// Fail-Fast Optimizations:
// 1. Check objID == emptyValue first (already implemented)
// 2. Check rule type early - if not "lifecycle" or "required", can exit early
// 3. For lifecycle: check "Precondition not met" marker before processing
// 4. For lifecycle: check pattern matches before building query hint
