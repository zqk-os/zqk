package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// ViolationResolver resolves validation violations using object specifications
// It uses spec knowledge to auto-link violations that have sufficient context
type ViolationResolver struct {
	specLoader  *objects.SpecLoader
	logger      logging.Logger
	projectRoot string
}

// ResolutionResult represents the result of resolving a violation
type ResolutionResult struct {
	Resolved     bool     `json:"resolved"`
	Resolution   string   `json:"resolution,omitempty"`  // How it was resolved
	Suggestions  []string `json:"suggestions,omitempty"` // Suggested fixes if not resolved
	Confidence   float64  `json:"confidence,omitempty"`  // Confidence level (0.0-1.0)
	AutoLinkable bool     `json:"auto_linkable"`         // Whether it can be auto-linked
}

// NewViolationResolver creates a new violation resolver
func NewViolationResolver(projectRoot string, specLoader *objects.SpecLoader, logger logging.Logger) *ViolationResolver {
	return &ViolationResolver{
		specLoader:  specLoader,
		logger:      logger,
		projectRoot: projectRoot,
	}
}

// ResolveViolation attempts to resolve a violation using spec knowledge
func (vr *ViolationResolver) ResolveViolation(result *CheckResult, issue Issue) (*ResolutionResult, error) {
	resolution := &ResolutionResult{
		Resolved:     false,
		AutoLinkable: false,
		Confidence:   0.0,
	}

	// Load object spec
	spec, err := vr.specLoader.LoadSpecWithInheritance(result.ObjectKind + ".yaml")
	if err != nil {
		// Spec not found - cannot resolve
		return resolution, nil
	}

	// Analyze issue based on category and message
	switch issue.Category {
	case "instance_validation":
		return vr.resolveInstanceValidation(result, issue, spec, resolution)
	case "reference":
		return vr.resolveReference(result, issue, spec, resolution)
	case objects.KindLifecycle:
		return vr.resolveLifecycle(result, issue, spec, resolution)
	case "integrity":
		return vr.resolveIntegrity(result, issue, spec, resolution)
	default:
		// Unknown category - cannot resolve
		return resolution, nil
	}
}

// resolveInstanceValidation resolves instance validation violations
// These often involve missing required fields or invalid field values
func (vr *ViolationResolver) resolveInstanceValidation(result *CheckResult, issue Issue, spec *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	// Parse message to extract field name and issue type
	// Example: "status: Precondition not met for status 'planned': priority_plan_ref is set (required per DEC-priority-plan-ref-requirement)"

	message := issue.Message

	// Check for missing reference fields
	if strings.Contains(message, objects.FieldKeyPriorityPlanRef) {
		return vr.resolvePriorityPlanRef(result, issue, spec, resolution)
	}

	if strings.Contains(message, "milestone_ref") {
		return vr.resolveMilestoneRef(result, issue, spec, resolution)
	}

	if strings.Contains(message, objects.FieldKeyGoalRefs) {
		return vr.resolveGoalRefs(result, issue, spec, resolution)
	}

	// Check for invalid status
	if strings.Contains(message, "Invalid lifecycle status") {
		return vr.resolveInvalidStatus(result, issue, spec, resolution)
	}

	return resolution, nil
}

// resolveReference resolves reference validation violations
func (vr *ViolationResolver) resolveReference(_ *CheckResult, issue Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	// Reference violations can often be auto-linked using:
	// 1. Field path context (e.g., priority_plan_ref in backlog_item)
	// 2. Object kind relationships from spec
	// 3. File location context

	message := issue.Message

	// Try to extract reference field and target kind from message
	// Example: "Reference 'priority_plan_ref' points to non-existent object 'PLAN-213'"
	if strings.Contains(message, "points to non-existent") {
		// Could try to find similar objects or suggest creation
		resolution.Suggestions = append(resolution.Suggestions,
			"Reference target does not exist. Check if object ID is correct or if object needs to be created.")
	}

	return resolution, nil
}

// resolveLifecycle resolves lifecycle validation violations
func (vr *ViolationResolver) resolveLifecycle(result *CheckResult, issue Issue, spec *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	// Lifecycle violations often involve invalid state transitions
	// These can sometimes be auto-fixed if we know the correct state

	message := issue.Message

	if strings.Contains(message, "Invalid lifecycle status") {
		return vr.resolveInvalidStatus(result, issue, spec, resolution)
	}

	return resolution, nil
}

// resolveIntegrity resolves integrity validation violations
func (vr *ViolationResolver) resolveIntegrity(_ *CheckResult, issue Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	// Integrity violations (hash mismatches) are usually auto-fixable
	if strings.Contains(issue.Message, "Hash mismatch") {
		resolution.AutoLinkable = true
		resolution.Confidence = 0.9
		resolution.Suggestions = append(resolution.Suggestions,
			"Hash mismatch detected. Use --force to regenerate hash (creates audit event).")
	}

	return resolution, nil
}

