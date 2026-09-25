package mcp

import (
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// RoleAwarePromptGenerator generates prompts based on project context and agent role
type RoleAwarePromptGenerator struct {
	context           *ProjectContext
	role              string                           // agent role (developer, admin, viewer)
	roles             []string                         // agent roles
	guidanceGenerator *RoleGuidanceGenerator           // Optional: for loading role objects
	secCtx            interface{ GetRoles() []string } // Optional: security context for role loading
	assigneePersona   string                           // Persona context filter
	storage           StorageProvider                  // Optional: for querying metrics
}

// NewRoleAwarePromptGenerator creates a new role-aware prompt generator
func NewRoleAwarePromptGenerator(context *ProjectContext, roles []string) *RoleAwarePromptGenerator {
	role := "viewer" // default
	if len(roles) > 0 {
		role = roles[0] // primary role
	}
	return &RoleAwarePromptGenerator{
		context: context,
		role:    role,
		roles:   roles,
	}
}

// WithRoleGuidanceGenerator sets the role guidance generator for loading role objects
func (g *RoleAwarePromptGenerator) WithRoleGuidanceGenerator(guidanceGen *RoleGuidanceGenerator) *RoleAwarePromptGenerator {
	g.guidanceGenerator = guidanceGen
	return g
}

// WithSecurityContext sets the security context for role loading
func (g *RoleAwarePromptGenerator) WithSecurityContext(secCtx interface{ GetRoles() []string }) *RoleAwarePromptGenerator {
	g.secCtx = secCtx
	return g
}

// WithAssigneePersona sets the persona reference for context filtering
func (g *RoleAwarePromptGenerator) WithAssigneePersona(persona string) *RoleAwarePromptGenerator {
	g.assigneePersona = persona
	return g
}

// WithStorageProvider sets the storage provider for querying metrics
func (g *RoleAwarePromptGenerator) WithStorageProvider(storage StorageProvider) *RoleAwarePromptGenerator {
	g.storage = storage
	return g
}

// roleGuidanceDepsMissing reports whether role-object guidance cannot be loaded (missing generator or sec context).
func (g *RoleAwarePromptGenerator) roleGuidanceDepsMissing() bool {
	return g.guidanceGenerator == nil || g.secCtx == nil
}

// GenerateWelcomeMessage generates a role-specific welcome message.
// Returns fallback content when role objects are unavailable (no panic).
func (g *RoleAwarePromptGenerator) GenerateWelcomeMessage() (baseMessage, quickStart string) {
	if g.roleGuidanceDepsMissing() {
		return WelcomeMessageBase, WelcomeMessageQuickStartFallback
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		Roles: g.roles,
	}

	guidance, err := g.guidanceGenerator.GenerateRoleGuidance(ctx, secCtx, g.role)
	if err != nil || guidance == nil {
		return WelcomeMessageBase, WelcomeMessageQuickStartFallback
	}
	if guidance.WelcomeMessage == emptyValue || guidance.WelcomeQuickStart == emptyValue {
		return WelcomeMessageBase, WelcomeMessageQuickStartFallback
	}

	return guidance.WelcomeMessage, guidance.WelcomeQuickStart
}

// GenerateBigPicturePrompt generates a comprehensive "big picture" prompt
func (g *RoleAwarePromptGenerator) GenerateBigPicturePrompt() string {
	var parts []string

	// Determine persona
	personaLower := strings.ToLower(g.assigneePersona)
	isCoder := strings.Contains(personaLower, "coder") || strings.Contains(personaLower, "backend") || strings.Contains(personaLower, "frontend")
	isTPM := strings.Contains(personaLower, "tpm") || strings.Contains(personaLower, "orchestrator")
	isObserver := strings.Contains(personaLower, "observer")
	isIA := strings.Contains(personaLower, "architect") || strings.Contains(personaLower, "ia")

	// If Coder, truncate Layer 1 entirely
	if !isCoder {
		// Mission and Vision
		parts = append(parts, "## Project Mission and Vision\n")
		if g.context.Mission != nil {
			parts = append(parts,
				fmt.Sprintf("**Mission (%s)**: %s\n", g.context.Mission.ID, g.context.Mission.Title),
				fmt.Sprintf("%s\n", g.context.Mission.Statement))
		}
		if g.context.Vision != nil {
			parts = append(parts,
				fmt.Sprintf("\n**Vision (%s)**: %s\n", g.context.Vision.ID, g.context.Vision.Title),
				fmt.Sprintf("%s\n", g.context.Vision.Statement))
		}

		// Current Strategic Context
		if g.context.Strategy != nil {
			parts = append(parts, "\n## Current Strategic Context\n")

			if len(g.context.Strategy.Goals) > 0 {
				parts = append(parts, "**Active Goals**:\n")
				for _, goal := range g.context.Strategy.Goals {
					parts = append(parts, fmt.Sprintf("- %s: %s\n", goal.ID, goal.Title))
				}
			}

			if len(g.context.Strategy.Workstreams) > 0 {
				parts = append(parts, "\n**Active Workstreams**:\n")
				for _, ws := range g.context.Strategy.Workstreams {
					parts = append(parts, fmt.Sprintf("- %s (%s): %s [%s]\n", ws.ID, ws.Priority, ws.Title, ws.Status))
				}
			}
		}
	}

	// Current Priority Plans (TPM gets maximum priority, but also standard for others unless Coder)
	if !isCoder {
		if g.context.CurrentState != nil && len(g.context.CurrentState.ActivePriorityPlans) > 0 {
			parts = append(parts, "\n## Active Priority Plans\n")
			for _, pp := range g.context.CurrentState.ActivePriorityPlans {
				parts = append(parts, fmt.Sprintf("- %s (%s): %s [%s]\n", pp.ID, pp.Priority, pp.Title, pp.Status))
			}
		}
	}

	// TPM Dependency Graph Expansion (Mocked expansion or placeholder for dependency logic)
	if isTPM {
		parts = append(parts, "\n## Dependency Graph\n")
		parts = append(parts, "**Focus**: Ensure dependencies and risk blockers are resolved. Maximize structural overview.\n")
	}

	// Observer Metrics Expansion
	if isObserver {
		parts = append(parts, "\n## Observer Operational Metrics\n")
		// Programmatically pull leading/trailing metrics here
		parts = append(parts, g.pullObserverMetrics())
	}

	// Workflow Policies (Critical for execution)
	if g.context.Workflows != nil {
		parts = append(parts, "\n## Workflow Policies - CRITICAL FOR EXECUTION\n")

		if g.context.Workflows.GitBranchPolicy != nil {
			parts = append(parts,
				fmt.Sprintf("### %s: %s\n", g.context.Workflows.GitBranchPolicy.ID, g.context.Workflows.GitBranchPolicy.Title),
				fmt.Sprintf("**Type**: %s\n", g.context.Workflows.GitBranchPolicy.PolicyType),
				formatPolicyBody(g.context.Workflows.GitBranchPolicy.Body))
		}

		if g.context.Workflows.PRPolicy != nil {
			parts = append(parts,
				fmt.Sprintf("\n### %s: %s\n", g.context.Workflows.PRPolicy.ID, g.context.Workflows.PRPolicy.Title),
				fmt.Sprintf("**Type**: %s\n", g.context.Workflows.PRPolicy.PolicyType),
				formatPolicyBody(g.context.Workflows.PRPolicy.Body))
		}

		if g.context.Workflows.CommitPolicy != nil {
			parts = append(parts,
				fmt.Sprintf("\n### %s: %s\n", g.context.Workflows.CommitPolicy.ID, g.context.Workflows.CommitPolicy.Title),
				formatPolicyBody(g.context.Workflows.CommitPolicy.Body))
		}
	}

	// Lifecycle Understanding
	if !isCoder && g.context.Lifecycles != nil && len(g.context.Lifecycles.ObjectKinds) > 0 {
		enumerator := NewLifecycleEnumerator(g.context.Lifecycles)
		lifecycleOverview := enumerator.FormatLifecycleOverview()
		if lifecycleOverview != emptyValue {
			parts = append(parts, "\n", lifecycleOverview)
		}
	}

	// IA Ontology injection
	if isIA {
		parts = append(parts, "\n## Information Architecture (Ontology)\n")
		parts = append(parts, "**Focus**: Maintain taxonomy, specs, definitions, and rule objects.\n")
	}

	// Role-Specific Guidance
	parts = append(parts, g.generateRoleSpecificGuidance())

	// Code Quality Policies (for developers)
	if g.role == "developer" || g.role == "admin" {
		if g.context.Policies != nil && len(g.context.Policies.Code) > 0 {
			parts = append(parts, "\n## Code Quality Policies\n")
			for _, policy := range g.context.Policies.Code[:minInt(5, len(g.context.Policies.Code))] { // Top 5
				parts = append(parts, fmt.Sprintf("- **%s**: %s (%s)\n", policy.ID, policy.Title, policy.PolicyType))
			}
		}
	}

	return strings.Join(parts, "")
}

// pullObserverMetrics pulls leading/trailing metrics from storage
func (g *RoleAwarePromptGenerator) pullObserverMetrics() string {
	if g.storage == nil {
		return "- Metrics unavailable (storage disconnected)\n"
	}

	ctx := pkgctx.NewSystemContext()
	secCtx, _ := g.secCtx.(*pkgctx.SecurityContext)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()

	var metricsOutput []string

	// Helper to pull a specific metric kind
	pullMetric := func(kind string) {
		filter := map[string]any{
			objects.FieldKeyKind: kind,
			"limit":              5,
		}
		resultAny, err := g.storage.List(ctx, secCtx, storageCtx, filter)
		if err == nil {
			adapter := AdaptStorageResult(resultAny)
			if len(adapter.Objects) > 0 {
				metricsOutput = append(metricsOutput, fmt.Sprintf("### %s\n", kind))
				for _, obj := range adapter.Objects {
					id, _ := obj[objects.FieldKeyID].(string)
					title, _ := obj[objects.FieldKeyTitle].(string)
					metricsOutput = append(metricsOutput, fmt.Sprintf("- %s: %s\n", id, title))
				}
			}
		}
	}

	pullMetric("scheduler_health_metric")
	pullMetric("file_lock_metric")
	pullMetric("test_audit_aggregation_metric")
	pullMetric("metrics_feedback")

	if len(metricsOutput) == 0 {
		return "- No operational metrics found.\n"
	}
	return strings.Join(metricsOutput, "")
}

// generateRoleSpecificGuidance generates role-specific guidance from role objects
func (g *RoleAwarePromptGenerator) generateRoleSpecificGuidance() string {
	var parts []string
	parts = append(parts,
		"\n## Your Role and Responsibilities\n",
		fmt.Sprintf("**Your Role**: %s\n", g.role),
		fmt.Sprintf("**Your Roles**: %s\n", strings.Join(g.roles, ", ")))

	// Load role guidance from role objects; use safe fallback if not available
	formattedGuidance := g.loadAndFormatRoleGuidance()
	if formattedGuidance == emptyValue {
		formattedGuidance = fmt.Sprintf("\n### Guidance for %s\nOperate within assigned role scope and comply with kernel integrity constraints.\n", g.role)
	}

	parts = append(parts, formattedGuidance)
	return strings.Join(parts, "")
}

// loadAndFormatRoleGuidance loads and formats role guidance from role objects
// Returns empty string if loading fails (caller should fail hard)
func (g *RoleAwarePromptGenerator) loadAndFormatRoleGuidance() string {
	if g.roleGuidanceDepsMissing() {
		return emptyValue // Caller will fail hard
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		Roles: g.roles,
	}

	// Load guidance for primary role
	guidance, err := g.guidanceGenerator.GenerateRoleGuidance(ctx, secCtx, g.role)
	if err != nil || guidance == nil {
		return emptyValue // Caller will fail hard
	}

	// Format and return role object-based guidance
	return g.guidanceGenerator.FormatRoleGuidance(ctx, secCtx, guidance)
}

// GenerateExecutionContextPrompt generates a prompt focused on execution context
func (g *RoleAwarePromptGenerator) GenerateExecutionContextPrompt() string {
	var parts []string

	parts = append(parts,
		"# Execution Context\n\n",
		"This prompt provides the critical execution context you need to operate effectively.\n\n")

	// Current Priority Plans (most immediate context)
	if g.context.CurrentState != nil && len(g.context.CurrentState.ActivePriorityPlans) > 0 {
		parts = append(parts, "## Current Priority Plans (Active Work)\n\n")
		for _, pp := range g.context.CurrentState.ActivePriorityPlans {
			parts = append(parts,
				fmt.Sprintf("**%s** (%s): %s\n", pp.ID, pp.Priority, pp.Title),
				fmt.Sprintf("- Status: %s\n", pp.Status),
				"\n")
		}
		parts = append(parts, "**Action**: Focus your work on these priority plans. All backlog items should be linked to one of these plans.\n\n")
	}

	// Workflow Requirements
	if g.context.Workflows != nil {
		parts = append(parts, "## Mandatory Workflow Requirements\n\n")

		if g.context.Workflows.GitBranchPolicy != nil {
			parts = append(parts,
				"### Git Branch Workflow\n",
				formatPolicyBody(g.context.Workflows.GitBranchPolicy.Body),
				"\n")
		}

		if g.context.Workflows.PRPolicy != nil {
			parts = append(parts,
				"### Pull Request Workflow\n",
				formatPolicyBody(g.context.Workflows.PRPolicy.Body),
				"\n")
		}
	}

	// Lifecycle Requirements
	if g.context.Lifecycles != nil {
		enumerator := NewLifecycleEnumerator(g.context.Lifecycles)
		lifecycleRequirements := enumerator.FormatLifecycleRequirements()
		if lifecycleRequirements != emptyValue {
			parts = append(parts, lifecycleRequirements)
		}
	}

	return strings.Join(parts, "")
}

// formatPolicyBody formats policy body text for display
func formatPolicyBody(body string) string {
	// Simple formatting - preserve markdown-like structure
	lines := strings.Split(body, "\n")
	var formatted []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == emptyValue {
			formatted = append(formatted, "")
			continue
		}
		// Preserve markdown formatting
		formatted = append(formatted, line)
	}
	return strings.Join(formatted, "\n")
}

