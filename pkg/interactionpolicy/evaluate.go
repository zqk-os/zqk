package interactionpolicy

import (
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// Result is the ping-pong payload for CLI / hooks.
type Result struct {
	Matched bool   `json:"matched"`
	Event   string `json:"event"`
	Step    *Step  `json:"step,omitempty"`
}

// Evaluate returns the first catalog step for event that the persona is
// allowed to receive. Empty persona (or no POL refs) uses catalog defaults
// so ping-pong works before personas are bound.
func Evaluate(persona map[string]any, event string) Result {
	event = strings.TrimSpace(event)
	out := Result{Event: event}
	if event == "" {
		return out
	}
	candidates := stepsForEvent(event)
	if len(candidates) == 0 {
		return out
	}
	bound := objects.CollectPersonaInteractionPolicyRefs(persona)
	if len(bound) == 0 {
		s := candidates[0]
		out.Matched = true
		out.Step = &s
		return out
	}
	allow := make(map[string]struct{}, len(bound))
	for _, id := range bound {
		allow[id] = struct{}{}
	}
	for _, s := range candidates {
		if _, ok := allow[s.PolicyID]; ok {
			cp := s
			out.Matched = true
			out.Step = &cp
			return out
		}
	}
	return out
}
