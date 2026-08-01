package agentprompt

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// PolicyEnforcement represents the compiled policy constraints for an agent.
type PolicyEnforcement struct {
	ActivePolicies []map[string]any
}

// LoadActivePolicies fetches all active policy objects from the Knowledge Kernel.
func LoadActivePolicies(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) (*PolicyEnforcement, error) {
	storageCtx := &pkgctx.StorageContext{}
	filter := storage.ListFilter{
		Kind: objects.KindPolicy,
		Filters: map[string]any{
			objects.FieldKeyStatus: "active",
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

// GeneratePromptSection generates a markdown section embedding all active policies
// to bind the sub-agent's execution strictly to project guidelines.
func (p *PolicyEnforcement) GeneratePromptSection() string {
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
