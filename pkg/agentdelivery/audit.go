package agentdelivery

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	// AgentPromptDeliveriesJSONLFile is one JSON object per agent-prompt output delivery (audit / traceability).
	AgentPromptDeliveriesJSONLFile = "agent_prompt_deliveries.jsonl"
	// EventTypeAgentPromptDelivery is the event_type field for delivery audit lines.
	EventTypeAgentPromptDelivery = "agent_prompt_delivery"

	auditJSONLFileLockTimeout = 5 * time.Second
)

// DeliveryAuditRecord is a single JSON line in agent_prompt_deliveries.jsonl (CRIT-AO-002 / REQ-AO-001).
type DeliveryAuditRecord struct {
	Timestamp            string   `json:"timestamp"`
	EventType            string   `json:"event_type"`
	ConvergenceSessionID string   `json:"convergence_session_id"`
	Format               string   `json:"format"`
	DeliverMode          string   `json:"deliver_mode"`
	AttentionMode        string   `json:"attention_mode,omitempty"`
	OutputPath           string   `json:"output_path,omitempty"`
	PrimaryDestination   string   `json:"primary_destination"`
	DeliveredTo          []string `json:"delivered_to"`
	HTTPURL              string   `json:"http_url,omitempty"`
	HTTPStatusCode       int      `json:"http_status,omitempty"`
	HTTPError            string   `json:"http_error,omitempty"`
	MarkdownBytes        int      `json:"markdown_bytes"`
}

// AgentPromptDeliveriesJSONLPath returns the path to agent_prompt_deliveries.jsonl under the project.
func AgentPromptDeliveriesJSONLPath(projectRoot string) string {
	if projectRoot == "" {
		projectRoot = "."
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, AgentPromptDeliveriesJSONLFile)
}

// AppendDeliveryAuditJSONL appends one JSON line for a delivery audit record. Best-effort; errors are ignored.
// Uses the same flock pattern as scheduler agent_prompt_runs.jsonl writers.
func AppendDeliveryAuditJSONL(projectRoot string, rec *DeliveryAuditRecord) {
	if rec == nil {
		return
	}
	rec2 := *rec
	rec2.Timestamp = zqktime.NowRFC3339UTC()
	rec2.EventType = EventTypeAgentPromptDelivery

	data, err := json.Marshal(rec2)
	if err != nil {
		return
	}
	line := append(data, '\n')

	path := AgentPromptDeliveriesJSONLPath(projectRoot)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return
	}
	lock, err := storagepkg.NewFileLock(path + paths.LockFileSuffix)
	if err != nil {
		return
	}
	defer func() { _ = lock.Close() }()
	_ = lock.WithLockTimeout(auditJSONLFileLockTimeout, func() error {
		f, err := fileutil.OpenFile(path, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm644)
		if err != nil {
			return nil
		}
		defer f.Close()
		_, _ = f.Write(line)
		return nil
	})
}
