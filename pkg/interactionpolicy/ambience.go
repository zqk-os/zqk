package interactionpolicy

import (
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
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
	return paths.CLIUsage("test", "run", testCaseID)
}

// DetectStaleInProgressTyped identifies in-progress backlog items that have had no progress beyond staleThreshold using typed summaries.
func DetectStaleInProgressTyped(blis []BacklogItemSummary, testCases []TestCaseSummary, now time.Time, staleThreshold time.Duration) []StaleInProgressItem {
	var results []StaleInProgressItem
	for _, bli := range blis {
		if bli.Status != objects.ObjectStatusInProgress {
			continue
		}
		updatedAtStr := bli.UpdatedAt
		if updatedAtStr == "" {
			updatedAtStr = bli.CreatedAt
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
			tcID := findMatchingTestCaseForBLITyped(bli, testCases)
			results = append(results, StaleInProgressItem{
				BacklogItemID: bli.ID,
				TestCaseID:    tcID,
				Reason:        "in_progress with no recent activity",
				StaleDuration: duration,
			})
		}
	}
	return results
}

// DetectStaleInProgress identifies in-progress backlog items that have had no progress beyond staleThreshold.
// It converts untyped maps into typed domain summaries at the boundary.
func DetectStaleInProgress(blis []map[string]any, testCases []map[string]any, now time.Time, staleThreshold time.Duration) []StaleInProgressItem {
	typedBLIs := BacklogItemSummariesFromMaps(blis)
	typedTestCases := TestCaseSummariesFromMaps(testCases)
	return DetectStaleInProgressTyped(typedBLIs, typedTestCases, now, staleThreshold)
}

// DetectStaleAgentTasksTyped identifies in-progress agent_tasks that have had no progress beyond staleThreshold using typed summaries.
func DetectStaleAgentTasksTyped(tasks []AgentTaskSummary, now time.Time, staleThreshold time.Duration) []StaleInProgressItem {
	var results []StaleInProgressItem
	for _, task := range tasks {
		if task.Status != objects.ObjectStatusInProgress {
			continue
		}
		updatedAtStr := task.UpdatedAt
		if updatedAtStr == "" {
			updatedAtStr = task.ClaimedAt
		}
		if updatedAtStr == "" {
			updatedAtStr = task.CreatedAt
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
				BacklogItemID: task.BacklogItemRef,
				TaskID:        task.ID,
				Reason:        "agent_task in_progress with no recent activity",
				StaleDuration: duration,
			})
		}
	}
	return results
}

// DetectStaleAgentTasks identifies in-progress agent_tasks that have had no progress beyond staleThreshold.
// It converts untyped maps into typed domain summaries at the boundary.
func DetectStaleAgentTasks(tasks []map[string]any, now time.Time, staleThreshold time.Duration) []StaleInProgressItem {
	typedTasks := AgentTaskSummariesFromMaps(tasks)
	return DetectStaleAgentTasksTyped(typedTasks, now, staleThreshold)
}

func findMatchingTestCaseForBLITyped(bli BacklogItemSummary, testCases []TestCaseSummary) string {
	bliID := bli.ID
	bliCrits := bli.CriteriaRefs

	for _, tc := range testCases {
		if tc.ID == "" {
			continue
		}
		for _, b := range tc.BacklogItemRefs {
			if b == bliID {
				return tc.ID
			}
		}
		for _, tcCrit := range tc.CriteriaRefs {
			for _, bCrit := range bliCrits {
				if tcCrit == bCrit {
					return tc.ID
				}
			}
		}
	}
	return ""
}

func findMatchingTestCaseForBLI(bli map[string]any, testCases []map[string]any) string {
	return findMatchingTestCaseForBLITyped(BacklogItemSummaryFromMap(bli), TestCaseSummariesFromMaps(testCases))
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
// TRACK: follow-up in kernel backlog
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
// TRACK: follow-up in kernel backlog
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
