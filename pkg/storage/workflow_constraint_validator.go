package storage

import (
	"context"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// WorkflowConstraintValidator validates workflow constraints for priority plans and workstreams
type WorkflowConstraintValidator struct {
	storage ObjectStorageProvider
}

// NewWorkflowConstraintValidator creates a new workflow constraint validator
func NewWorkflowConstraintValidator(storage ObjectStorageProvider) *WorkflowConstraintValidator {
	return &WorkflowConstraintValidator{
		storage: storage,
	}
}

// WorkflowConstraints represents the constraints defined in a workflow
type WorkflowConstraints struct {
	RoleConstraints   map[string]RoleConstraint   `yaml:"role_constraints" json:"role_constraints"`
	ObjectConstraints map[string]ObjectConstraint `yaml:"object_constraints" json:"object_constraints"`
}

// RoleConstraint defines constraints for a specific role
type RoleConstraint struct {
	AllowedOperations []string `yaml:"allowed_operations" json:"allowed_operations"` // create, update, delete
	AllowedKinds      []string `yaml:"allowed_kinds" json:"allowed_kinds"`           // Object kinds this role can create/manipulate
	BlockedKinds      []string `yaml:"blocked_kinds" json:"blocked_kinds"`           // Object kinds this role cannot create/manipulate
}

// ObjectConstraint defines constraints for a specific object kind
type ObjectConstraint struct {
	RequiredRoles []string `yaml:"required_roles" json:"required_roles"` // Roles required to create/manipulate
	BlockedRoles  []string `yaml:"blocked_roles" json:"blocked_roles"`   // Roles blocked from creating/manipulating
}

// ValidatePriorityPlanActivation validates workflow constraints when a priority plan is activated
func (wcv *WorkflowConstraintValidator) ValidatePriorityPlanActivation(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	priorityPlanID string,
) error {
	// Read the priority plan
	plan, err := wcv.storage.Read(ctx, secCtx, priorityPlanID)
	if err != nil {
		return errfmt.Errorf(ConstMiscFailedToReadPriorityPlanSW, priorityPlanID, err)
	}

	// Check if workflow_ref is set
	workflowRef, ok := plan[objects.FieldKeyWorkflowRef].(string)
	if !ok || workflowRef == emptyValue {
		// No workflow constraint - allow activation
		return nil
	}

	return wcv.validateWorkflowConstraints(ctx, secCtx, workflowRef, objects.KindPriorityPlan, "activate")
}

// ValidateWorkstreamOperation validates workflow constraints for workstream operations
func (wcv *WorkflowConstraintValidator) ValidateWorkstreamOperation(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	workstreamID string,
	operation string, // create, update, delete
	kind string, // Object kind being created/manipulated
) error {
	// Read the workstream
	workstream, err := wcv.storage.Read(ctx, secCtx, workstreamID)
	if err != nil {
		return errfmt.Errorf(ConstMiscFailedToReadWorkstreamSW, workstreamID, err)
	}

	// Check if workflow_ref is set on workstream
	workflowRef, ok := workstream[objects.FieldKeyWorkflowRef].(string)
	if !ok || workflowRef == emptyValue {
		// Try to inherit from priority plan
		priorityPlanRef, _ := workstream[objects.FieldKeyPriorityPlanRef].(string)
		if priorityPlanRef != emptyValue {
			plan, err := wcv.storage.Read(ctx, secCtx, priorityPlanRef)
			if err == nil {
				workflowRef, _ = plan[objects.FieldKeyWorkflowRef].(string)
			}
		}
	}

	if workflowRef == emptyValue {
		// No workflow constraint - allow operation
		return nil
	}

	return wcv.validateWorkflowConstraints(ctx, secCtx, workflowRef, kind, operation)
}

func (wcv *WorkflowConstraintValidator) validateWorkflowConstraints(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	workflowRef string,
	kind string,
	operation string,
) error {
	// Read the workflow
	workflow, err := wcv.storage.Read(ctx, secCtx, workflowRef)
	if err != nil {
		return errfmt.Errorf(ConstMiscFailedToReadWorkflowSW, workflowRef, err)
	}

	// Check if workflow is enabled
	enabled, _ := workflow[objects.FieldKeyEnabled].(bool)
	if !enabled {
		return errfmt.Errorf(ConstMiscWorkflowSIsNotEnabled, workflowRef)
	}

	// Extract constraints
	constraints := wcv.extractConstraints(workflow)

	// Validate constraints for the current user's roles
	return wcv.validateConstraintsForRoles(ctx, secCtx, constraints, kind, operation)
}

// extractConstraints extracts constraints from a workflow object
// extractConstraints extracts workflow constraints from workflow definition
//
//nolint:gocyclo // Function orchestrates constraint extraction; complexity reduced via helper methods
func (wcv *WorkflowConstraintValidator) extractConstraints(workflow map[string]any) *WorkflowConstraints {
	constraintsData, ok := workflow[objects.FieldKeyConstraints].(map[string]any)
	if !ok {
		return &WorkflowConstraints{
			RoleConstraints:   make(map[string]RoleConstraint),
			ObjectConstraints: make(map[string]ObjectConstraint),
		}
	}

	constraints := &WorkflowConstraints{
		RoleConstraints:   make(map[string]RoleConstraint),
		ObjectConstraints: make(map[string]ObjectConstraint),
	}

	// Extract role constraints
	wcv.extractRoleConstraints(constraintsData, constraints)

	// Extract object constraints
	wcv.extractObjectConstraints(constraintsData, constraints)

	return constraints
}

// extractRoleConstraints extracts role constraints from constraints data
func (wcv *WorkflowConstraintValidator) extractRoleConstraints(constraintsData map[string]any, constraints *WorkflowConstraints) {
	roleConstraintsData, ok := constraintsData[ConstMiscRoleConstraints].(map[string]any)
	if !ok {
		return
	}

	for roleID, constraintData := range roleConstraintsData {
		constraintMap, ok := constraintData.(map[string]any)
		if !ok {
			continue
		}

		roleConstraint := wcv.buildRoleConstraint(constraintMap)
		constraints.RoleConstraints[roleID] = roleConstraint
	}
}

// buildRoleConstraint builds a RoleConstraint from constraint map
func (wcv *WorkflowConstraintValidator) buildRoleConstraint(constraintMap map[string]any) RoleConstraint {
	roleConstraint := RoleConstraint{}

	roleConstraint.AllowedOperations = wcv.extractStringSlice(constraintMap, ConstMiscAllowedOperations)
	roleConstraint.AllowedKinds = wcv.extractStringSlice(constraintMap, "allowed_kinds")
	roleConstraint.BlockedKinds = wcv.extractStringSlice(constraintMap, "blocked_kinds")

	return roleConstraint
}

// extractObjectConstraints extracts object constraints from constraints data
func (wcv *WorkflowConstraintValidator) extractObjectConstraints(constraintsData map[string]any, constraints *WorkflowConstraints) {
	objectConstraintsData, ok := constraintsData[ConstMiscObjectConstraints].(map[string]any)
	if !ok {
		return
	}

	for kind, constraintData := range objectConstraintsData {
		constraintMap, ok := constraintData.(map[string]any)
		if !ok {
			continue
		}

		objectConstraint := wcv.buildObjectConstraint(constraintMap)
		constraints.ObjectConstraints[kind] = objectConstraint
	}
}

// buildObjectConstraint builds an ObjectConstraint from constraint map
func (wcv *WorkflowConstraintValidator) buildObjectConstraint(constraintMap map[string]any) ObjectConstraint {
	objectConstraint := ObjectConstraint{}

	objectConstraint.RequiredRoles = wcv.extractStringSlice(constraintMap, ConstMiscRequiredRoles)
	objectConstraint.BlockedRoles = wcv.extractStringSlice(constraintMap, "blocked_roles")

	return objectConstraint
}

// extractStringSlice extracts a string slice from a map
func (wcv *WorkflowConstraintValidator) extractStringSlice(constraintMap map[string]any, key string) []string {
	var result []string
	sliceData, ok := constraintMap[key].([]any)
	if !ok {
		return result
	}

	for _, item := range sliceData {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}

	return result
}

// validateConstraintsForRoles validates constraints for the user's roles
//
//nolint:unparam // ctx parameter is kept for API consistency
//nolint:gocyclo // Function orchestrates multiple constraint validations; complexity reduced via helper methods
func (wcv *WorkflowConstraintValidator) validateConstraintsForRoles(
	_ context.Context,
	secCtx *pkgctx.SecurityContext,
	constraints *WorkflowConstraints,
	kind string,
	operation string,
) error {
	// Check object-level constraints first
	if err := wcv.validateObjectConstraints(constraints, secCtx, kind, operation); err != nil {
		return err
	}

	// Check role-level constraints
	return wcv.validateRoleConstraints(constraints, secCtx, kind, operation)
}

// validateObjectConstraints validates object-level constraints
func (wcv *WorkflowConstraintValidator) validateObjectConstraints(
	constraints *WorkflowConstraints,
	secCtx *pkgctx.SecurityContext,
	kind string,
	operation string,
) error {
	objectConstraint, ok := constraints.ObjectConstraints[kind]
	if !ok {
		return nil
	}

	// Check if any of the user's roles are blocked
	if err := wcv.checkBlockedRoles(objectConstraint.BlockedRoles, secCtx.Roles, kind, operation); err != nil {
		return err
	}

	// Check if any required roles are present
	return wcv.checkRequiredRoles(objectConstraint.RequiredRoles, secCtx.Roles, kind, operation)
}

// checkBlockedRoles checks if any user roles are blocked
func (wcv *WorkflowConstraintValidator) checkBlockedRoles(blockedRoles, userRoles []string, kind, operation string) error {
	for _, userRole := range userRoles {
		for _, blockedRole := range blockedRoles {
			if userRole == blockedRole || strings.HasSuffix(blockedRole, userRole) {
				return errfmt.Errorf(ConstMiscRoleSIsBlockedFromSOperationOnSObjectsBy, userRole, operation, kind)
			}
		}
	}
	return nil
}

// checkRequiredRoles checks if user has any required roles
func (wcv *WorkflowConstraintValidator) checkRequiredRoles(requiredRoles, userRoles []string, kind, operation string) error {
	if len(requiredRoles) == 0 {
		return nil
	}

	for _, userRole := range userRoles {
		for _, requiredRole := range requiredRoles {
			if userRole == requiredRole || strings.HasSuffix(requiredRole, userRole) {
				return nil // Has required role
			}
		}
	}

	return errfmt.Errorf(ConstMiscOperationSOnSRequiresOneOfTheseRolesV, operation, kind, requiredRoles)
}

// validateRoleConstraints validates role-level constraints
func (wcv *WorkflowConstraintValidator) validateRoleConstraints(
	constraints *WorkflowConstraints,
	secCtx *pkgctx.SecurityContext,
	kind string,
	operation string,
) error {
	for _, userRole := range secCtx.Roles {
		roleConstraint, ok := constraints.RoleConstraints[userRole]
		if !ok {
			continue
		}

		if err := wcv.validateRoleOperation(roleConstraint, userRole, operation); err != nil {
			return err
		}

		if err := wcv.validateRoleKind(roleConstraint, userRole, kind); err != nil {
			return err
		}
	}

	return nil
}

// validateRoleOperation validates if operation is allowed for role
func (wcv *WorkflowConstraintValidator) validateRoleOperation(roleConstraint RoleConstraint, userRole, operation string) error {
	if len(roleConstraint.AllowedOperations) == 0 {
		return nil
	}

	for _, allowedOp := range roleConstraint.AllowedOperations {
		if allowedOp == operation || allowedOp == "*" {
			return nil // Operation allowed
		}
	}

	return errfmt.Errorf(ConstMiscRoleSIsNotAllowedToPerformSOperationByWo, userRole, operation)
}

// validateRoleKind validates if kind is allowed for role
func (wcv *WorkflowConstraintValidator) validateRoleKind(roleConstraint RoleConstraint, userRole, kind string) error {
	// Check if kind is blocked
	for _, blockedKind := range roleConstraint.BlockedKinds {
		if blockedKind == kind || blockedKind == "*" {
			return errfmt.Errorf(ConstMiscRoleSIsBlockedFromOperatingOnSObjectsByW, userRole, kind)
		}
	}

	// Check if kind is allowed (if allowed_kinds is specified, only those are allowed)
	if len(roleConstraint.AllowedKinds) > 0 {
		for _, allowedKind := range roleConstraint.AllowedKinds {
			if allowedKind == kind || allowedKind == "*" {
				return nil // Kind allowed
			}
		}
		return errfmt.Errorf(ConstMiscRoleSIsNotAllowedToOperateOnSObjectsByWo, userRole, kind, roleConstraint.AllowedKinds)
	}

	return nil
}
