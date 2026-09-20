package agentprompt

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// PolicyEnforcement represents the compiled policy constraints for an agent.
type PolicyEnforcement struct {
	ActivePolicies []map[string]any
}

// LoadActivePolicies fetches all active policy objects from the Knowledge Kernel.
// Prefer LoadBoundPolicies for task prompts — the full catalog must not be inlined.
func LoadActivePolicies(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) (*PolicyEnforcement, error) {
	storageCtx := &pkgctx.StorageContext{}
	filter := storage.ListFilter{
		Kind: objects.KindPolicy,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		Limit: 0,
	}

	result, err := sp.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}

	return &PolicyEnforcement{
		ActivePolicies: result.Objects,
	}, nil
}

// LoadBoundPolicies loads standing + persona-bound policies by ID (not the whole catalog).
func LoadBoundPolicies(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, personaID string) (*PolicyEnforcement, error) {
	if sp == nil {
		return &PolicyEnforcement{}, nil
	}
	ids := append([]string{}, StandingPolicyRefs()...)
	if pid := strings.TrimSpace(personaID); pid != "" && strings.HasPrefix(strings.ToUpper(pid), "PER-") {
		if persona, err := sp.Read(ctx, secCtx, pid); err == nil && persona != nil {
			ids = uniqueIDs(ids, objects.CollectPersonaInteractionPolicyRefs(persona))
		}
	}
	var policies []map[string]any
	for _, id := range ids {
		obj, err := sp.Read(ctx, secCtx, id)
		if err != nil || obj == nil {
			continue
		}
		policies = append(policies, obj)
	}
	return &PolicyEnforcement{ActivePolicies: policies}, nil
}

// GeneratePromptSection lists bound policies as ID+title refs. Bodies stay in the kernel.
func (p *PolicyEnforcement) GeneratePromptSection() string {
	return p.GeneratePromptSectionRefs()
}

// GeneratePromptSectionRefs is the default policy section: links, not copied bodies.
func (p *PolicyEnforcement) GeneratePromptSectionRefs() string {
	if len(p.ActivePolicies) == 0 {
		return "## Policy Compliance\nBound by the active policy catalog. Resolve with `zqk object list policy --filter status=active` or `zqk object get <POL-id>`.\n"
	}

	var sb strings.Builder
	sb.WriteString("## Policy Compliance\n")
	sb.WriteString("You are bound by these policies. Resolve bodies with `zqk object get <POL-id>`. Do not treat this list as a copy of policy text.\n\n")

	for _, policy := range p.ActivePolicies {
		title, _ := policy[objects.FieldKeyTitle].(string)
		id, _ := policy[objects.FieldKeyID].(string)
		if title == "" {
			title = "Unnamed Policy"
		}
		sb.WriteString(fmt.Sprintf("- `%s` — %s\n", id, title))
	}
	sb.WriteString("\n")
	return sb.String()
}

// GeneratePromptSectionBodies inlines policy descriptions. Do not persist this on agent_task.
func (p *PolicyEnforcement) GeneratePromptSectionBodies() string {
	if len(p.ActivePolicies) == 0 {
		return "## Policy Compliance\nNo active policies found.\n"
	}

	var sb strings.Builder
	sb.WriteString("## Policy Compliance\n")
	sb.WriteString("You are bound by the following active project policies. You MUST adhere to these constraints during execution:\n\n")

	for _, policy := range p.ActivePolicies {
		title, _ := policy[objects.FieldKeyTitle].(string)
		desc, _ := policy[objects.FieldKeyDescription].(string)
		id, _ := policy[objects.FieldKeyID].(string)

		if title == "" {
			title = "Unnamed Policy"
		}

		sb.WriteString(fmt.Sprintf("### %s (%s)\n", title, id))
		if desc != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", desc))
		}
	}

	return sb.String()
}
