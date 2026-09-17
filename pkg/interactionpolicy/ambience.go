package interactionpolicy

import (
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// StaleInProgressItem records an in-progress work item that has stalled.
type StaleInProgressItem struct {
	BacklogItemID string        `json:"backlog_item_id,omitempty"`
	TaskID        string        `json:"task_id,omitempty"`
	TestCaseID    string        `json:"test_case_id,omitempty"`
	Reason        string        `json:"reason"`
	StaleDuration time.Duration `json:"stale_duration,omitempty"`
}

// StaleTestCatalystHint returns single-click test execution command for the test case.
func StaleTestCatalystHint(testCaseID string) string {
	testCaseID = strings.TrimSpace(testCaseID)
	if testCaseID == "" {
		return ""
	}
	return "zqk test run " + testCaseID
}

// DetectStaleInProgress identifies in-progress backlog items that have had no progress beyond staleThreshold.
func DetectStaleInProgress(blis []map[string]any, testCases []map[string]any, now time.Time, staleThreshold time.Duration) []StaleInProgressItem {
	var results []StaleInProgressItem
	for _, bli := range blis {
		status, _ := bli[objects.FieldKeyStatus].(string)
		if status != objects.ObjectStatusInProgress {
			continue
		}
		bliID, _ := bli[objects.FieldKeyID].(string)
		updatedAtStr, _ := bli[objects.FieldKeyUpdatedAt].(string)
		if updatedAtStr == "" {
			updatedAtStr, _ = bli[objects.FieldKeyCreatedAt].(string)
		}
		stale := false
		var duration time.Duration
		if updatedAtStr != "" {
			if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
				duration = now.Sub(t)
				if duration >= staleThreshold {
					stale = true
				}
			}
		} else {
			stale = true
		}

		if stale {
			tcID := findMatchingTestCaseForBLI(bli, testCases)
			results = append(results, StaleInProgressItem{
				BacklogItemID: bliID,
				TestCaseID:    tcID,
				Reason:        "in_progress with no recent activity",
				StaleDuration: duration,
			})
		}
	}
	return results
}

// DetectStaleAgentTasks identifies in-progress agent_tasks that have had no progress beyond staleThreshold.
func DetectStaleAgentTasks(tasks []map[string]any, now time.Time, staleThreshold time.Duration) []StaleInProgressItem {
	var results []StaleInProgressItem
	for _, task := range tasks {
		status, _ := task[objects.FieldKeyStatus].(string)
		if status != objects.ObjectStatusInProgress {
			continue
		}
		taskID, _ := task[objects.FieldKeyID].(string)
		bliID, _ := task[objects.FieldKeyBacklogItemRef].(string)
		updatedAtStr, _ := task[objects.FieldKeyUpdatedAt].(string)
		if updatedAtStr == "" {
			updatedAtStr, _ = task[objects.FieldKeyClaimedAt].(string)
		}
		if updatedAtStr == "" {
			updatedAtStr, _ = task[objects.FieldKeyCreatedAt].(string)
		}
		stale := false
		var duration time.Duration
		if updatedAtStr != "" {
			if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
				duration = now.Sub(t)
				if duration >= staleThreshold {
					stale = true
				}
			}
		} else {
			stale = true
		}

		if stale {
			results = append(results, StaleInProgressItem{
				BacklogItemID: bliID,
				TaskID:        taskID,
				Reason:        "agent_task in_progress with no recent activity",
				StaleDuration: duration,
			})
		}
	}
	return results
}

func findMatchingTestCaseForBLI(bli map[string]any, testCases []map[string]any) string {
	bliID, _ := bli[objects.FieldKeyID].(string)
	var bliCrits []string
	if raw, ok := bli[objects.FieldKeyCriteriaRefs]; ok {
		bliCrits = stringSliceFromAny(raw)
	}

	for _, tc := range testCases {
		tcID, _ := tc[objects.FieldKeyID].(string)
		if tcID == "" {
			continue
		}
		if raw, ok := tc[objects.FieldKeyBacklogItemRefs]; ok {
			for _, b := range stringSliceFromAny(raw) {
				if b == bliID {
					return tcID
				}
			}
		}
		if raw, ok := tc[objects.FieldKeyCriteriaRefs]; ok {
			tcCrits := stringSliceFromAny(raw)
			for _, tcCrit := range tcCrits {
				for _, bCrit := range bliCrits {
					if tcCrit == bCrit {
						return tcID
					}
				}
			}
		}
	}
	return ""
}

func stringSliceFromAny(val any) []string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		var res []string
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				res = append(res, s)
			}
		}
		return res
	}
	return nil
}

// Ambience is compiled micro-signals from whats-next (not a shell command).
// TRACK: BLI-1787035087372193000-c022117d
type Ambience struct {
	Planned        int
	InProgress     int
	Exploring      int
	Validated      int
	PlanStatus     string
	InboxUnacked   int
	OutboxAwaiting int
	// BranchAhead is local commits not on the tracked upstream (0 if unknown).
	BranchAhead     int
	StaleInProgress []StaleInProgressItem
}

func planExecutionLocked(s Ambience) bool {
	if s.InProgress > 0 {
		return true
	}
	switch s.PlanStatus {
	case objects.ObjectStatusInProgress, objects.ObjectStatusComplete:
		return true
	default:
		return false
	}
}

// ClassifyAmbience picks one event from compiled signals.
// Preemption (POL-AGENT-INTERACTION-POLICY-001): inbox > push-ahead >
// planned execution (idle) > kernel fill (compiled onto idle, not a
// separate event) > groom-ahead > align. Never silence.
// TRACK: BLI-1787035087372193000-c022117d
//
// Inbox unacked is the TPM swarm gland: MCP notify does not start a Cursor
// turn, so hunger must compile a followup_message or the seat goes idle while
// peers wait on hourglass.
//
// Push-ahead is landing duty already written on PROCESS-ADMIN (push the seated
// plan trunk). It is a classifier rung, not a new subsystem.
//
// planned==0 is not automatically "groom this priority_plan." An execution-locked
// lead (in_progress items or plan status) is stratplan_ahead: forward planning
// and alignment, never restuff/seal-break. shovel_ready_empty is only unsealed
// intake (exploring/validated present, no in-flight work). Align is the last
// rung of that event (CompileStratplanCommandHint), not a separate gland.
//
// Outbox awaiting peer_ack with planned>0 is keep-working idle, not a hole.
func ClassifyAmbience(s Ambience) string {
	if s.InboxUnacked > 0 {
		return EventInboxUnacked
	}
	if s.BranchAhead > 0 {
		return EventPushAhead
	}
	if s.Planned > 0 {
		return EventIdle
	}
	if planExecutionLocked(s) || s.Exploring+s.Validated == 0 {
		return EventStratplanAhead
	}
	return EventShovelReadyEmpty
}
