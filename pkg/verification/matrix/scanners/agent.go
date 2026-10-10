package scanners

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// AgentRubricEvaluator performs a blind LLM or adversarial evaluation against a specified rubric.
type AgentRubricEvaluator interface {
	EvaluateRubric(ctx context.Context, repoRoot string, entry *matrix.FileEntry, rubric string) (matrix.DiamondScore, string, []matrix.Finding, error)
}

// AgentEvaluatorFunc allows using a function as an AgentRubricEvaluator.
type AgentEvaluatorFunc func(ctx context.Context, repoRoot string, entry *matrix.FileEntry, rubric string) (matrix.DiamondScore, string, []matrix.Finding, error)

// EvaluateRubric invokes the function.
func (f AgentEvaluatorFunc) EvaluateRubric(ctx context.Context, repoRoot string, entry *matrix.FileEntry, rubric string) (matrix.DiamondScore, string, []matrix.Finding, error) {
	return f(ctx, repoRoot, entry, rubric)
}

// AgentScanner executes a rubric-driven adversarial or LLM agent check returning a 1-5 diamond score.
type AgentScanner struct {
	checkID       string
	rubric        string
	evaluator     AgentRubricEvaluator
	targetClasses []matrix.FileClass
}

// NewAgentScanner creates a new agent-based rubric scanner.
func NewAgentScanner(checkID string, rubric string, evaluator AgentRubricEvaluator, targetClasses []matrix.FileClass) *AgentScanner {
	if checkID == "" {
		checkID = "agent-rubric"
	}
	return &AgentScanner{
		checkID:       checkID,
		rubric:        rubric,
		evaluator:     evaluator,
		targetClasses: targetClasses,
	}
}

// Run executes the blind rubric evaluation for the target file entry.
func (s *AgentScanner) Run(ctx context.Context, repoRoot string, entry *matrix.FileEntry) (matrix.CheckResult, error) {
	evaluatedAt := time.Now().UTC()

	// Check target classes filter if configured
	if len(s.targetClasses) > 0 {
		matchedClass := false
		for _, tc := range s.targetClasses {
			if tc == entry.Class {
				matchedClass = true
				break
			}
		}
		if !matchedClass {
			return matrix.CheckResult{
				CheckID:     s.checkID,
				Status:      matrix.CheckStatusSkipped,
				Evaluator:   "scanner:agent:" + s.checkID,
				EvaluatedAt: evaluatedAt,
				Feedback:    fmt.Sprintf("Class %s not in target classes", entry.Class),
			}, nil
		}
	}

	if s.evaluator == nil {
		return matrix.CheckResult{
			CheckID:      s.checkID,
			Status:       matrix.CheckStatusPending,
			DiamondScore: matrix.ScoreUnrated,
			Evaluator:    "unassigned",
			EvaluatedAt:  evaluatedAt,
			Feedback:     "Awaiting blind agent evaluation pass against rubric",
		}, nil
	}

	score, feedback, findings, err := s.evaluator.EvaluateRubric(ctx, repoRoot, entry, s.rubric)
	if err != nil {
		return matrix.CheckResult{
			CheckID:      s.checkID,
			Status:       matrix.CheckStatusFailed,
			DiamondScore: matrix.ScoreCritical,
			Evaluator:    "scanner:agent:" + s.checkID,
			EvaluatedAt:  evaluatedAt,
			Feedback:     fmt.Sprintf("Agent rubric evaluation failed: %v", err),
		}, err
	}

	status := matrix.CheckStatusPassed
	if score < matrix.ScoreMinor {
		status = matrix.CheckStatusFailed
	}

	return matrix.CheckResult{
		CheckID:      s.checkID,
		Status:       status,
		DiamondScore: score,
		Evaluator:    "scanner:agent:" + s.checkID,
		EvaluatedAt:  evaluatedAt,
		Feedback:     feedback,
		Findings:     findings,
	}, nil
}
