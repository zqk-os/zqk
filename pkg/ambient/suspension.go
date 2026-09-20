package ambient

import (
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// SuspendContext packages an agent's state into a .csnap file for later resumption.
// Returns the file path of the saved snapshot.
func SuspendContext(projectRoot, agentID string, state map[string]any, logger logging.Logger) (string, error) {
	if projectRoot == "" {
		return "", errfmt.Errorf("project root is required")
	}
	if agentID == "" {
		return "", errfmt.Errorf("agent ID is required")
	}
	if state == nil {
		return "", errfmt.Errorf("state map is required")
	}

	timestamp := time.Now()
	cs, err := storage.CreateCompressedSnapshot([]map[string]any{state}, timestamp, logger)
	if err != nil {
		logging.Fluent(logger).Error("Failed to create compressed snapshot", err).
			String("agent_id", agentID).
			Log()
		return "", errfmt.Errorf("failed to create compressed snapshot: %w", err)
	}

	filename := agentID + storage.CompressedSnapshotFileExtension
	filePath := filepath.Join(projectRoot, ".zqk-state", "suspended_agents", filename)

	if err := storage.WriteCompressedSnapshot(cs, filePath); err != nil {
		logging.Fluent(logger).Error("Failed to write compressed snapshot", err).
			String("agent_id", agentID).
			String("file_path", filePath).
			Log()
		return "", errfmt.Errorf("failed to write compressed snapshot to %s: %w", filePath, err)
	}

	logging.Fluent(logger).Info("Agent context suspended").
		String("agent_id", agentID).
		String("file_path", filePath).
		Log()

	return filePath, nil
}

// WakeContext re-hydrates an agent's state from a .csnap file.
func WakeContext(projectRoot, agentID string, logger logging.Logger) (map[string]any, error) {
	if projectRoot == "" {
		return nil, errfmt.Errorf("project root is required")
	}
	if agentID == "" {
		return nil, errfmt.Errorf("agent ID is required")
	}

	filename := agentID + storage.CompressedSnapshotFileExtension
	filePath := filepath.Join(projectRoot, ".zqk-state", "suspended_agents", filename)

	cs, err := storage.ReadCompressedSnapshot(filePath)
	if err != nil {
		logging.Fluent(logger).Error("Failed to read compressed snapshot", err).
			String("agent_id", agentID).
			String("file_path", filePath).
			Log()
		return nil, errfmt.Errorf("failed to read compressed snapshot from %s: %w", filePath, err)
	}

	objects, err := cs.Expand()
	if err != nil {
		logging.Fluent(logger).Error("Failed to expand compressed snapshot", err).
			String("agent_id", agentID).
			String("file_path", filePath).
			Log()
		return nil, errfmt.Errorf("failed to expand compressed snapshot: %w", err)
	}

	if len(objects) == 0 {
		err := errfmt.Errorf("no objects found in compressed snapshot")
		logging.Fluent(logger).Error("Snapshot empty", err).
			String("agent_id", agentID).
			String("file_path", filePath).
			Log()
		return nil, err
	}

	logging.Fluent(logger).Info("Agent context awoken").
		String("agent_id", agentID).
		String("file_path", filePath).
		Log()

	return objects[0], nil
}
