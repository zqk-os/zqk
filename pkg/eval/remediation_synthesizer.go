package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var (
	errEmptyScenarioID = errors.New("scenario_id cannot be empty")
	errEmptyErrorMsg   = errors.New("error message cannot be empty")

	reTimestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`)
	reHexAddr   = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	reDuration  = regexp.MustCompile(`\(\d+(?:\.\d+)?(?:ms|s|µs|ns)\)`)
)

// FailureCategory classifies the failure mode of a scenario evaluation.
type FailureCategory string

const (
	CategoryPanic     FailureCategory = "panic"
	CategoryBoundary  FailureCategory = "boundary"
	CategoryAssertion FailureCategory = "assertion"
	CategoryTimeout   FailureCategory = "timeout"
	CategoryGeneral   FailureCategory = "general"

	SeverityP0 = "P0"
	SeverityP1 = "P1"
	SeverityP2 = "P2"
)

// FailurePayload represents the input failure telemetry from an evaluation run.
type FailurePayload struct {
	ScenarioID    string
	ModelName     string
	ErrorMessage  string
	Category      FailureCategory
	CodeLocation  string
	SessionID     string
	ExecutionMeta map[string]string
}

// RemediationItem holds the synthesized defect/remediation record.
type RemediationItem struct {
	Title        string
	Fingerprint  string
	Severity     string
	ScenarioID   string
	ModelName    string
	CodeLocation string
	Summary      string
	Payload      map[string]any
}

// RemediationResult summarizes the synthesizer decision.
type RemediationResult struct {
	Minted       bool
	Deduplicated bool
	Fingerprint  string
	Severity     string
	Title        string
}

// RemediationMinter abstracts creation of remediation records in kernel storage.
type RemediationMinter func(ctx context.Context, item RemediationItem) error

// RemediationSynthesizer provides deterministic failure synthesis and deduplication.
type RemediationSynthesizer struct {
	mu           sync.RWMutex
	fingerprints map[string]string
	minter       RemediationMinter
}

// NewRemediationSynthesizer constructs a new synthesizer.
func NewRemediationSynthesizer(minter RemediationMinter) *RemediationSynthesizer {
	return &RemediationSynthesizer{
		fingerprints: make(map[string]string),
		minter:       minter,
	}
}

// NormalizeErrorMessage strips transient runtime data to produce deterministic signatures.
func NormalizeErrorMessage(msg string) string {
	clean := reTimestamp.ReplaceAllString(msg, "<TIMESTAMP>")
	clean = reHexAddr.ReplaceAllString(clean, "<ADDR>")
	clean = reDuration.ReplaceAllString(clean, "<DURATION>")
	return strings.TrimSpace(clean)
}

// ComputeFailureFingerprint calculates deterministic SHA-256 hash across failure dimensions.
func ComputeFailureFingerprint(scenarioID, modelName, normalizedMsg string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s:%s:%s", scenarioID, modelName, normalizedMsg)
	return hex.EncodeToString(h.Sum(nil))
}

// ClassifySeverity maps failure category to priority tier.
func ClassifySeverity(category FailureCategory) string {
	switch category {
	case CategoryPanic, CategoryBoundary:
		return SeverityP0
	case CategoryAssertion:
		return SeverityP1
	default:
		return SeverityP2
	}
}

// Synthesize validates telemetry, checks deduplication, and mints remediation if novel.
func (s *RemediationSynthesizer) Synthesize(ctx context.Context, p FailurePayload) (*RemediationResult, error) {
	if strings.TrimSpace(p.ScenarioID) == "" {
		return nil, errEmptyScenarioID
	}
	if strings.TrimSpace(p.ErrorMessage) == "" {
		return nil, errEmptyErrorMsg
	}

	normMsg := NormalizeErrorMessage(p.ErrorMessage)
	fp := ComputeFailureFingerprint(p.ScenarioID, p.ModelName, normMsg)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.fingerprints[fp]; exists {
		return &RemediationResult{
			Minted:       false,
			Deduplicated: true,
			Fingerprint:  fp,
			Severity:     ClassifySeverity(p.Category),
			Title:        formatTitle(p.ScenarioID, normMsg),
		}, nil
	}

	sev := ClassifySeverity(p.Category)
	title := formatTitle(p.ScenarioID, normMsg)

	item := RemediationItem{
		Title:        title,
		Fingerprint:  fp,
		Severity:     sev,
		ScenarioID:   p.ScenarioID,
		ModelName:    p.ModelName,
		CodeLocation: p.CodeLocation,
		Summary:      normMsg,
		Payload: map[string]any{
			"scenario_id": p.ScenarioID,
			"model_name":  p.ModelName,
			"category":    string(p.Category),
			"fingerprint": fp,
		},
	}

	if s.minter != nil {
		if err := s.minter(ctx, item); err != nil {
			return nil, fmt.Errorf("remediation minter failed: %w", err)
		}
	}

	s.fingerprints[fp] = title
	return &RemediationResult{
		Minted:       true,
		Deduplicated: false,
		Fingerprint:  fp,
		Severity:     sev,
		Title:        title,
	}, nil
}

// ActiveFingerprintCount returns the number of deduplicated fingerprints tracked.
func (s *RemediationSynthesizer) ActiveFingerprintCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.fingerprints)
}

func formatTitle(scenarioID, normMsg string) string {
	maxLen := 50
	summary := normMsg
	if len(summary) > maxLen {
		summary = summary[:maxLen] + "..."
	}
	return fmt.Sprintf("[EVAL-FAILURE] %s: %s", scenarioID, summary)
}
