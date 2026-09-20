package testdiscovery

import (
	"context"
	"time"
)

// DiscoveredTarget represents a discovered test function, suite, or file target.
type DiscoveredTarget struct {
	ID               string   `json:"id"`
	Path             string   `json:"path"`
	Language         string   `json:"language"`
	Package          string   `json:"package,omitempty"`
	Suite            string   `json:"suite,omitempty"`
	Function         string   `json:"function"`
	Line             int      `json:"line"`
	Tags             []string `json:"tags,omitempty"`
	CriteriaRefs     []string `json:"criteria_refs,omitempty"`
	RequirementRefs  []string `json:"requirement_refs,omitempty"`
	ExecutionCommand string   `json:"execution_command,omitempty"`
}

// DiscoveryOptions defines configuration for a test discovery run.
type DiscoveryOptions struct {
	ProjectRoot  string
	Paths        []string
	Languages    []string
	ExcludePaths []string
	Workers      int
	Incremental  bool
	CachePath    string
	Timeout      time.Duration
}

// Discoverer defines the interface for language-specific static AST/file discovery.
type Discoverer interface {
	Language() string
	CanHandle(relPath string) bool
	Discover(ctx context.Context, projectRoot, relPath string, content []byte) ([]DiscoveredTarget, error)
}
