package system

import (
	"testing"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestLifecyclePreconditionPatterns_SingularMilestoneRef(t *testing.T) {
	pattern, matched := matchLifecyclePreconditionPattern("milestone_ref is set")
	if !matched {
		t.Fatal("expected match for 'milestone_ref is set'")
	}
	if pattern.FieldName != objects.FieldKeyMilestoneRef {
		t.Errorf("expected FieldName %q, got %q", objects.FieldKeyMilestoneRef, pattern.FieldName)
	}
	if pattern.Operation != "set" {
		t.Errorf("expected Operation 'set', got %q", pattern.Operation)
	}
}

func TestCheckInstanceValidation_MalformedTitleYAMLPrefix(t *testing.T) {
	ctx := cli.ContextForProjectAndProfile("", "test")
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("")
	if validator == nil {
		t.Fatal("default validator missing")
	}

	testCases := []struct {
		title    string
		hasIssue bool
	}{
		{"Normal title", false},
		{"id: BLI-12345", true},
		{"kind: backlog_item", true},
		{"  id: indented", true},
	}

	for _, tc := range testCases {
		obj := &parser.ParsedObject{
			ID:   "BLI-TEST-TITLE-001",
			Kind: objects.KindBacklogItem,
			Properties: map[string]any{
				objects.FieldKeyID:          "BLI-TEST-TITLE-001",
				objects.FieldKeyKind:        objects.KindBacklogItem,
				objects.FieldKeyTitle:       tc.title,
				objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
				objects.FieldKeyDescription: "Test description",
			},
		}

		issues := checkInstanceValidationWithValidatorAndData(
			ctx,
			pkgctx.NewSystemContext(),
			obj,
			objects.KindBacklogItem,
			validator,
			obj.Properties,
			nil,
		)

		found := false
		for _, issue := range issues {
			if issue.Tier == 1 && issue.Category == "instance_validation" && issue.Message != "" {
				if len(issue.Message) > 0 && issue.Message[:5] == "Title" {
					found = true
					break
				}
			}
		}

		if found != tc.hasIssue {
			t.Errorf("title %q: expected hasIssue=%v, got found=%v", tc.title, tc.hasIssue, found)
		}
	}
}
