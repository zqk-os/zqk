package agent

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/objects"
)

func orchestrationCLI() string {
	name := strings.TrimSpace(brand.ExecutableName())
	if name == "" {
		return brand.CanonicalExecutableToken
	}
	return name
}

func personaRoleFromObject(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	role, _ := obj[objects.FieldKeyRole].(string)
	return strings.TrimSpace(role)
}

func orchestrationEstimatedEffort(item map[string]any) string {
	if item == nil {
		return ""
	}
	if s, ok := item[objects.FieldKeyEstimatedEffort].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func normalizePersonaRoleToken(role, agentType string) string {
	token := strings.ToLower(strings.TrimSpace(role))
	if token == "" {
		token = strings.ToLower(strings.TrimSpace(agentType))
	}
	token = strings.ReplaceAll(token, "-", "_")
	return strings.ReplaceAll(token, " ", "_")
}

type orchestrationTaskBoundary struct {
	GraphCompose bool
	CodeValidate bool
}

// orchestrationTaskBoundaryFor uses kernel persona.role / agent_type.
// Do not sniff persona IDs for "tpm", "design", or "architect".
func orchestrationTaskBoundaryFor(role, agentType string, workClass agentprompt.WorkClass) orchestrationTaskBoundary {
	token := normalizePersonaRoleToken(role, agentType)
	b := orchestrationTaskBoundary{}
	switch token {
	case "operator", "technical_program_manager", "planner", "designer":
		b.GraphCompose = true
	case "system_architect":
		b.GraphCompose = true
		b.CodeValidate = true
	case "agent", "software_engineer", "coder_agent", "coder", "qa_auditor", "test_agent", "devops_agent":
		b.CodeValidate = true
	default:
		if workClass.IsDocsEval() {
			b.GraphCompose = true
		} else {
			b.CodeValidate = true
		}
	}
	if workClass.IsDocsEval() {
		b.CodeValidate = false
	}
	return b
}

func buildOrchestrationTaskSteps(cli string, boundary orchestrationTaskBoundary) []map[string]any {
	cli = strings.TrimSpace(cli)
	if cli == "" {
		cli = orchestrationCLI()
	}
	steps := []map[string]any{
		{
			objects.FieldKeyTitle:       "Implementation Phase",
			objects.FieldKeyStatus:      objects.ObjectStatusPendingImplementation,
			objects.FieldKeyDescription: fmt.Sprintf("Resolve the task envelope refs (`%s object get`, `%s agent prepare-context`) and execute the dynamic task. Do not copy policy or skill bodies onto the task object.", cli, cli),
		},
	}
	if boundary.GraphCompose {
		steps = append(steps, map[string]any{
			objects.FieldKeyTitle:       "Kernel Graph Composition Context",
			objects.FieldKeyStatus:      objects.ObjectStatusPending,
			objects.FieldKeyDescription: fmt.Sprintf("Kernel graph composition: use `%s object get` for the onboarding policy, then `%s intake` / `%s object import`; do not script `%s object create` loops.", cli, cli, cli, cli),
		})
	}
	if boundary.CodeValidate {
		steps = append(steps, map[string]any{
			objects.FieldKeyTitle:       "Validation Phase",
			"verification_strategy":     "command_exit_code",
			objects.FieldKeyStatus:      objects.ObjectStatusPending,
			objects.FieldKeyDescription: "Must pass targeted validation script.",
			objects.FieldKeyCommand:     cli + " agent validate",
		})
	}
	return steps
}
