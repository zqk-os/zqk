package mcp

import (
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/logging"
)

// mapValues collects all values from a map into a slice
// This is a DRY helper to avoid repeating the common pattern of iterating
// over a map and appending values to a slice
func mapValues[K comparable, V any](m map[K]V) []V {
	if len(m) == 0 {
		return nil
	}
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// mapValuesDeref collects all values from a map of pointers into a slice of dereferenced values
// This is a variant of mapValues for maps where values are pointers
func mapValuesDeref[K comparable, V any](m map[K]*V) []V {
	if len(m) == 0 {
		return nil
	}
	values := make([]V, 0, len(m))
	for _, v := range m {
		if v != nil {
			values = append(values, *v)
		}
	}
	return values
}

// generateSequenceID generates a unique sequence ID for a new client connection
func (s *Server) generateSequenceID() string {
	return fmt.Sprintf("seq_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}

// getOrCreateSequenceID gets the current sequence ID or creates a new one
func (s *Server) getOrCreateSequenceID() string {
	var sequenceID string
	_ = concurrency.RunInLockWithLogger(
		&s.sequenceIDMu, LockNameMcpServerGetOrCreateSequenceId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if s.currentSequenceID == emptyValue {
				s.currentSequenceID = s.generateSequenceID()
			}
			sequenceID = s.currentSequenceID
			return nil
		},
	)
	return sequenceID
}