// GenerateMyRolePrompt generates a prompt showing the agent's current role and permissions.
// Returns fallback content when role objects are unavailable (no panic).
func (g *RoleAwarePromptGenerator) GenerateMyRolePrompt() string {
	var parts []string

	parts = append(parts,
		"# Your Current Role and Permissions\n\n",
		"## Your Role Assignment\n\n",
		fmt.Sprintf("**Primary Role**: %s\n", g.role))
	if len(g.roles) > 1 {
		parts = append(parts, fmt.Sprintf("**All Roles**: %s\n", strings.Join(g.roles, ", ")), "\n")
	} else {
		parts = append(parts, "\n")
	}

	permissionsText := g.generatePermissionsFromRoleObjects()
	if permissionsText == emptyValue {
		parts = append(parts,
			"## Your Permissions\n\n",
			"Role objects are not available in storage, so permissions could not be loaded.\n\n",
			"**What you can do**: Use `tools/list` to see the tools available to you. Use `prompts/get` with name=\"role_based_access\" to understand how permissions work.\n")
		return strings.Join(parts, "")
	}

	parts = append(parts, "## Your Permissions\n\n", permissionsText)

	if g.roleGuidanceDepsMissing() {
		return strings.Join(parts, "")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		Roles: g.roles,
	}

	guidance, err := g.guidanceGenerator.GenerateRoleGuidance(ctx, secCtx, g.role)
	if err != nil || guidance == nil {
		return strings.Join(parts, "")
	}

	availableOpsText := g.generateAvailableOperationsText(guidance)
	if availableOpsText != emptyValue {
		parts = append(parts, "\n", availableOpsText)
	}

	formattedGuidance := g.guidanceGenerator.FormatRoleGuidance(ctx, secCtx, guidance)
	if formattedGuidance != emptyValue {
		parts = append(parts, "\n", formattedGuidance)
	}

	accessUpgradeText := g.generateAccessUpgradeInstructions()
	if accessUpgradeText != emptyValue {
		parts = append(parts, "\n", accessUpgradeText)
	}

	return strings.Join(parts, "")
}

