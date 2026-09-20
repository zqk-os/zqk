package validation

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/stretchr/testify/assert"
)

func TestScopeCreepProtection(t *testing.T) {
	gv := NewGoValidator()

	t.Run("rejects new planned assignment to active plan (sealed column)", func(t *testing.T) {
		options := &ValidationOptions{
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
			ObjectLookup: func(id string) (map[string]any, error) {
				if id == "BLI-123" {
					return map[string]any{
						objects.FieldKeyID:              "BLI-123",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
						objects.FieldKeyPriorityPlanRef: "",
					}, nil
				}
				if id == "PRI-1" {
					return map[string]any{
						objects.FieldKeyID:     "PRI-1",
						objects.FieldKeyKind:   objects.KindPriorityPlan,
						objects.FieldKeyStatus: objects.ObjectStatusActive,
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PRI-1" {
					return "active", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "BLI-123",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriority:        "high",
			objects.FieldKeyGoalRefs:        []any{"GOL-1"},
			objects.FieldKeyPriorityPlanRef: "PRI-1",
		}

		errors := gv.validateCustomRules(context.Background(), "backlog_item", obj, options)
		found := false
		for _, err := range errors {
			if err.Field == objects.FieldKeyPriorityPlanRef && err.Rule == "scope_creep_protection" {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected scope_creep_protection when newly linking planned BLI to active plan")
	})

	t.Run("allows planned update when already linked to active plan", func(t *testing.T) {
		options := &ValidationOptions{
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
			ObjectLookup: func(id string) (map[string]any, error) {
				if id == "BLI-123c" {
					return map[string]any{
						objects.FieldKeyID:              "BLI-123c",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
						objects.FieldKeyPriorityPlanRef: "PRI-1",
					}, nil
				}
				if id == "PRI-1" {
					return map[string]any{
						objects.FieldKeyID:     "PRI-1",
						objects.FieldKeyKind:   objects.KindPriorityPlan,
						objects.FieldKeyStatus: objects.ObjectStatusActive,
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PRI-1" {
					return "active", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "BLI-123c",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriority:        "high",
			objects.FieldKeyGoalRefs:        []any{"GOL-1"},
			objects.FieldKeyPriorityPlanRef: "PRI-1",
		}

		errors := gv.validateCustomRules(context.Background(), "backlog_item", obj, options)
		for _, err := range errors {
			assert.NotEqual(t, "scope_creep_protection", err.Rule, "existing membership must not trip sealed-column refuse: %v", err)
		}
	})

	t.Run("rejects exploring assignment to active plan via membership", func(t *testing.T) {
		options := &ValidationOptions{
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PRI-1" {
					return "active", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "BLI-123b",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          objects.ObjectStatusExploring,
			objects.FieldKeyGoalRefs:        []any{"GOL-1"},
			objects.FieldKeyPriorityPlanRef: "PRI-1",
		}

		errors := gv.validateCustomRules(context.Background(), "backlog_item", obj, options)
		found := false
		for _, err := range errors {
			if err.Field == objects.FieldKeyStatus && err.Rule == "execution_facing_membership" {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected execution_facing_membership error for exploring on active plan")
	})

	t.Run("rejects assignment if target priority plan is complete", func(t *testing.T) {
		options := &ValidationOptions{
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
			ObjectLookup: func(id string) (map[string]any, error) {
				if id == "BLI-124" {
					return map[string]any{
						objects.FieldKeyID:              "BLI-124",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
						objects.FieldKeyPriorityPlanRef: "",
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PRI-2" {
					return "complete", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "BLI-124",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriority:        "high",
			objects.FieldKeyGoalRefs:        []any{"GOL-1"},
			objects.FieldKeyPriorityPlanRef: "PRI-2",
		}

		errors := gv.validateCustomRules(context.Background(), "backlog_item", obj, options)

		found := false
		for _, err := range errors {
			if err.Field == objects.FieldKeyPriorityPlanRef && err.Rule == "scope_creep_protection" {
				found = true
				break
			}
		}

		assert.True(t, found, "Expected scope_creep_protection validation error when assigning to complete plan")
	})

	t.Run("allows assignment if target priority plan is exploring", func(t *testing.T) {
		options := &ValidationOptions{
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
			ObjectLookup: func(id string) (map[string]any, error) {
				if id == "BLI-125" {
					return map[string]any{
						objects.FieldKeyID:              "BLI-125",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
						objects.FieldKeyPriorityPlanRef: "",
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PRI-3" {
					return "exploring", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "BLI-125",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriority:        "high",
			objects.FieldKeyGoalRefs:        []any{"GOL-1"},
			objects.FieldKeyPriorityPlanRef: "PRI-3",
		}

		errors := gv.validateCustomRules(context.Background(), "backlog_item", obj, options)

		for _, err := range errors {
			assert.NotEqual(t, "scope_creep_protection", err.Rule, "Did not expect scope_creep_protection validation error for exploring plan")
		}
	})

	t.Run("allows update if priority plan ref is unchanged and target is active", func(t *testing.T) {
		options := &ValidationOptions{
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
			ObjectLookup: func(id string) (map[string]any, error) {
				if id == "BLI-126" {
					return map[string]any{
						objects.FieldKeyID:              "BLI-126",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
						objects.FieldKeyPriorityPlanRef: "PRI-4",
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PRI-4" {
					return "active", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "BLI-126",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress, // Changing status, not plan ref
			objects.FieldKeyPriority:        "high",
			objects.FieldKeyGoalRefs:        []any{"GOL-1"},
			objects.FieldKeyPriorityPlanRef: "PRI-4",
		}

		errors := gv.validateCustomRules(context.Background(), "backlog_item", obj, options)

		for _, err := range errors {
			assert.NotEqual(t, "scope_creep_protection", err.Rule, "Did not expect scope_creep_protection validation error when plan ref is unchanged")
		}
	})
}
