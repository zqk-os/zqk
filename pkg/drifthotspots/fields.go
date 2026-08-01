package drifthotspots

import "github.com/lanceman/zqk/pkg/objects"

// SystemObjectFieldKeys are map/JSON keys shared by object instances across kinds (system + common).
// Drift risk: renames in specs or validation expect these exact spellings everywhere.
// Keep aligned with pkg/specbuilder/instance_builders/codegen.go systemFieldOrder and pkg/objects field key constants.
var SystemObjectFieldKeys = map[string]struct{}{
	objects.FieldKeyID:            {},
	objects.FieldKeyKind:          {},
	objects.FieldKeySchemaVersion: {},
	objects.FieldKeyCreatedAt:     {},
	objects.FieldKeyCreatedBy:     {},
	objects.FieldKeyUpdatedAt:     {},
	objects.FieldKeyUpdatedBy:     {},
	objects.FieldKeyNamespaceID:   {},
	objects.FieldKeyStatus:        {},
	// Common ref / traceability keys (drift parity with scenario + storage)
	objects.FieldKeyGoalRefs:        {},
	objects.FieldKeyMilestoneRefs:   {},
	objects.FieldKeyRequirementRefs: {},
	objects.FieldKeyWorkstreamRefs:  {},
	objects.FieldKeyBacklogItemRefs: {},
	objects.FieldKeyCriteriaRefs:    {},
	objects.FieldKeyTestCaseRefs:    {},
	objects.FieldKeyPriorityPlanRef: {},
}