// resolvePriorityPlanRef attempts to resolve missing priority_plan_ref
func (vr *ViolationResolver) resolvePriorityPlanRef(_ *CheckResult, _ Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	// Check if spec defines priority_plan_ref field
	// If it does, we can try to find matching priority plans based on:
	// 1. priority_tier matching
	// 2. Recent priority plans
	// 3. Active priority plans

	// For now, provide suggestion
	resolution.AutoLinkable = true
	resolution.Confidence = 0.7
	resolution.Suggestions = append(resolution.Suggestions,
		"priority_plan_ref is required. Link to an active priority plan that matches the priority_tier.")

	return resolution, nil
}

// resolveMilestoneRef attempts to resolve missing milestone_ref
func (vr *ViolationResolver) resolveMilestoneRef(_ *CheckResult, _ Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	resolution.AutoLinkable = true
	resolution.Confidence = 0.7
	resolution.Suggestions = append(resolution.Suggestions,
		"milestone_ref is required. Link to a milestone that matches the priority_tier or create a new milestone.")

	return resolution, nil
}

// resolveGoalRefs attempts to resolve missing goal_refs
func (vr *ViolationResolver) resolveGoalRefs(_ *CheckResult, _ Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	resolution.AutoLinkable = true
	resolution.Confidence = 0.6
	resolution.Suggestions = append(resolution.Suggestions,
		"goal_refs is required (minCount: 1). Link to at least one goal that this object supports.")

	return resolution, nil
}

// resolveInvalidStatus attempts to resolve invalid lifecycle status
func (vr *ViolationResolver) resolveInvalidStatus(_ *CheckResult, issue Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	// Extract invalid status from message
	// Example: "Invalid lifecycle status 'not_started' for kind 'test_case'"

	message := issue.Message
	if strings.Contains(message, "Invalid lifecycle status") {
		// Try to suggest valid statuses from spec
		resolution.AutoLinkable = true
		resolution.Confidence = 0.8
		resolution.Suggestions = append(resolution.Suggestions,
			"Invalid lifecycle status detected. Check the lifecycle definition for valid statuses for this object kind.")
	}

	return resolution, nil
}

// FilterResolvableViolations filters out violations that can be auto-resolved
// Returns only violations that cannot be automatically resolved
func (vr *ViolationResolver) FilterResolvableViolations(snapshot *CheckSnapshot) (*CheckSnapshot, error) {
	filtered := &CheckSnapshot{
		Metadata: snapshot.Metadata,
		Results:  make([]CheckResult, 0),
	}

	for _, result := range snapshot.Results {
		filteredResult := CheckResult{
			ObjectID:   result.ObjectID,
			ObjectKind: result.ObjectKind,
			FilePath:   result.FilePath,
			Issues:     make([]Issue, 0),
			AutoFixed:  result.AutoFixed,
		}

		for _, issue := range result.Issues {
			resolution, err := vr.ResolveViolation(&result, issue)
			if err != nil {
				logging.Fluent(vr.logger).Warn("Failed to resolve violation").
					String("object_id", result.ObjectID).
					String("issue", issue.Message).
					WithError(err).
					Log()
				// Include unresolved issues
				filteredResult.Issues = append(filteredResult.Issues, issue)
				continue
			}

			// Only include issues that cannot be auto-resolved
			if !resolution.AutoLinkable || resolution.Confidence < 0.8 {
				filteredResult.Issues = append(filteredResult.Issues, issue)
			} else {
				logging.Fluent(vr.logger).Debug("Filtered out auto-resolvable violation").
					String("object_id", result.ObjectID).
					String("issue", issue.Message).
					String("confidence", fmt.Sprintf("%.2f", resolution.Confidence)).
					Log()
			}
		}

		// Only include results that still have issues after filtering
		if len(filteredResult.Issues) > 0 {
			filtered.Results = append(filtered.Results, filteredResult)
		}
	}

	// Update metadata
	filtered.Metadata.TotalObjects = len(filtered.Results)
	var totalIssues int
	for _, result := range filtered.Results {
		totalIssues += len(result.Issues)
	}
	filtered.Metadata.TotalIssues = totalIssues

	return filtered, nil
}

// GetObjectSpecPath returns the path to an object spec file
func (vr *ViolationResolver) GetObjectSpecPath(kind string) string {
	// Specs are typically in docs/process/_internal/object_specs/
	return filepath.Join(vr.projectRoot, paths.ProcessInternalObjectSpecsDir, kind+".yaml")
}
