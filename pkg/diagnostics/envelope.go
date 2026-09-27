package diagnostics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Standard failure error categories (project-agnostic).
const (
	CategorySuccess             = "success"
	CategoryTimeout             = "timeout"
	CategoryExecutionFailure    = "execution_failure"
	CategoryConstraintViolation = "constraint_violation"
	CategoryContextCancelled    = "context_cancelled"
	CategoryVerificationFailed  = "verification_failed"
	CategoryUnknown             = "unknown"
)

// Default limits for bounding memory and serialization overhead.
const (
	DefaultMaxErrorLength   = 4096
	DefaultMaxActionsLimit  = 64
	DefaultMaxConstraints   = 64
	DefaultMaxArtifactCount = 32
)

// FailureEnvelope captures structured, project-agnostic diagnostic details
// across task execution attempts.
type FailureEnvelope struct {
	Attempt            int                  `json:"attempt"`
	ExitCode           int                  `json:"exit_code"`
	ErrorMessage       string               `json:"error_message"`
	Category           string               `json:"category"`
	FailingConstraints []string             `json:"failing_constraints,omitempty"`
	ExecutedActions    []string             `json:"executed_actions,omitempty"`
	DiagnosticReports  []DiagnosticArtifact `json:"diagnostic_artifacts,omitempty"`
	Signature          string               `json:"signature"`
	Timestamp          time.Time            `json:"timestamp"`
}

// ComputeSignature calculates a deterministic cryptographic fingerprint of the
// structural failure characteristics (category, exit code, normalized message,
// sorted constraints, and executed action sequence), invariant to timestamp.
func ComputeSignature(category string, exitCode int, errMsg string, constraints, actions []string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "category:%s\n", category)
	_, _ = fmt.Fprintf(h, "exit_code:%d\n", exitCode)

	normalizedMsg := strings.TrimSpace(errMsg)
	if len(normalizedMsg) > DefaultMaxErrorLength {
		normalizedMsg = normalizedMsg[:DefaultMaxErrorLength]
	}
	_, _ = fmt.Fprintf(h, "error:%s\n", normalizedMsg)

	// Sorted constraints for deterministic ordering
	sortedConstraints := make([]string, len(constraints))
	copy(sortedConstraints, constraints)
	sort.Strings(sortedConstraints)
	for _, c := range sortedConstraints {
		_, _ = fmt.Fprintf(h, "constraint:%s\n", strings.TrimSpace(c))
	}

	// Action sequence order is significant
	for _, a := range actions {
		_, _ = fmt.Fprintf(h, "action:%s\n", strings.TrimSpace(a))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// ClassifyError categorizes an error into a project-agnostic failure category.
func ClassifyError(err error, exitCode int, constraints []string) string {
	if err == nil && exitCode == 0 && len(constraints) == 0 {
		return CategorySuccess
	}
	if len(constraints) > 0 {
		return CategoryConstraintViolation
	}
	if err == nil {
		if exitCode != 0 {
			return CategoryExecutionFailure
		}
		return CategoryUnknown
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "timed out") || strings.Contains(msg, "timeout"):
		return CategoryTimeout
	case strings.Contains(msg, "context canceled") || strings.Contains(msg, "context cancelled"):
		return CategoryContextCancelled
	case strings.Contains(msg, "verification failed") || strings.Contains(msg, "verifier rejected"):
		return CategoryVerificationFailed
	case exitCode != 0:
		return CategoryExecutionFailure
	default:
		return CategoryUnknown
	}
}

// ExtractFailureEnvelope produces a bounded, structured FailureEnvelope from an attempt outcome.
func ExtractFailureEnvelope(attempt int, exitCode int, err error, executedActions []string, failingConstraints []string, report *DiagnosticReport) FailureEnvelope {
	if attempt < 1 {
		attempt = 1
	}

	var errMsg string
	if err != nil {
		errMsg = strings.TrimSpace(err.Error())
		if len(errMsg) > DefaultMaxErrorLength {
			errMsg = errMsg[:DefaultMaxErrorLength]
		}
	}

	category := ClassifyError(err, exitCode, failingConstraints)

	// Copy and bound constraints
	var boundedConstraints []string
	if len(failingConstraints) > 0 {
		limit := len(failingConstraints)
		if limit > DefaultMaxConstraints {
			limit = DefaultMaxConstraints
		}
		boundedConstraints = make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			c := strings.TrimSpace(failingConstraints[i])
			if c != "" {
				boundedConstraints = append(boundedConstraints, c)
			}
		}
	}

	// Copy and bound actions
	var boundedActions []string
	if len(executedActions) > 0 {
		limit := len(executedActions)
		if limit > DefaultMaxActionsLimit {
			limit = DefaultMaxActionsLimit
		}
		boundedActions = make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			a := strings.TrimSpace(executedActions[i])
			if a != "" {
				boundedActions = append(boundedActions, a)
			}
		}
	}

	// Extract artifacts if present
	var artifacts []DiagnosticArtifact
	if report != nil && len(report.Results) > 0 {
		for _, res := range report.Results {
			if res.OK && res.Artifact != nil {
				artifacts = append(artifacts, *res.Artifact)
				if len(artifacts) >= DefaultMaxArtifactCount {
					break
				}
			}
		}
	}

	sig := ComputeSignature(category, exitCode, errMsg, boundedConstraints, boundedActions)

	return FailureEnvelope{
		Attempt:            attempt,
		ExitCode:           exitCode,
		ErrorMessage:       errMsg,
		Category:           category,
		FailingConstraints: boundedConstraints,
		ExecutedActions:    boundedActions,
		DiagnosticReports:  artifacts,
		Signature:          sig,
		Timestamp:          time.Now().UTC(),
	}
}

// IsSuccess returns true if the envelope represents a successful execution.
func (e FailureEnvelope) IsSuccess() bool {
	return e.Category == CategorySuccess && e.ExitCode == 0 && e.ErrorMessage == "" && len(e.FailingConstraints) == 0
}

// HasSignatureMatch checks if another envelope has an identical failure signature.
func (e FailureEnvelope) HasSignatureMatch(other FailureEnvelope) bool {
	return e.Signature != "" && e.Signature == other.Signature
}
