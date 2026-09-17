package system

import (
	"strings"

	"github.com/lanceman/zqk/pkg/logging"
)

// DiagnosticCacheItemStrategy is a strategy for tracking specific objects for diagnostic purposes
// This is useful for debugging cache issues with known problematic objects
type DiagnosticCacheItemStrategy struct {
	objectIDs map[string]bool
	name      string
}

// NewDiagnosticCacheItemStrategy creates a new diagnostic strategy for specific object IDs
func NewDiagnosticCacheItemStrategy(name string, objectIDs []string) *DiagnosticCacheItemStrategy {
	ids := make(map[string]bool)
	for _, id := range objectIDs {
		ids[id] = true
	}
	return &DiagnosticCacheItemStrategy{
		objectIDs: ids,
		name:      name,
	}
}

func (s *DiagnosticCacheItemStrategy) ShouldTrack(objectID string) bool {
	return s.objectIDs[objectID]
}

func (s *DiagnosticCacheItemStrategy) OnCacheMiss(objectID string, cacheSize int) (bool, logging.LogLevel, string) {
	return true, logging.WarnLevel, "Diagnostic object not found in cache after build - cache may be incomplete"
}

func (s *DiagnosticCacheItemStrategy) OnCacheHit(objectID string) (bool, logging.LogLevel, string) {
	return false, logging.DebugLevel, "" // Don't log hits by default
}

func (s *DiagnosticCacheItemStrategy) OnCacheLoad(objectID string, cacheSize int) (bool, logging.LogLevel, string) {
	return true, logging.DebugLevel, "Diagnostic object found in cache after load"
}

func (s *DiagnosticCacheItemStrategy) Name() string {
	return s.name
}

// PatternBasedCacheItemStrategy is a strategy that matches objects by pattern (prefix, suffix, regex, etc.)
type PatternBasedCacheItemStrategy struct {
	pattern   string
	matchFunc func(objectID, pattern string) bool
	name      string
	logLevel  logging.LogLevel
}

// NewPrefixBasedCacheItemStrategy creates a strategy that matches objects by prefix
func NewPrefixBasedCacheItemStrategy(name, prefix string, logLevel logging.LogLevel) *PatternBasedCacheItemStrategy {
	return &PatternBasedCacheItemStrategy{
		pattern:   prefix,
		matchFunc: strings.HasPrefix,
		name:      name,
		logLevel:  logLevel,
	}
}

// NewSuffixBasedCacheItemStrategy creates a strategy that matches objects by suffix
func NewSuffixBasedCacheItemStrategy(name, suffix string, logLevel logging.LogLevel) *PatternBasedCacheItemStrategy {
	return &PatternBasedCacheItemStrategy{
		pattern:   suffix,
		matchFunc: strings.HasSuffix,
		name:      name,
		logLevel:  logLevel,
	}
}

func (s *PatternBasedCacheItemStrategy) ShouldTrack(objectID string) bool {
	return s.matchFunc(objectID, s.pattern)
}

func (s *PatternBasedCacheItemStrategy) OnCacheMiss(objectID string, cacheSize int) (bool, logging.LogLevel, string) {
	return true, s.logLevel, "Pattern-matched object not found in cache"
}

func (s *PatternBasedCacheItemStrategy) OnCacheHit(objectID string) (bool, logging.LogLevel, string) {
	return false, logging.DebugLevel, "" // Don't log hits by default
}

func (s *PatternBasedCacheItemStrategy) OnCacheLoad(objectID string, cacheSize int) (bool, logging.LogLevel, string) {
	return false, logging.DebugLevel, "" // Don't log loads by default
}

func (s *PatternBasedCacheItemStrategy) Name() string {
	return s.name
}

// NoOpCacheItemStrategy is a strategy that does nothing (useful for disabling strategies)
type NoOpCacheItemStrategy struct {
	name string
}

func NewNoOpCacheItemStrategy(name string) *NoOpCacheItemStrategy {
	return &NoOpCacheItemStrategy{name: name}
}

func (s *NoOpCacheItemStrategy) ShouldTrack(objectID string) bool {
	return false
}

func (s *NoOpCacheItemStrategy) OnCacheMiss(objectID string, cacheSize int) (bool, logging.LogLevel, string) {
	return false, logging.DebugLevel, ""
}

func (s *NoOpCacheItemStrategy) OnCacheHit(objectID string) (bool, logging.LogLevel, string) {
	return false, logging.DebugLevel, ""
}

func (s *NoOpCacheItemStrategy) OnCacheLoad(objectID string, cacheSize int) (bool, logging.LogLevel, string) {
	return false, logging.DebugLevel, ""
}

func (s *NoOpCacheItemStrategy) Name() string {
	return s.name
}