// generateAvailableOperationsText generates the "Available Operations" section text from role guidance
// Loads from prompt templates or role object fields - fails hard if not available
func (g *RoleAwarePromptGenerator) generateAvailableOperationsText(guidance *RoleGuidance) string {
	// Try to get from prompt templates first
	if guidance.PromptTemplates != nil {
		if template, ok := guidance.PromptTemplates["my_role_operations"]; ok && template.Content != emptyValue {
			return template.Content
		}
		if template, ok := guidance.PromptTemplates["available_operations"]; ok && template.Content != emptyValue {
			return template.Content
		}
	}

	// Try to get from role object field
	// This would require loading the role object again, but for now we'll fail hard
	// The text should be in prompt_templates["my_role_operations"] or similar
	return emptyValue // Empty means caller should fail hard or skip
}

// Returns empty string if role objects are not available (caller should fail hard)
func (g *RoleAwarePromptGenerator) generatePermissionsFromRoleObjects() string {
	if g.roleGuidanceDepsMissing() {
		return emptyValue // Caller will fail hard
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		Roles: g.roles,
	}

	// Load guidance for primary role
	guidance, err := g.guidanceGenerator.GenerateRoleGuidance(ctx, secCtx, g.role)
	if err != nil || guidance == nil {
		return emptyValue // Caller will fail hard
	}

	// Format permissions from role object
	var parts []string
	if len(guidance.Permissions) > 0 {
		// Capitalize first letter of role name
		roleTitle := g.role
		if len(roleTitle) > 0 {
			roleTitle = strings.ToUpper(roleTitle[:1]) + roleTitle[1:]
		}
		parts = append(parts, fmt.Sprintf("**%s Access**:\n", roleTitle))

		// Group permissions by type
		readPerms := []string{}
		writePerms := []string{}
		deletePerms := []string{}
		executePerms := []string{}
		otherPerms := []string{}

		for _, perm := range guidance.Permissions {
			if strings.HasPrefix(perm, "read:") {
				readPerms = append(readPerms, perm)
			} else if strings.HasPrefix(perm, "write:") {
				writePerms = append(writePerms, perm)
			} else if strings.HasPrefix(perm, "delete:") {
				deletePerms = append(deletePerms, perm)
			} else if strings.HasPrefix(perm, "execute:") {
				executePerms = append(executePerms, perm)
			} else {
				otherPerms = append(otherPerms, perm)
			}
		}

		// Format read permissions
		if len(readPerms) > 0 {
			parts = append(parts, g.formatPermissionGroup("Read", readPerms))
		}

		// Format write permissions
		if len(writePerms) > 0 {
			parts = append(parts, g.formatPermissionGroup("Write", writePerms))
		} else {
			parts = append(parts, "- No write permissions\n")
		}

		// Format delete permissions
		if len(deletePerms) > 0 {
			parts = append(parts, g.formatPermissionGroup("Delete", deletePerms))
		} else {
			parts = append(parts, "- No delete permissions\n")
		}

		// Format execute permissions
		if len(executePerms) > 0 {
			parts = append(parts, g.formatPermissionGroup("Execute", executePerms))
		}

		// Format other permissions
		if len(otherPerms) > 0 {
			parts = append(parts, g.formatPermissionGroup("Other", otherPerms))
		}
	}

	return strings.Join(parts, "")
}

