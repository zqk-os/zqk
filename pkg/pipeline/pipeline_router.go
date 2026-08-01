package pipeline

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// PipelineRouter handles routing logic for priority plans.
type PipelineRouter struct{}

// NewPipelineRouter creates a new PipelineRouter.
func NewPipelineRouter() *PipelineRouter {
	return &PipelineRouter{}
}

// RouteResult holds the output of the routing logic.
type RouteResult struct {
	WorkstreamIDs   []string
	TaskAssignments map[string]string // TaskID -> Persona
}

// Route parses a priority plan, extracts workstreams, and assigns tasks to personas based on vocabulary_scheme tags.
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

			persona := "persona:default"
			tagsRaw, ok := task[objects.FieldKeyTags]
			if ok {
				if tagsAny, ok := tagsRaw.([]any); ok {
					for _, tagAny := range tagsAny {
						if tagStr, ok := tagAny.(string); ok && strings.HasPrefix(tagStr, "vocabulary_scheme:") {
							scheme := strings.TrimPrefix(tagStr, "vocabulary_scheme:")
							persona = fmt.Sprintf("persona:%s", scheme)
							break
						}
					}
				}
			}

			result.TaskAssignments[taskID] = persona
		}
	}

	return result, nil
}
