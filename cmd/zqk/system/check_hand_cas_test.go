package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	testkit "github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
)

// Policy fixtures in this file need a valid enforcement_level; the field is policy-specific, so it
// has no objects.FieldKey* constant.
const (
	testFieldKeyEnforcementLevel = "enforcement_level"
	testEnforcementLevelRequired = "required"
)

// TestHandCASMaterialize_Detection tests that files materialized without created_by and updated_by
// are flagged as Tier 1 blocking issues.
func TestHandCASMaterialize_Detection(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: pkgctx.SystemAccountID,
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)

	// Valid policy created by system
	validPolicy := map[string]any{
		objects.FieldKeyID:            "POL-CODE-999",
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        scheduler.StatusActive,
		objects.FieldKeyTitle:         "Valid Policy",
		objects.FieldKeyPolicyType:    "standard",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "This is valid",
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		testFieldKeyEnforcementLevel:  testEnforcementLevelRequired,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, validPolicy, scheduler.StatusActive)
	storage.FlushAllOrFail(t, testRoot)

	// Create hand-cas policy (missing created_by and updated_by)
	handCasContent := fmt.Sprintf(`id: POL-CODE-1000
kind: policy
schema_version: "`+objects.DefaultSchemaVersion+`"
status: active
title: Hand CAS Policy
policy_type: standard
category: code_quality
body: "missing metadata"
origin_project: %s
origin_system: %s
enforcement_level: required
`, validation.DefaultOriginProject, validation.DefaultOriginSystem)

	policiesDir := filepath.Join(testRoot, paths.ProcessPoliciesDir)
	handCasPath := filepath.Join(policiesDir, "POL-CODE-1000-HAND.yaml")
	if err := fileutil.WriteFile(handCasPath, []byte(handCasContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create hand-cas policy file: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Use global loaders (same as bulk check path)
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("") // Default validator

	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}
	objectIDCache := GetGlobalObjectIDCache()
	if err := objectIDCache.BuildCache(context.Background(), testRoot, true); err != nil {
		t.Fatalf("Failed to build object ID cache: %v", err)
	}

	results, _, err := CheckKindObjectsWithCache(
		checkCtx,
		pkgctx.NewSystemContext(),
		&cobra.Command{},
		"policy",
		nil,
		specLoader,
		lifecycleLoader,
		validator,
		hashRegistryCache,
		objectIDCache,
	)
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	var validFileResult, handCasResult *CheckResult
	for i := range results {
		if results[i].ObjectID == "POL-CODE-999" {
			validFileResult = &results[i]
		}
		if results[i].ObjectID == "POL-CODE-1000" {
			handCasResult = &results[i]
		}
	}

	if validFileResult == nil {
		t.Fatal("Valid policy file not found in check results")
	}
	if handCasResult == nil {
		t.Fatal("Hand-CAS policy file not found in check results")
	}

	// Verify hand-CAS was flagged as Tier 1
	handCasIssues := findIssuesByCategory(handCasResult.Issues, "registration")
	foundHandCasIssue := false
	for _, issue := range handCasIssues {
		if strings.Contains(issue.Message, "hand-CAS materialized") {
			foundHandCasIssue = true
			if issue.Tier != 1 {
				t.Errorf("Hand-CAS issue should be Tier 1, got %d", issue.Tier)
			}
			break
		}
	}
	if !foundHandCasIssue {
		t.Error("Expected hand-CAS materialized issue to be flagged")
	}

	// Verify valid policy was not flagged
	validIssues := findIssuesByCategory(validFileResult.Issues, "registration")
	for _, issue := range validIssues {
		if strings.Contains(issue.Message, "hand-CAS materialized") {
			t.Errorf("Valid policy falsely flagged as hand-CAS materialized: %s", issue.Message)
		}
	}
}
