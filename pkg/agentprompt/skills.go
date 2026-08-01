package agentprompt

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// SkillEnforcement represents the compiled relevant skills for an agent.
type SkillEnforcement struct {
	RelevantSkills []map[string]any
}

// LoadRelevantSkills fetches agent_skill objects relevant to the given task description.
func LoadRelevantSkills(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, taskDescription string) (*SkillEnforcement, error) {
	storageCtx := &pkgctx.StorageContext{}

	// Fetch all skills first (could be optimized with semantic search/vector query later)
	filter := storage.ListFilter{
		Kind:  objects.KindAgentSkill,
		Limit: 0,
	}

	result, err := sp.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to list agent skills from kernel").Wrap(err)
	}

	var relevant []map[string]any
	taskLower := strings.ToLower(taskDescription)

	for _, skill := range result.Objects {
		title, _ := skill[objects.FieldKeyTitle].(string)
		desc, _ := skill[objects.FieldKeyDescription].(string)
		summary, _ := skill[objects.FieldKeyInstructionsSummary].(string)

		// Basic keyword matching
		if strings.Contains(taskLower, strings.ToLower(title)) ||
			strings.Contains(taskLower, strings.ToLower(desc)) ||
			(summary != "" && strings.Contains(taskLower, strings.ToLower(summary))) {
			relevant = append(relevant, skill)
		}
	}

	return &SkillEnforcement{
		RelevantSkills: relevant,
	}, nil
}

// GeneratePromptSection formats the relevant skills into a markdown section.
func (s *SkillEnforcement) GeneratePromptSection() string {
	if len(s.RelevantSkills) == 0 {
		return "## Relevant Agent Skills\nNo specialized skills discovered for this task.\n"
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Agent Skills\n")
	sb.WriteString("The following specialized project skills have been identified as relevant to your current task. Use their codified mandates to ensure high-quality execution:\n\n")

	for _, skill := range s.RelevantSkills {
		title, _ := skill[objects.FieldKeyTitle].(string)
		desc, _ := skill[objects.FieldKeyDescription].(string)
		id, _ := skill[objects.FieldKeyID].(string)
		path, _ := skill[objects.FieldKeyFilePath].(string)

		sb.WriteString(fmt.Sprintf("### %s (%s)\n", title, id))
		sb.WriteString(fmt.Sprintf("- **Location**: `%s`\n", path))
		if desc != "" {
			sb.WriteString(fmt.Sprintf("- **Focus**: %s\n", desc))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
