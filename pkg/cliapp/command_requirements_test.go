package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestGetCommandRequirements_Defaults(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	sub := &cobra.Command{Use: "status"}
	root.AddCommand(sub)

	req := GetCommandRequirements(root, []string{"status"})
	if req.RequiresStorage {
		t.Errorf("expected RequiresStorage=false by default, got true")
	}
	if !req.RequiresSession {
		t.Errorf("expected RequiresSession=true by default, got false")
	}
	if !req.RequiresSchedulerCheck {
		t.Errorf("expected RequiresSchedulerCheck=true by default, got false")
	}
	if req.RequiresGovernorApproval {
		t.Errorf("expected RequiresGovernorApproval=false by default, got true")
	}
}

func TestGetCommandRequirements_ObjectAndInternalSubtree(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	objCmd := &cobra.Command{Use: "object"}
	objGet := &cobra.Command{Use: "get"}
	objCmd.AddCommand(objGet)
	root.AddCommand(objCmd)

	intCmd := &cobra.Command{Use: "internal"}
	intList := &cobra.Command{Use: "list"}
	intCmd.AddCommand(intList)
	root.AddCommand(intCmd)

	reqObj := GetCommandRequirements(root, []string{"object", "get"})
	if !reqObj.RequiresStorage {
		t.Errorf("expected RequiresStorage=true under 'object', got false")
	}

	reqInt := GetCommandRequirements(root, []string{"internal", "list"})
	if !reqInt.RequiresStorage {
		t.Errorf("expected RequiresStorage=true under 'internal', got false")
	}
}

func TestGetCommandRequirements_CustomAnnotations(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	custom := &cobra.Command{
		Use: "custom",
		Annotations: map[string]string{
			AnnRequiresStorage:          "false",
			AnnRequiresSession:          "0",
			AnnRequiresSchedulerCheck:   "no",
			AnnRequiresGovernorApproval: "true",
		},
	}
	root.AddCommand(custom)

	req := GetCommandRequirements(root, []string{"custom"})
	if req.RequiresStorage {
		t.Errorf("expected RequiresStorage=false from annotation, got true")
	}
	if req.RequiresSession {
		t.Errorf("expected RequiresSession=false from annotation, got true")
	}
	if req.RequiresSchedulerCheck {
		t.Errorf("expected RequiresSchedulerCheck=false from annotation, got true")
	}
	if !req.RequiresGovernorApproval {
		t.Errorf("expected RequiresGovernorApproval=true from annotation, got false")
	}
}

func TestCommandRequirements_HelperSetters(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	RequireSession(cmd, true)
	if SessionOptional(cmd) {
		t.Errorf("expected SessionOptional=false when RequireSession(true)")
	}

	RequireSession(cmd, false)
	if !SessionOptional(cmd) {
		t.Errorf("expected SessionOptional=true when RequireSession(false)")
	}

	RequireStorage(cmd, true)
	if cmd.Annotations[AnnRequiresStorage] != "true" {
		t.Errorf("expected AnnRequiresStorage='true', got %s", cmd.Annotations[AnnRequiresStorage])
	}

	RequireStorage(cmd, false)
	if cmd.Annotations[AnnRequiresStorage] != "false" {
		t.Errorf("expected AnnRequiresStorage='false', got %s", cmd.Annotations[AnnRequiresStorage])
	}

	RequireSchedulerCheck(cmd, true)
	if cmd.Annotations[AnnRequiresSchedulerCheck] != "true" {
		t.Errorf("expected AnnRequiresSchedulerCheck='true', got %s", cmd.Annotations[AnnRequiresSchedulerCheck])
	}

	RequireSchedulerCheck(cmd, false)
	if cmd.Annotations[AnnRequiresSchedulerCheck] != "false" {
		t.Errorf("expected AnnRequiresSchedulerCheck='false', got %s", cmd.Annotations[AnnRequiresSchedulerCheck])
	}

	RequireGovernorApproval(cmd, true)
	if cmd.Annotations[AnnRequiresGovernorApproval] != "true" {
		t.Errorf("expected AnnRequiresGovernorApproval='true', got %s", cmd.Annotations[AnnRequiresGovernorApproval])
	}

	RequireGovernorApproval(cmd, false)
	if cmd.Annotations[AnnRequiresGovernorApproval] != "false" {
		t.Errorf("expected AnnRequiresGovernorApproval='false', got %s", cmd.Annotations[AnnRequiresGovernorApproval])
	}
}

func TestApplyAnnotation_Variants(t *testing.T) {
	tests := []struct {
		val      string
		initial  bool
		expected bool
	}{
		{"true", false, true},
		{"1", false, true},
		{"yes", false, true},
		{"TRUE", false, true},
		{"false", true, false},
		{"0", true, false},
		{"no", true, false},
		{"unrecognized", true, true},
	}

	for _, tt := range tests {
		cmd := &cobra.Command{
			Use: "test",
			Annotations: map[string]string{
				"test.key": tt.val,
			},
		}
		res := tt.initial
		applyAnnotation(cmd, "test.key", &res)
		if res != tt.expected {
			t.Errorf("val %q: expected %v, got %v", tt.val, tt.expected, res)
		}
	}
}
