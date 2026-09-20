package agentprompt

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// TaskEnvelopeMarker is the persist contract for agent_task.description.
// Static policy/skill/AST bodies must not be copied into the CAS object.
// TRACK: BLI-1787805421435713000-3cf3884a — GLS-1787805412435495000-332acc7b
const TaskEnvelopeMarker = "zqk_task_envelope_v1"

// PromptLayer selects persist (CAS) vs execute (ephemeral) assembly.
type PromptLayer string

const (
	// PromptLayerExecute assembles a worker-facing prompt from refs (default).
	PromptLayerExecute PromptLayer = ""
	// PromptLayerPersist writes the compact envelope only — no policy/skill bodies, no AST census.
	PromptLayerPersist PromptLayer = "persist"
)

// TaskEnvelope is the durable ATK persist shape: dynamic facts + kernel refs.
type TaskEnvelope struct {
	Description string
	PolicyRefs  []string
	SkillRefs   []string
}

// IsTaskEnvelope reports whether s is a persist-layer envelope (not a pasted prompt catalog).
func IsTaskEnvelope(s string) bool {
	return strings.Contains(s, TaskEnvelopeMarker)
}

// BuildTaskEnvelope loads persona-bound refs and formats a compact persist description.
func BuildTaskEnvelope(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, projectRoot string, opts TaskPromptOptions) (*TaskEnvelope, error) {
	personaID := resolvePersonaID(opts)
	policyIDs := append([]string{}, StandingPolicyRefs()...)
	if personaID != "" && strings.HasPrefix(strings.ToUpper(personaID), "PER-") && sp != nil {
		if persona, err := sp.Read(ctx, secCtx, personaID); err == nil && persona != nil {
			policyIDs = uniqueIDs(policyIDs, objects.CollectPersonaInteractionPolicyRefs(persona))
		}
	}

	var skillIDs []string
	skillEnforcement, err := LoadRelevantSkillsOpts(ctx, sp, secCtx, skillLoadOpts(opts, personaID, projectRoot))
	if err != nil {
		return nil, fmt.Errorf("skill seal verification failed (fail-closed): %w", err)
	}
	if skillEnforcement != nil {
		skillIDs = objectIDs(skillEnforcement.RelevantSkills)
	}

	return &TaskEnvelope{
		Description: FormatTaskEnvelope(opts, policyIDs, skillIDs),
		PolicyRefs:  policyIDs,
		SkillRefs:   skillIDs,
	}, nil
}

// FormatTaskEnvelope renders the persist-layer markdown. No policy/skill bodies.
func FormatTaskEnvelope(opts TaskPromptOptions, policyRefs, skillRefs []string) string {
	var sb strings.Builder
	sb.WriteString("# Agent Task Envelope\n")
	sb.WriteString(TaskEnvelopeMarker)
	sb.WriteString("\n\n")
	sb.WriteString("Static context lives in kernel objects. Do not treat this description as a copy of policy or skill bodies.\n")
	sb.WriteString(paths.RewriteCanonicalCLIInvocations("Resolve at execute: `zqk object get <id>`, `zqk agent prepare-context --task-id <ATK>`, or `zqk workflow whats-next`.\n\n"))

	sb.WriteString("## Dynamic\n")
	if opts.PlanTitle != "" || opts.PlanID != "" {
		sb.WriteString(fmt.Sprintf("- Plan: %s (%s)\n", opts.PlanTitle, opts.PlanID))
	}
	if opts.SessionID != "" {
		sb.WriteString(fmt.Sprintf("- Session: %s\n", opts.SessionID))
	}
	if opts.TargetAgent != "" {
		sb.WriteString(fmt.Sprintf("- Target: %s (capability: %s)\n", opts.TargetAgent, opts.Capability))
	}
	if opts.PersonaID != "" {
		sb.WriteString(fmt.Sprintf("- Persona: %s\n", opts.PersonaID))
	}
	if opts.TaskTitle != "" {
		sb.WriteString(fmt.Sprintf("- Task: %s\n", opts.TaskTitle))
	}
	if opts.TaskContext != "" {
		sb.WriteString("\n### Context\n")
		sb.WriteString(opts.TaskContext)
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Standing refs (do not duplicate bodies)\n")
	for _, id := range StandingPolicyRefs() {
		sb.WriteString(paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("- `%s` — `zqk object get %s`\n", id, id)))
	}

	if len(policyRefs) > 0 {
		sb.WriteString("\n## Bound policies\n")
		for _, id := range policyRefs {
			sb.WriteString(fmt.Sprintf("- `%s`\n", id))
		}
	}
	if len(skillRefs) > 0 {
		sb.WriteString("\n## Bound skills\n")
		for _, id := range skillRefs {
			sb.WriteString(paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("- `%s` — `zqk object get %s`\n", id, id)))
		}
	}

	sb.WriteString("\n## Observer\n")
	sb.WriteString("Do not persist an AST census. At execute time call observer_search, then read_code the returned path.\n")
	return sb.String()
}

func resolvePersonaID(opts TaskPromptOptions) string {
	if id := strings.TrimSpace(opts.PersonaID); id != "" {
		return id
	}
	target := strings.TrimSpace(opts.TargetAgent)
	if strings.HasPrefix(strings.ToUpper(target), "PER-") {
		return target
	}
	return ""
}

func skillLoadOpts(opts TaskPromptOptions, personaID, projectRoot string) SkillLoadOptions {
	return SkillLoadOptions{
		TaskDescription: fmt.Sprintf("Process task: %s on plan %s", opts.TaskTitle, opts.PlanTitle),
		PersonaID:       personaID,
		ExtraQuery:      strings.TrimSpace(opts.TargetAgent + " " + opts.Capability),
		ProjectRoot:     projectRoot,
	}
}

func objectIDs(objs []map[string]any) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, obj := range objs {
		id, _ := obj[objects.FieldKeyID].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func uniqueIDs(base []string, extra []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, id := range append(append([]string{}, base...), extra...) {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
