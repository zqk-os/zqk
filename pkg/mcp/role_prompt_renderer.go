package mcp

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// RolePromptRenderer renders prompts with role-specific context
type RolePromptRenderer struct {
	guidanceGenerator *RoleGuidanceGenerator
	generator         *RoleAwarePromptGenerator
	secCtx            *pkgctx.SecurityContext
}

// NewRolePromptRenderer creates a new role prompt renderer
func NewRolePromptRenderer(
	guidanceGen *RoleGuidanceGenerator,
	promptGen *RoleAwarePromptGenerator,
	secCtx *pkgctx.SecurityContext,
) *RolePromptRenderer {
	return &RolePromptRenderer{
		guidanceGenerator: guidanceGen,
		generator:         promptGen,
		secCtx:            secCtx,
	}
}

// rolePromptRendererDepsMissing reports whether rendering cannot run (missing generator or security context).
func (r *RolePromptRenderer) rolePromptRendererDepsMissing() bool {
	return r.guidanceGenerator == nil || r.secCtx == nil
}

// RenderPrompt renders a prompt with role-specific context
// Returns the rendered content and whether it was found in role objects
func (r *RolePromptRenderer) RenderPrompt(ctx context.Context, promptName string) (string, bool) {
	if r.rolePromptRendererDepsMissing() {
		return "", false
	}

	// Get primary role
	roleID := "viewer"
	if len(r.secCtx.Roles) > 0 {
		roleID = r.secCtx.Roles[0]
	}

	// Load role guidance
	guidance, err := r.guidanceGenerator.GenerateRoleGuidance(ctx, r.secCtx, roleID)
	if err != nil || guidance == nil {
		return "", false
	}

	// Check if prompt template exists for this role
	template, exists := guidance.PromptTemplates[promptName]
	if !exists {
		return "", false
	}

	// Render template with role context
	content := r.renderTemplate(template.Content, guidance, roleID, promptName)

	return content, true
}

// renderTemplate renders a template string with role context substitutions
func (r *RolePromptRenderer) renderTemplate(template string, guidance *RoleGuidance, roleID string, promptName string) string {
	content := template

	// Standard variable substitutions
	substitutions := map[string]string{
		"{{role}}":            roleID,
		"{{role_title}}":      strings.Title(roleID),
		"{{description}}":     guidance.Description,
		"{{influence_level}}": guidance.InfluenceLevel,
		"{{system_name}}":     "ZQK", // Can be made configurable
		"{{mcp_server_name}}": "ZQK MCP Server",
	}

	// Add permissions context
	if len(guidance.Permissions) > 0 {
		readPerms := []string{}
		writePerms := []string{}
		for _, perm := range guidance.Permissions {
			if strings.HasPrefix(perm, "read:") {
				readPerms = append(readPerms, perm)
			} else if strings.HasPrefix(perm, "write:") {
				writePerms = append(writePerms, perm)
			}
		}
		if len(readPerms) > 0 {
			substitutions["{{read_permissions}}"] = strings.Join(readPerms, ", ")
		}
		if len(writePerms) > 0 {
			substitutions["{{write_permissions}}"] = strings.Join(writePerms, ", ")
		} else {
			substitutions["{{write_permissions}}"] = "none (read-only access)"
		}
	}

	// Add responsibilities context
	if len(guidance.Responsibilities) > 0 {
		respList := []string{}
		for _, resp := range guidance.Responsibilities {
			respList = append(respList, fmt.Sprintf("- %s", resp.Description))
		}
		substitutions["{{responsibilities}}"] = strings.Join(respList, "\n")
	}

	// Add privileges context
	if len(guidance.Privileges) > 0 {
		substitutions["{{privileges}}"] = strings.Join(guidance.Privileges, "\n- ")
	}

	// Add restrictions context
	if len(guidance.Restrictions) > 0 {
		substitutions["{{restrictions}}"] = strings.Join(guidance.Restrictions, "\n- ")
	}

	// Apply template variable substitutions from role object
	if template, ok := guidance.PromptTemplates[promptName]; ok {
		for k, v := range template.Variables {
			substitutions[fmt.Sprintf("{{%s}}", k)] = v
		}
	}

	// Apply all substitutions
	for placeholder, value := range substitutions {
		content = strings.ReplaceAll(content, placeholder, value)
	}

	// Apply tool name substitutions (e.g., {{tool:system_status}})
	content = r.substituteToolNames(content)

	return content
}

// substituteToolNames substitutes tool name placeholders (e.g., {{tool:system_status}})
func (r *RolePromptRenderer) substituteToolNames(content string) string {
	// Find all {{tool:name}} patterns
	start := 0
	for {
		startIdx := strings.Index(content[start:], "{{tool:")
		if startIdx == -1 {
			break
		}
		startIdx += start
		endIdx := strings.Index(content[startIdx:], "}}")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx

		// Extract tool name
		toolName := content[startIdx+7 : endIdx] // Skip "{{tool:" prefix
		toolName = strings.TrimSpace(toolName)

		// Get tool name
		displayName := GetToolName(toolName)
		if displayName == emptyValue {
			displayName = toolName
		}

		// Replace placeholder
		content = content[:startIdx] + displayName + content[endIdx+2:]
		start = startIdx + len(displayName)
	}

	return content
}

// GetStandardPromptNames returns the list of standard prompt names
func GetStandardPromptNames() []string {
	return []string{
		"welcome",
		"getting_started",
		"query_help",
		"common_tasks",
		"object_lifecycle",
		"filter_syntax",
		"role_based_access",
		"create_object_template",
		"execution_context",
		"big_picture",
		"my_role",
		"current_role",
	}
}
