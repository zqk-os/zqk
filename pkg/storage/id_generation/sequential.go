package id_generation

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// SequentialStrategy generates sequential IDs (e.g., ITEM-001, ITEM-002, ITEM-003)
// This is the default strategy and matches current system behavior
// Now uses thread-safe batch generator for consistency
type SequentialStrategy struct {
	minDigits int // Minimum number of digits (default: 3)
	startAt   int // Starting sequence number (default: 1)
}

// NewSequentialStrategy creates a new sequential ID generation strategy
func NewSequentialStrategy() *SequentialStrategy {
	return &SequentialStrategy{
		minDigits: 3,
		startAt:   1,
	}
}

// NewSequentialStrategyWithParams creates a sequential strategy with custom parameters
func NewSequentialStrategyWithParams(params map[string]any) *SequentialStrategy {
	s := NewSequentialStrategy()

	if minDigits, ok := params["min_digits"].(int); ok && minDigits > 0 {
		s.minDigits = minDigits
	}
	if startAt, ok := params["start_at"].(int); ok && startAt >= 0 {
		s.startAt = startAt
	}

	return s
}

// Name returns the strategy name
func (s *SequentialStrategy) Name() string {
	return "sequential"
}

// Description returns a description of the sequential strategy
func (s *SequentialStrategy) Description() string {
	return ConstGeneratesSequentialNumericIDs
}

// GenerateNextID generates the next sequential ID
// Uses thread-safe batch generator for consistency with audit ID generation
// Note: kindDir is inferred from existingIDs context (legacy interface compatibility)
// For new code, use GetBatchIDGenerator directly with known kindDir
func (s *SequentialStrategy) GenerateNextID(ctx context.Context, kind string, prefix string, existingIDs []string) (string, error) {
	if prefix == emptyValue {
		return "", errfmt.Errorf(ConstPrefixIsRequiredForSequentialIDGeneration)
	}

	// Try to extract kindDir from context if available
	// This is a fallback for legacy code that doesn't provide kindDir
	var kindDir string
	if ctx != nil {
		if dir, ok := ctx.Value("kindDir").(string); ok {
			kindDir = dir
		}
	}

	// If kindDir not available, we need to scan existingIDs (legacy behavior)
	// This is less efficient but maintains backward compatibility
	if kindDir == emptyValue {
		return s.generateFromExistingIDs(prefix, existingIDs)
	}

	// Use thread-safe batch generator (preferred path)
	generator := GetBatchIDGenerator(ctx, kindDir, kind, prefix, s.minDigits, s.startAt)
	return generator.GenerateNextID()
}

// generateFromExistingIDs generates ID by scanning existing IDs (legacy fallback)
// This maintains backward compatibility when kindDir is not available
func (s *SequentialStrategy) generateFromExistingIDs(prefix string, existingIDs []string) (string, error) {
	// This is the old implementation - kept for backward compatibility
	// New code should use GetBatchIDGenerator directly with known kindDir
	maxSeq := s.startAt - 1

	// Pattern to extract sequence number from IDs like "ITEM-001", "ITEM-100", etc.
	prefixPattern := regexp.QuoteMeta(prefix)
	if len(prefixPattern) > 0 && prefixPattern[len(prefixPattern)-1] != '-' {
		prefixPattern += "-"
	}
	seqPattern := regexp.MustCompile(fmt.Sprintf(`^%s(\d+)$`, prefixPattern))

	for _, id := range existingIDs {
		matches := seqPattern.FindStringSubmatch(id)
		if len(matches) > 1 {
			seq, err := strconv.Atoi(matches[1])
			if err == nil && seq > maxSeq {
				maxSeq = seq
			}
		}
	}

	// Generate next ID
	nextSeq := maxSeq + 1
	if nextSeq < s.startAt {
		nextSeq = s.startAt
	}

	// Ensure prefix has trailing dash for formatting
	prefixWithDash := prefix
	if len(prefixWithDash) == 0 || prefixWithDash[len(prefixWithDash)-1] != '-' {
		prefixWithDash += "-"
	}

	// Format with minimum digits
	formatStr := fmt.Sprintf("%%s%%0%dd", s.minDigits)
	id := fmt.Sprintf(formatStr, prefixWithDash, nextSeq)

	return id, nil
}
