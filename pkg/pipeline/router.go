package pipeline

import (
	"context"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// PlanDataSource provides data for the router.
type PlanDataSource interface {
	GetBacklogItemsForPlan(ctx context.Context, planID string) ([]string, error)
	GroupBacklogItems(ctx context.Context, backlogItems []string) ([][]string, error)
	CreateAgentTask(ctx context.Context, planID string, backlogItems []string) (string, error)
	AssignPersonaToTask(ctx context.Context, taskID string) (string, error)
	SetTaskStatus(ctx context.Context, taskID string, status string) error
	WakeAgent(ctx context.Context, taskID string, persona string) error
	ScheduleHourglassFlip(ctx context.Context, taskID string) error
}

// Router defines a pipeline router that splits priority plans into workstreams.
type Router struct {
	pipeline *Pipeline
	ds       PlanDataSource
}

// RouterPayload represents the input for the router.
type RouterPayload struct {
	PriorityPlanID string
}

// RouterResult represents the output of the router.
type RouterResult struct {
	AgentTaskIDs []string
	Assignments  map[string]string // maps AgentTaskID to Persona
}

// NewRouter creates a new multi-agent pipeline router.
func NewRouter(logger logging.Logger, ds PlanDataSource) *Router {
	b := NewInstrumentedBuilder("multi_agent_router", logger)

	b.AddStage("breakdown", func(ctx *Context, payload any) (any, error) {
		p, ok := payload.(*RouterPayload)
		if !ok {
			return nil, fmt.Errorf("invalid payload type")
		}

		items, err := ds.GetBacklogItemsForPlan(ctx.Ctx, p.PriorityPlanID)
		if err != nil {
			return nil, fmt.Errorf("failed to get backlog items: %w", err)
		}

		groups, err := ds.GroupBacklogItems(ctx.Ctx, items)
		if err != nil {
			return nil, fmt.Errorf("failed to group backlog items: %w", err)
		}

		var taskIDs []string
		for _, group := range groups {
			taskID, err := ds.CreateAgentTask(ctx.Ctx, p.PriorityPlanID, group)
			if err != nil {
				return nil, fmt.Errorf("failed to create agent task: %w", err)
			}
			taskIDs = append(taskIDs, taskID)
		}

		return &RouterResult{
			AgentTaskIDs: taskIDs,
			Assignments:  make(map[string]string),
		}, nil
	})

	b.AddStage("assign_personas", func(ctx *Context, payload any) (any, error) {
		res, ok := payload.(*RouterResult)
		if !ok {
			return nil, fmt.Errorf("invalid payload type")
		}

		for _, taskID := range res.AgentTaskIDs {
			persona, err := ds.AssignPersonaToTask(ctx.Ctx, taskID)
			if err != nil {
				return nil, fmt.Errorf("failed to assign persona to task %s: %w", taskID, err)
			}
			res.Assignments[taskID] = persona
		}
		return res, nil
	})

	b.AddStage("queueing", func(ctx *Context, payload any) (any, error) {
		res, ok := payload.(*RouterResult)
		if !ok {
			return nil, fmt.Errorf("invalid payload type")
		}

		for _, taskID := range res.AgentTaskIDs {
			err := ds.SetTaskStatus(ctx.Ctx, taskID, objects.ObjectStatusPending)
			if err != nil {
				return nil, fmt.Errorf("failed to set task status to pending %s: %w", taskID, err)
			}
		}
		return res, nil
	})

	b.AddStage("wake_agents", func(ctx *Context, payload any) (any, error) {
		res, ok := payload.(*RouterResult)
		if !ok {
			return nil, fmt.Errorf("invalid payload type")
		}

		for _, taskID := range res.AgentTaskIDs {
			persona := res.Assignments[taskID]
			err := ds.WakeAgent(ctx.Ctx, taskID, persona)
			if err != nil {
				return nil, fmt.Errorf("failed to wake agent for task %s: %w", taskID, err)
			}
			err = ds.ScheduleHourglassFlip(ctx.Ctx, taskID)
			if err != nil {
				return nil, fmt.Errorf("failed to schedule hourglass flip for task %s: %w", taskID, err)
			}
		}
		return res, nil
	})

	return &Router{
		pipeline: b.Build(),
		ds:       ds,
	}
}

// Route executes the routing logic.
func (r *Router) Route(ctx context.Context, planID string) (*RouterResult, error) {
	pctx := &Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	if ctx != nil {
		pctx.Ctx = ctx
	}
	payload := &RouterPayload{PriorityPlanID: planID}

	res, err := r.pipeline.Run(pctx, payload)
	if err != nil {
		return nil, err
	}

	return res.(*RouterResult), nil
}
