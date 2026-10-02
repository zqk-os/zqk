package testdiscovery

import (
	"bufio"
	"bytes"
	"context"
	"regexp"
	"strings"
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

type lineDiscoveryState struct {
	scanner      *bufio.Scanner
	targets      []DiscoveredTarget
	currentSuite string
	pendingTags  []string
	pendingCrit  []string
	pendingReq   []string
	lineNum      int
}

func newLineDiscoveryState(content []byte) *lineDiscoveryState {
	return &lineDiscoveryState{
		scanner: bufio.NewScanner(bytes.NewReader(content)),
	}
}

func (s *lineDiscoveryState) appendMetadata(crit, req, tags []string) {
	s.pendingCrit = append(s.pendingCrit, crit...)
	s.pendingReq = append(s.pendingReq, req...)
	s.pendingTags = append(s.pendingTags, tags...)
}

func (s *lineDiscoveryState) resetPending() {
	s.pendingTags = nil
	s.pendingCrit = nil
	s.pendingReq = nil
}

func parseCommaSeparatedMatches(regex *regexp.Regexp, s string) []string {
	m := regex.FindStringSubmatch(s)
	if len(m) <= 1 {
		return nil
	}
	var items []string
	for _, p := range strings.Split(m[1], ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			items = append(items, p)
		}
	}
	return items
}

func extractCriteriaAndReqs(comment string) ([]string, []string) {
	return parseCommaSeparatedMatches(critRegex, comment), parseCommaSeparatedMatches(reqRegex, comment)
}

func extractCommentMetadata(comment string) (crit, req, tags []string) {
	crit = parseCommaSeparatedMatches(critRegex, comment)
	req = parseCommaSeparatedMatches(reqRegex, comment)
	if m := tagRegex.FindStringSubmatch(comment); len(m) > 1 {
		tag := strings.TrimSpace(m[1])
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return crit, req, tags
}

