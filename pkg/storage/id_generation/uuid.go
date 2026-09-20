package id_generation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// UUIDStrategy generates UUID-like IDs using random hex strings
// Useful when you need globally unique IDs without sequential ordering
type UUIDStrategy struct {
	length int // Length of random hex string (default: 32 for full UUID-like, 8 for short)
}

// NewUUIDStrategy creates a new UUID-like ID generation strategy
// Default length is 8 characters for readability (can be overridden via params)
func NewUUIDStrategy() *UUIDStrategy {
	return &UUIDStrategy{
		length: 8, // Short format by default (8 hex chars = 4 bytes) for readability
	}
}

// NewUUIDStrategyWithParams creates a UUID strategy with custom parameters
func NewUUIDStrategyWithParams(params map[string]any) *UUIDStrategy {
	s := NewUUIDStrategy()

	if length, ok := params["length"].(int); ok && length > 0 {
		s.length = length
	} else if short, ok := params[objects.FieldKeyShort].(bool); ok && short {
		s.length = 8 // Short format
	}

	return s
}

// Name returns the strategy name
func (s *UUIDStrategy) Name() string {
	return "uuid"
}

// Description returns a description of the UUID strategy
func (s *UUIDStrategy) Description() string {
	if s.length == 8 {
		return ConstGeneratesUUIDLikeIDsUsingShortRandomHexFormat
	}
	return ConstGeneratesUUIDLikeIDsUsingRandomHexFormat
}

// GenerateNextID generates a UUID-like ID using random hex
// Collision handling:
//   - Checks collisions within same kind (operational context)
//   - Retries up to maxRetries times on collision
//   - Full namespace-qualified URI provides uniqueness for external aggregation
//   - White-label namespace seeds (from business registrations) provide uniqueness across instances
func (s *UUIDStrategy) GenerateNextID(ctx context.Context, kind string, prefix string, existingIDs []string) (string, error) {
	if prefix == emptyValue {
		return "", errfmt.Errorf(ConstPrefixIsRequiredForUUIDIDGeneration)
	}

	maxRetries := 5 // Retry up to 5 times on collision (extremely unlikely to collide 5 times)

	// Build a set of existing IDs for fast lookup
	existingSet := make(map[string]bool, len(existingIDs))
	for _, id := range existingIDs {
		existingSet[id] = true
	}

	// Generate ID with collision detection and retry
	var newID string
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Generate random bytes
		bytes := make([]byte, s.length/2) // Each hex char represents 4 bits, so length/2 bytes
		if _, err := rand.Read(bytes); err != nil {
			return "", errfmt.Newf(ConstFailedToGenerateRandomID).Wrap(err)
		}

		// Convert to hex string
		randomHex := hex.EncodeToString(bytes)
		newID = fmt.Sprintf("%s-%s", prefix, randomHex)

		// Check for collision within same kind (operational context)
		// Note: Full namespace-qualified URI [namespace_id:domain:organization:<sub-org>:kind:id]
		// provides uniqueness for external aggregation, even if IDs collide across kinds/namespaces.
		// White-label namespace seeds (from business registrations) provide uniqueness across instances.
		if !existingSet[newID] {
			// No collision - return the ID
			return newID, nil
		}

		// Collision detected - will retry (unless this is the last attempt)
	}

	// All retries exhausted - this should be extremely rare
	// For 8-char hex UUIDs (4 bytes = 4.3B possibilities):
	// - 1,000 objects: ~0.0001% collision risk per generation
	// - Probability of 5 collisions in a row: astronomically low
	return "", errfmt.Errorf(ConstFailedToGenerateUniqueUUIDIDAfterDAttempts, maxRetries, prefix, kind)
}
