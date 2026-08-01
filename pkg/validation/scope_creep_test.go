package validation

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/stretchr/testify/assert"
)

func TestScopeCreepProtection(t *testing.T) {
	gv := NewGoValidator()
	// Disable logging or skip certain setups if needed

	t.Run("rejects assignment if target priority plan is active", func(t *testing.T) {
		options := &ValidationOptions{
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
			ObjectLookup: func(id string) (map[string]any, error) {
				if id == "ITEM-123" {
					return map[string]any{
						objects.FieldKeyID:              "ITEM-123",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          "planned",
						objects.FieldKeyPriorityPlanRef: "",
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PLAN-1" {
					return "active", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "ITEM-123",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyPriorityPlanRef: "PLAN-1",
		}

		errors := gv.validateCustomRules(context.Background(), "backlog_item", obj, options)

		found := false
		for _, err := range errors {
			if err.Field == objects.FieldKeyPriorityPlanRef && err.Rule == "scope_creep_protection" {
				found = true
				break
			}
		}

		assert.True(t, found, "Expected scope_creep_protection validation error when assigning to active plan")
	})

	t.Run("rejects assignment if target priority plan is complete", func(t *testing.T) {
		options := &ValidationOptions{
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
			ObjectLookup: func(id string) (map[string]any, error) {
				if id == "ITEM-124" {
					return map[string]any{
						objects.FieldKeyID:              "ITEM-124",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          "planned",
						objects.FieldKeyPriorityPlanRef: "",
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PLAN-2" {
					return "complete", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "ITEM-124",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyPriorityPlanRef: "PLAN-2",
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
				if id == "ITEM-125" {
					return map[string]any{
						objects.FieldKeyID:              "ITEM-125",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          "planned",
						objects.FieldKeyPriorityPlanRef: "",
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PLAN-3" {
					return "exploring", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "ITEM-125",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyPriorityPlanRef: "PLAN-3",
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
				if id == "ITEM-126" {
					return map[string]any{
						objects.FieldKeyID:              "ITEM-126",
						objects.FieldKeyKind:            "backlog_item",
						objects.FieldKeyStatus:          "planned",
						objects.FieldKeyPriorityPlanRef: "PLAN-4",
					}, nil
				}
				return nil, nil
			},
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "PLAN-4" {
					return "active", nil
				}
				return "", nil
			},
		}

		obj := map[string]any{
			objects.FieldKeyID:              "ITEM-126",
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyStatus:          "in_progress", // Changing status, not plan ref
			objects.FieldKeyPriorityPlanRef: "PLAN-4",
		}

		errors := gv.validateCustomRules(context.Background(), "backlog_item", obj, options)

		for _, err := range errors {
			assert.NotEqual(t, "scope_creep_protection", err.Rule, "Did not expect scope_creep_protection validation error when plan ref is unchanged")
		}
	})
}
