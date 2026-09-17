package orchestration

import (
	"context"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/storage"
)

// AgentID represents a unique identifier for an agent.
type AgentID string

// ConvergenceSession defines the lifecycle of a multi-agent convergence session.
type ConvergenceSession interface {
	// AddAgent adds an agent to the session.
	AddAgent(ctx context.Context, agentID AgentID) error
	// Start begins the convergence process among agents.
	Start(ctx context.Context, topic string, options map[string]any) error
	// Status returns the current status of the convergence session.
	Status() string
}

// Session struct implements ConvergenceSession using the pipeline package.
type Session struct {
	logger  logging.Logger
	agents  []AgentID
	status  string
	pipe    *pipeline.Pipeline
	storage storage.ObjectStorageProvider
}

// NewSession initializes a new multi-agent convergence session.
func NewSession(logger logging.Logger, store storage.ObjectStorageProvider) *Session {
	b := pipeline.NewBuilder("multi_agent_convergence", logger)

	b.AddStage("INGEST", func(ctx *pipeline.Context, payload any) (any, error) {
		// Ingest topic and agent info
		return payload, nil
	})
	b.AddStage("DECIDE", func(ctx *pipeline.Context, payload any) (any, error) {
		// Multi-agent policy negotiation
		return payload, nil
	})
	b.AddStage("COMMIT", func(ctx *pipeline.Context, payload any) (any, error) {
		if store != nil {
			secCtx := pkgctx.NewSystemSecurityContext()
			policyObj, err := store.Read(ctx.Ctx, secCtx, "POL-GATE-001")
			if err != nil {
				return nil, fmt.Errorf("failed to fetch Truth Sentinel policy: %w", err)
			}

			pMap, ok := payload.(map[string]any)
			if ok {
				if mergeReq, isMerge := pMap["local_merge_to_main"].(bool); isMerge && mergeReq {
					prLink, _ := pMap["pr_link"].(string)
					if prLink == "" {
						if fl, ok := logging.TryFluentEvent(logger); ok {
							fl.Warn("Truth Sentinel: Code Movement Gate policy violation").String("policy", fmt.Sprintf("%v", policyObj[objects.FieldKeyID])).Log()
						}
						return nil, fmt.Errorf("policy violation [%v]: local merge to main attempted without a valid PR link", policyObj[objects.FieldKeyID])
					}
				}
			}
		}
		// Commit decisions
		return payload, nil
	})

	return &Session{
		logger:  logger,
		agents:  make([]AgentID, 0),
		status:  objects.ObjectStatusPending,
		pipe:    b.Build(),
		storage: store,
	}
}

func (s *Session) AddAgent(ctx context.Context, agentID AgentID) error {
	s.agents = append(s.agents, agentID)
	return nil
}

func (s *Session) Start(ctx context.Context, topic string, options map[string]any) error {
	s.status = objects.ObjectStatusRunning

	pctx := &pipeline.Context{
		Ctx: ctx,
	}

	payload := map[string]any{
		"topic":  topic,
		"agents": s.agents,
	}

	for k, v := range options {
		payload[k] = v
	}

	_, err := s.pipe.Run(pctx, payload)
	if err != nil {
		s.status = objects.ObjectStatusFailed
		return err
	}

	s.status = objects.ObjectStatusCompleted
	return nil
}

func (s *Session) Status() string {
	return s.status
}
