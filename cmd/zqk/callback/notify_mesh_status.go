package callback

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Scheduler callback feed identity — matches IsAckTheatreInboxItem
// (ack-theatre / non-injecting status) and historical scheduler-callback wakes.
const (
	callbackSchedulerFeedAgentID = "scheduler-callback"
	callbackMeshMessageMaxLen    = 240
)

// summarizeCallbackMeshMessage builds the undirected mesh_status body for a
// scheduler job callback. Same shape as the retired studio Python helper.
func summarizeCallbackMeshMessage(payload map[string]any) string {
	jobID := firstNonEmptyString(payload, "job_id", "id")
	if jobID == "" {
		jobID = "unknown"
	}
	outcome := firstNonEmptyString(payload, "outcome", objects.FieldKeyCallbackType, objects.FieldKeyStatus)
	if outcome == "" {
		outcome = "done"
	}
	msg := fmt.Sprintf("scheduler callback %s %s", jobID, outcome)
	if errVal := payload["error"]; errVal != nil {
		if s := strings.TrimSpace(fmt.Sprint(errVal)); s != "" && s != "<nil>" {
			msg += " error=" + s
		}
	}
	if len(msg) > callbackMeshMessageMaxLen {
		return msg[:callbackMeshMessageMaxLen]
	}
	return msg
}

func firstNonEmptyString(payload map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := payload[k]; ok {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

// stampSchedulerCallbackMeshStatus appends an undirected mesh_status event so
// seats observe job completion without flooding inbox_unacked (wake broadcast).
func stampSchedulerCallbackMeshStatus(projectRoot string, payload map[string]any) error {
	msg := summarizeCallbackMeshMessage(payload)
	_, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: projectRoot,
		Message:     msg,
		AgentID:     callbackSchedulerFeedAgentID,
		Sender:      agentfeed.FeedSenderMeshStatus,
		EventType:   agentfeed.FeedEventTypeMeshStatus,
		SelfACK:     false,
	})
	return err
}
