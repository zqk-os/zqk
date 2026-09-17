package systemcheck

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

// ViolationResolver resolves validation violations using object specifications.
// It uses spec knowledge to auto-link violations that have sufficient context.
type ViolationResolver struct {
	specLoader  *objects.SpecLoader
	logger      logging.Logger
	projectRoot string
}

// NewViolationResolver creates a new violation resolver.
func NewViolationResolver(projectRoot string, specLoader *objects.SpecLoader, logger logging.Logger) *ViolationResolver {
	return &ViolationResolver{
		specLoader:  specLoader,
		logger:      logger,
		projectRoot: projectRoot,
	}
}

// ResolveViolation attempts to resolve a violation using spec knowledge.
func (vr *ViolationResolver) ResolveViolation(result *CheckResult, issue Issue) (*ResolutionResult, error) {
	resolution := &ResolutionResult{
		Resolved:     false,
		AutoLinkable: false,
		Confidence:   0.0,
	}

	spec, err := vr.specLoader.LoadSpecWithInheritance(result.ObjectKind + ".yaml")
	if err != nil {
		return resolution, nil
	}

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
		return resolution, nil
	}
}

func (vr *ViolationResolver) resolveInstanceValidation(result *CheckResult, issue Issue, spec *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	message := issue.Message

	if strings.Contains(message, objects.FieldKeyPriorityPlanRef) {
		return vr.resolvePriorityPlanRef(result, issue, spec, resolution)
	}
	if strings.Contains(message, "milestone_ref") {
		return vr.resolveMilestoneRef(result, issue, spec, resolution)
	}
	if strings.Contains(message, objects.FieldKeyGoalRefs) {
		return vr.resolveGoalRefs(result, issue, spec, resolution)
	}
	if strings.Contains(message, "Invalid lifecycle status") {
		return vr.resolveInvalidStatus(result, issue, spec, resolution)
	}
	return resolution, nil
}

func (vr *ViolationResolver) resolveReference(_ *CheckResult, issue Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	if strings.Contains(issue.Message, "points to non-existent") {
		resolution.Suggestions = append(resolution.Suggestions,
			"Reference target does not exist. Check if object ID is correct or if object needs to be created.")
	}
	return resolution, nil
}

func (vr *ViolationResolver) resolveLifecycle(result *CheckResult, issue Issue, spec *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	if strings.Contains(issue.Message, "Invalid lifecycle status") {
		return vr.resolveInvalidStatus(result, issue, spec, resolution)
	}
	return resolution, nil
}

func (vr *ViolationResolver) resolveIntegrity(_ *CheckResult, issue Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	if strings.Contains(issue.Message, "Hash mismatch") {
		resolution.AutoLinkable = true
		resolution.Confidence = 0.9
		resolution.Suggestions = append(resolution.Suggestions,
			"Hash mismatch detected. Use --force to regenerate hash (creates audit event).")
	}
	return resolution, nil
}

func (vr *ViolationResolver) resolvePriorityPlanRef(_ *CheckResult, _ Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	resolution.AutoLinkable = true
	resolution.Confidence = 0.7
	resolution.Suggestions = append(resolution.Suggestions,
		"priority_plan_ref is required. Link to an active priority plan that matches the priority_tier.")
	return resolution, nil
}

func (vr *ViolationResolver) resolveMilestoneRef(_ *CheckResult, _ Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	resolution.AutoLinkable = true
	resolution.Confidence = 0.7
	resolution.Suggestions = append(resolution.Suggestions,
		"milestone_ref is required. Link to a milestone that matches the priority_tier or create a new milestone.")
	return resolution, nil
}

func (vr *ViolationResolver) resolveGoalRefs(_ *CheckResult, _ Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	resolution.AutoLinkable = true
	resolution.Confidence = 0.6
	resolution.Suggestions = append(resolution.Suggestions,
		"goal_refs is required (minCount: 1). Link to at least one goal that this object supports.")
	return resolution, nil
}

func (vr *ViolationResolver) resolveInvalidStatus(_ *CheckResult, issue Issue, _ *objects.Spec, resolution *ResolutionResult) (*ResolutionResult, error) {
	if strings.Contains(issue.Message, "Invalid lifecycle status") {
		resolution.AutoLinkable = true
		resolution.Confidence = 0.8
		resolution.Suggestions = append(resolution.Suggestions,
			"Invalid lifecycle status detected. Check the lifecycle definition for valid statuses for this object kind.")
	}
	return resolution, nil
}

// FilterResolvableViolations filters out violations that can be auto-resolved.
// Returns only violations that cannot be automatically resolved.
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
					ObjectID(result.ObjectID).
					Message(issue.Message).
					WithError(err).
					Log()
				filteredResult.Issues = append(filteredResult.Issues, issue)
				continue
			}

			if !resolution.AutoLinkable || resolution.Confidence < 0.8 {
				filteredResult.Issues = append(filteredResult.Issues, issue)
			} else {
				logging.Fluent(vr.logger).Debug("Filtered out auto-resolvable violation").
					ObjectID(result.ObjectID).
					Message(issue.Message).
					Note(fmt.Sprintf("confidence=%.2f", resolution.Confidence)).
					Log()
			}
		}

		if len(filteredResult.Issues) > 0 {
			filtered.Results = append(filtered.Results, filteredResult)
		}
	}

	filtered.Metadata.TotalObjects = len(filtered.Results)
	var totalIssues int
	for _, result := range filtered.Results {
		totalIssues += len(result.Issues)
	}
	filtered.Metadata.TotalIssues = totalIssues

	return filtered, nil
}

// GetObjectSpecPath returns the path to an object spec file.
func (vr *ViolationResolver) GetObjectSpecPath(kind string) string {
	return filepath.Join(vr.projectRoot, paths.ProcessInternalObjectSpecsDir, kind+".yaml")
}
