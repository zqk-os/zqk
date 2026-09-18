package monitors

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/healthcheck"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// autonomyInboxMonitor checks live Autonomy Inbox status by verifying inbox directories
// and active instruction queues.
type autonomyInboxMonitor struct{}

func (m *autonomyInboxMonitor) ID() string   { return "autonomy_inbox" }
func (m *autonomyInboxMonitor) Name() string { return "Autonomy Inbox live health" }

func (m *autonomyInboxMonitor) Run(ctx context.Context, projectRoot string) (*healthcheck.Result, error) {
	if projectRoot == emptyValue {
		return &healthcheck.Result{Status: statusOK, Summary: summaryNoProjectRoot}, nil
	}

	inboxDir := filepath.Join(projectRoot, paths.ProjectDataDir, "inbox")
	var pendingCount int
	if info, err := fileutil.Stat(inboxDir); err == nil && info.IsDir() {
		entries, _ := fileutil.ReadDir(inboxDir)
		for _, e := range entries {
			if !e.IsDir() {
				pendingCount++
			}
		}
	}

	status := statusOK
	if pendingCount > 500 {
		status = statusDegraded
	}

	summary := fmt.Sprintf("live autonomy inbox: %d pending instructions", pendingCount)
	details := map[string]any{
		"pending_count": pendingCount,
		"synthetic":     false,
		"available":     true,
	}

	return &healthcheck.Result{Status: status, Summary: summary, Details: details}, nil
}

func init() {
	healthcheck.DefaultRegistry.Register(&autonomyInboxMonitor{})
}
