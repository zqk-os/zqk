package agent

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/federation/meshbroker"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func itemPersonaID(item map[string]any) string {
	if item == nil {
		return ""
	}
	if assignee, ok := item[objects.FieldKeyAssigneePersonaRef].(string); ok {
		if id := strings.TrimSpace(assignee); id != "" {
			return id
		}
	}
	refs := stringIDsFromAny(item[objects.FieldKeyPersonaRefs])
	if len(refs) > 0 {
		return refs[0]
	}
	return ""
}

const defaultOrchestrationPersonaRole = "agent"

func orchestrationPersonaRole(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	personaIDs ...string,
) string {
	if sp == nil {
		return defaultOrchestrationPersonaRole
	}
	for _, id := range personaIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		obj, err := sp.Read(ctx, secCtx, id)
		if err != nil {
			continue
		}
		if role := personaRoleFromObject(obj); role != "" {
			return role
		}
	}
	return defaultOrchestrationPersonaRole
}

func stringIDsFromAny(raw any) []string {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// orchestrationMeshSkillIDs collects skill object ids from the work item and
// seated persona. Community kernels do not ship a studio AGE-* skill.
func orchestrationMeshSkillIDs(item, persona map[string]any) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if item != nil {
		if ref, ok := item[objects.FieldKeySkillRef].(string); ok {
			add(ref)
		}
		for _, id := range objects.CollectPersonaASKRefs(item) {
			add(id)
		}
	}
	for _, id := range objects.CollectPersonaASKRefs(persona) {
		add(id)
	}
	return out
}

func leaseOrchestrationMeshSkill(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	broker *meshbroker.SkillBroker,
	item map[string]any,
	personaID string,
) (section string, endpoint string) {
	if broker == nil {
		return "", ""
	}
	var persona map[string]any
	if pid := strings.TrimSpace(personaID); pid != "" && sp != nil {
		if obj, err := sp.Read(ctx, secCtx, pid); err == nil {
			persona = obj
		}
	}
	for _, skillID := range orchestrationMeshSkillIDs(item, persona) {
		remoteSkill, err := broker.EnsureSkill(ctx, skillID)
		if err != nil || remoteSkill == nil {
			continue
		}
		title := skillID
		if sp != nil {
			if obj, err := sp.Read(ctx, secCtx, skillID); err == nil {
				if t, _ := obj[objects.FieldKeyTitle].(string); strings.TrimSpace(t) != "" {
					title = strings.TrimSpace(t)
				}
			}
		}
		section = fmt.Sprintf("\n## Leased Mesh Capability\n- Skill: %s\n- Provider: %s\n- Token: %s\n- Status: %s\n",
			title, remoteSkill.Provider, remoteSkill.Token, "Authenticated")
		return section, remoteSkill.Endpoint
	}
	return "", ""
}
