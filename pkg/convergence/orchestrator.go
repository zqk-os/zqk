package convergence

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

type RemediationReceipt struct {
	ReceiptID    string        `json:"receipt_id"`
	RuleID       string        `json:"rule_id"`
	Action       ActionType    `json:"action"`
	SessionID    string        `json:"session_id"`
	JobID        string        `json:"job_id"`
	Attempt      int           `json:"attempt"`
	BackoffDelay time.Duration `json:"backoff_delay"`
	DispatchedAt time.Time     `json:"dispatched_at"`
	Executed     bool          `json:"executed"`
}

type RemediationOrchestrator struct {
	evaluator *PolicyEvaluator
	mu        sync.Mutex
	attempts  map[string]int // key: session_id:job_id
	receipts  []*RemediationReceipt
}

func NewRemediationOrchestrator(evaluator *PolicyEvaluator) *RemediationOrchestrator {
	if evaluator == nil {
		evaluator = NewPolicyEvaluator()
	}
	return &RemediationOrchestrator{
		evaluator: evaluator,
		attempts:  make(map[string]int),
	}
}

func (ro *RemediationOrchestrator) Evaluator() *PolicyEvaluator {
	return ro.evaluator
}

func (ro *RemediationOrchestrator) DispatchRemediation(ctx context.Context, payload *TelemetryPayload) (*RemediationReceipt, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rule, err := ro.evaluator.Evaluate(payload)
	if err != nil {
		return nil, err
	}

	key := fmt.Sprintf("%s:%s", payload.SessionID, payload.JobID)

	ro.mu.Lock()
	defer ro.mu.Unlock()

	currentAttempts := ro.attempts[key]
	if currentAttempts >= rule.MaxRetries {
		return nil, fmt.Errorf("%w: job %s has reached max retries (%d/%d)",
			ErrCyclicRemediationLoop, payload.JobID, currentAttempts, rule.MaxRetries)
	}

	attemptNum := currentAttempts + 1
	ro.attempts[key] = attemptNum

	// Exponential backoff: Base * 2^(attempt - 1)
	factor := math.Pow(2, float64(attemptNum-1))
	delayMs := float64(rule.BackoffBaseMs) * factor
	if delayMs > 120000 { // Max clamp: 2 minutes
		delayMs = 120000
	}
	backoff := time.Duration(delayMs) * time.Millisecond

	receipt := &RemediationReceipt{
		ReceiptID:    fmt.Sprintf("REM-%s-%d-%d", payload.JobID, attemptNum, time.Now().UnixNano()),
		RuleID:       rule.ID,
		Action:       rule.Action,
		SessionID:    payload.SessionID,
		JobID:        payload.JobID,
		Attempt:      attemptNum,
		BackoffDelay: backoff,
		DispatchedAt: time.Now().UTC(),
		Executed:     true,
	}

	ro.receipts = append(ro.receipts, receipt)
	return receipt, nil
}

func (ro *RemediationOrchestrator) GetAttemptCount(sessionID, jobID string) int {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	return ro.attempts[fmt.Sprintf("%s:%s", sessionID, jobID)]
}

func (ro *RemediationOrchestrator) ResetSession(sessionID string) {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	prefix := sessionID + ":"
	for k := range ro.attempts {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(ro.attempts, k)
		}
	}
}

func (ro *RemediationOrchestrator) Receipts() []*RemediationReceipt {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	cp := make([]*RemediationReceipt, len(ro.receipts))
	copy(cp, ro.receipts)
	return cp
}