// formatPermissionGroup formats a group of permissions with descriptions
func (g *RoleAwarePromptGenerator) formatPermissionGroup(category string, perms []string) string {
	var parts []string

	// Check if all permissions (wildcard)
	hasAll := false
	for _, perm := range perms {
		if strings.HasSuffix(perm, ":*") {
			hasAll = true
			break
		}
	}

	if hasAll {
		// Format as "all objects" if wildcard
		op := strings.TrimSuffix(perms[0], ":*")
		parts = append(parts, fmt.Sprintf("- %s:* (%s all objects)\n", op, strings.ToLower(op)))
	} else {
		// Format as list of specific permissions
		if len(perms) <= 3 {
			// Show all if few permissions
			for _, perm := range perms {
				parts = append(parts, fmt.Sprintf("- %s\n", perm))
			}
		} else {
			// Show first few and count
			for i := 0; i < 3 && i < len(perms); i++ {
				parts = append(parts, fmt.Sprintf("- %s\n", perms[i]))
			}
			parts = append(parts, fmt.Sprintf("- ... and %d more (%s access to specific object kinds)\n", len(perms)-3, strings.ToLower(category)))
		}
	}

	return strings.Join(parts, "")
}

// generateAccessUpgradeInstructions generates access upgrade instructions from role objects
func (g *RoleAwarePromptGenerator) generateAccessUpgradeInstructions() string {
	// Only show for roles with limited access (viewer, or roles without write permissions)
	// Check if role has write permissions
	hasWriteAccess := false
	if g.guidanceGenerator != nil && g.secCtx != nil {
		ctx := pkgctx.NewSystemContext()
		secCtx := &pkgctx.SecurityContext{
			Roles: g.roles,
		}

		guidance, err := g.guidanceGenerator.GenerateRoleGuidance(ctx, secCtx, g.role)
		if err == nil && guidance != nil {
			// Check if role has any write permissions
			for _, perm := range guidance.Permissions {
				if strings.HasPrefix(perm, "write:") {
					hasWriteAccess = true
					break
				}
			}

			// If role has access upgrade instructions, use them
			if len(guidance.AccessUpgradeSteps) > 0 {
				var parts []string
				parts = append(parts, "## Need More Access?\n\n")

				// Determine prompt name
				promptName := guidance.AccessUpgradePrompt
				if promptName == emptyValue {
					promptName = "role_based_access" // Default
				}

				// Add steps
				for i, step := range guidance.AccessUpgradeSteps {
					parts = append(parts, fmt.Sprintf("%d. %s\n", i+1, step))
				}

				// Add prompt reference if available
				if promptName != emptyValue {
					parts = append(parts, fmt.Sprintf("%d. Use prompts/get with name=%q for more details\n", len(guidance.AccessUpgradeSteps)+1, promptName))
				}

				parts = append(parts, "\n")
				return strings.Join(parts, "")
			}
		}
	}

	// When role has limited access but no upgrade steps in storage, return empty (no panic)
	if !hasWriteAccess {
		return emptyValue
	}

	return emptyValue
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
