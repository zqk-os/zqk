package workflow

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/nildecode"
)

// workflow_next_policy holds the decision table for `workflow next`: stable decision IDs,
// human rationales, and how to build the recommended shell command. Precedence order stays
// in applyDecision in next.go; add a row here when you add a new decision branch.

const (
	decisionCriticalPolicyInterrupt = "critical_policy_interrupt"
	decisionActiveConvergence       = "active_convergence_session"
	decisionPriorityPlanBacklog     = "priority_plan_backlog"
	decisionNone                    = "none"

	rationaleNoAction            = "No actionable policy interrupt, active convergence session, or backlog item found."
	rationaleCritical            = "Critical unacknowledged policy interrupt is highest precedence and blocks normal execution."
	rationaleConverge            = "Active convergence session takes precedence over backlog routing."
	rationalePriorityPlanBacklog = "No policy interrupt or active convergence session; selected highest-priority non-terminal backlog item."

	cmdObjectListInProgressBacklog = "zqk object list backlog_item --filter status=in_progress --format table"
	cmdAckPolicyTemplate           = "zqk system policy-interrupts ack --dedupe-key %s"
	cmdObjectGetTemplate           = "zqk object get %s"
)

type decisionSpec struct {
	Rationale      string
	CommandBuilder func(string) string
}

var decisionSpecs = map[string]decisionSpec{
	decisionNone: {
		Rationale: rationaleNoAction,
		CommandBuilder: func(string) string {
			return cmdObjectListInProgressBacklog
		},
	},
	decisionCriticalPolicyInterrupt: {
		Rationale: rationaleCritical,
		CommandBuilder: func(id string) string {
			return fmt.Sprintf(cmdAckPolicyTemplate, id)
		},
	},
	decisionActiveConvergence: {
		Rationale: rationaleConverge,
		CommandBuilder: func(id string) string {
			return fmt.Sprintf(cmdObjectGetTemplate, id)
		},
	},
	decisionPriorityPlanBacklog: {
		Rationale: rationalePriorityPlanBacklog,
		CommandBuilder: func(id string) string {
			return fmt.Sprintf(cmdObjectGetTemplate, id)
		},
	},
}

func applyDecisionSpec(result *workflowNextResult, decision, id string) {
	spec, ok := decisionSpecs[decision]
	if !ok {
		return
	}
	if _, ok := nildecode.DecodeNonNilPayload[*workflowNextResult](result); !ok {
		return
	}
	result.Decision = decision
	result.Rationale = spec.Rationale
	result.RecommendedCommand = spec.CommandBuilder(id)
}
