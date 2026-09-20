package scheduler

import (
	"context"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/zqk-os/zqk/pkg/ingestion/adapters"
	"github.com/zqk-os/zqk/pkg/logging"
)

type AgentSyncHandler struct {
	logger      logging.Logger
	projectRoot string
}

func NewAgentSyncHandler(projectRoot string, logger logging.Logger) *AgentSyncHandler {
	return &AgentSyncHandler{
		logger:      logger,
		projectRoot: projectRoot,
	}
}

func (h *AgentSyncHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	logging.Fluent(h.logger).Info("Starting agent sync background job").Log()

	dir := filepath.Join(h.projectRoot, paths.ProjectDataDir, "agents")

	adapter := adapters.NewGeminiAdapter(h.logger)
	if err := adapter.Ingest(ctx, dir); err != nil {
		logging.Fluent(h.logger).Error("Agent sync background job failed", err).Log()
		return err
	}

	logging.Fluent(h.logger).Info("Agent sync background job completed").Log()
	return nil
}
