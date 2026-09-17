package pipeline

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

const personaRefTagPrefix = "persona_ref:"

// PipelineRouter handles routing logic for priority plans.
type PipelineRouter struct{}

// NewPipelineRouter creates a new PipelineRouter.
func NewPipelineRouter() *PipelineRouter {
	return &PipelineRouter{}
}

// RouteResult holds the output of the routing logic.
type RouteResult struct {
	WorkstreamIDs   []string
	TaskAssignments map[string]string // TaskID -> PER-* persona reference
}

// Route parses a priority plan, extracts workstreams, and assigns tasks to
// kernel persona references. Vocabulary schemes classify work; they are not
// persona IDs and must never be written into assignee_persona_ref.
func (r *PipelineRouter) Route(plan map[string]any) (*RouteResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan cannot be nil")
	}

	result := &RouteResult{
		WorkstreamIDs:   []string{},
		TaskAssignments: make(map[string]string),
	}

	workstreamsRaw, ok := plan["workstreams"]
	if !ok {
		return result, nil
	}

	workstreams, ok := workstreamsRaw.([]any)
	if !ok {
		return result, fmt.Errorf("workstreams must be a list")
	}

	for _, wsRaw := range workstreams {
		ws, ok := wsRaw.(map[string]any)
		if !ok {
			continue
		}

		wsID, _ := ws[objects.FieldKeyID].(string)
		if wsID != "" {
			result.WorkstreamIDs = append(result.WorkstreamIDs, wsID)
		}

		tasksRaw, ok := ws["tasks"]
		if !ok {
			continue
		}

		tasks, ok := tasksRaw.([]any)
		if !ok {
			continue
		}

		for _, taskRaw := range tasks {
			task, ok := taskRaw.(map[string]any)
			if !ok {
				continue
			}

			taskID, _ := task[objects.FieldKeyID].(string)
			if taskID == "" {
				continue
			}

			result.TaskAssignments[taskID] = resolveTaskPersonaRef(task)
		}
	}

	return result, nil
}

func resolveTaskPersonaRef(task map[string]any) string {
	if ref, _ := task[objects.FieldKeyAssigneePersonaRef].(string); isPersonaRef(ref) {
		return strings.TrimSpace(ref)
	}
	if tags, ok := task[objects.FieldKeyTags].([]any); ok {
		for _, raw := range tags {
			tag, _ := raw.(string)
			if !strings.HasPrefix(tag, personaRefTagPrefix) {
				continue
			}
			if ref := strings.TrimSpace(strings.TrimPrefix(tag, personaRefTagPrefix)); isPersonaRef(ref) {
				return ref
			}
		}
	}
	return objects.ConstPersonaDefaultAgent
}

func isPersonaRef(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "PER-")
}
