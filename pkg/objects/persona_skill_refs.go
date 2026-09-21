package objects

import (
	"strings"
)

// CollectPersonaASKRefs returns ASK-* ids linked on a persona via canonical
// agent_skill_refs and/or related_object_refs (CRI-PERSONA-SKILL-BOUND dual-read).
// TRACK: follow-up in kernel backlog
func CollectPersonaASKRefs(persona map[string]any) []string {
	if persona == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	appendASK := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if !strings.HasPrefix(strings.ToUpper(id), "ASK-") {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range stringListField(persona, FieldKeyAgentSkillRefs) {
		appendASK(id)
	}
	for _, id := range stringListField(persona, FieldKeyRelatedObjectRefs) {
		appendASK(id)
	}
	return out
}

func stringListField(obj map[string]any, key string) []string {
	v, ok := obj[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		// Comma-separated or single id (legacy scalar agent_skill_refs).
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if x := strings.TrimSpace(p); x != "" {
				out = append(out, x)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			if x := strings.TrimSpace(s); x != "" {
				out = append(out, x)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				if x := strings.TrimSpace(s); x != "" {
					out = append(out, x)
				}
			}
		}
		return out
	default:
		return nil
	}
}
