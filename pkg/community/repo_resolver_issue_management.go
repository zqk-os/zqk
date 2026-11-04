package community

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// GitHub issue state enums.
const (
	GitHubIssueStateOpen   = "open"
	GitHubIssueStateClosed = "closed"
)

// GitHub issue resolution status enums.
const (
	IssueResolutionResolved  = "resolved"
	IssueResolutionDeferred  = "deferred"
	IssueResolutionDuplicate = "duplicate"
	IssueResolutionWontFix   = "wontfix"
)

// Domain errors for GitHub issue ingestion and resolution lifecycle.
var (
	ErrInvalidIssueNumber       = errors.New("repo-resolver: issue number must be positive")
	ErrEmptyIssueTitle          = errors.New("repo-resolver: issue title cannot be empty")
	ErrInvalidIssueState        = errors.New("repo-resolver: invalid issue state")
	ErrInvalidResolutionStatus  = errors.New("repo-resolver: invalid resolution status")
	ErrIssueNotFound            = errors.New("repo-resolver: issue not found")
	ErrNilGitHubIssue           = errors.New("repo-resolver: nil github issue")
	ErrNilGitHubIssueResolution = errors.New("repo-resolver: nil github issue resolution")
)

// GitHubIssue captures ingested metadata and state for a repository issue.
type GitHubIssue struct {
	IssueNumber int        `json:"issue_number" yaml:"issue_number"`
	Title       string     `json:"title" yaml:"title"`
	State       string     `json:"state" yaml:"state"`
	Labels      []string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Body        string     `json:"body,omitempty" yaml:"body,omitempty"`
	Author      string     `json:"author,omitempty" yaml:"author,omitempty"`
	URL         string     `json:"url,omitempty" yaml:"url,omitempty"`
	CreatedAt   time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" yaml:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty" yaml:"closed_at,omitempty"`
}

// Validate checks the structural invariants of a GitHubIssue.
func (i *GitHubIssue) Validate() error {
	if i == nil {
		return ErrNilGitHubIssue
	}
	if i.IssueNumber <= 0 {
		return fmt.Errorf("%w: got %d", ErrInvalidIssueNumber, i.IssueNumber)
	}
	if strings.TrimSpace(i.Title) == "" {
		return ErrEmptyIssueTitle
	}
	switch i.State {
	case GitHubIssueStateOpen, GitHubIssueStateClosed:
		// Valid state
	default:
		return fmt.Errorf("%w: %q (expected open or closed)", ErrInvalidIssueState, i.State)
	}
	return nil
}

// IsCritical evaluates whether the issue is a blocking defect requiring immediate convergence triage.
func (i *GitHubIssue) IsCritical() bool {
	if i == nil {
		return false
	}
	for _, l := range i.Labels {
		norm := strings.ToLower(strings.TrimSpace(l))
		if norm == "critical" || norm == "blocker" || norm == "p0" || norm == "p1" || norm == "security" || norm == "bug" {
			return true
		}
	}
	titleLower := strings.ToLower(i.Title)
	return strings.Contains(titleLower, "[critical]") || strings.Contains(titleLower, "[blocker]") || strings.Contains(titleLower, "panic")
}

// GitHubIssueResolution captures the resolution evidence, target commit, and outcome status.
type GitHubIssueResolution struct {
	IssueNumber      int       `json:"issue_number" yaml:"issue_number"`
	ResolutionStatus string    `json:"resolution_status" yaml:"resolution_status"`
	TargetCommitHash string    `json:"target_commit_hash,omitempty" yaml:"target_commit_hash,omitempty"`
	ResolutionNotes  string    `json:"resolution_notes,omitempty" yaml:"resolution_notes,omitempty"`
	ResolvedAt       time.Time `json:"resolved_at" yaml:"resolved_at"`
}

// Validate checks structural and logical invariants of a GitHubIssueResolution.
func (r *GitHubIssueResolution) Validate() error {
	if r == nil {
		return ErrNilGitHubIssueResolution
	}
	if r.IssueNumber <= 0 {
		return fmt.Errorf("%w: got %d", ErrInvalidIssueNumber, r.IssueNumber)
	}
	switch r.ResolutionStatus {
	case IssueResolutionResolved, IssueResolutionDeferred, IssueResolutionDuplicate, IssueResolutionWontFix:
		// Valid resolution status
	default:
		return fmt.Errorf("%w: %q", ErrInvalidResolutionStatus, r.ResolutionStatus)
	}
	return nil
}

func (s *RepoResolverConvergenceSession) ensureIssueMapsLocked() {
	if s.Issues == nil {
		s.Issues = make(map[int]GitHubIssue)
	}
	if s.Resolutions == nil {
		s.Resolutions = make(map[int]GitHubIssueResolution)
	}
}

// IngestGitHubIssue records or updates a single GitHub issue in the convergence session.
func (s *RepoResolverConvergenceSession) IngestGitHubIssue(issue GitHubIssue) error {
	if s == nil {
		return ErrNilConvergenceSession
	}
	if err := issue.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureIssueMapsLocked()
	s.Issues[issue.IssueNumber] = issue
	return nil
}

// IngestGitHubIssues records multiple GitHub issues in the convergence session.
func (s *RepoResolverConvergenceSession) IngestGitHubIssues(issues []GitHubIssue) error {
	if s == nil {
		return ErrNilConvergenceSession
	}
	for _, issue := range issues {
		if err := issue.Validate(); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureIssueMapsLocked()
	for _, issue := range issues {
		s.Issues[issue.IssueNumber] = issue
	}
	return nil
}

// ResolveGitHubIssue records a resolution for an existing tracked GitHub issue.
func (s *RepoResolverConvergenceSession) ResolveGitHubIssue(res GitHubIssueResolution) error {
	if s == nil {
		return ErrNilConvergenceSession
	}
	if err := res.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureIssueMapsLocked()

	issue, exists := s.Issues[res.IssueNumber]
	if !exists {
		return fmt.Errorf("%w: issue #%d has not been ingested", ErrIssueNotFound, res.IssueNumber)
	}

	if res.ResolvedAt.IsZero() {
		res.ResolvedAt = time.Now().UTC()
	}

	s.Resolutions[res.IssueNumber] = res

	// If resolved, duplicate, or wontfix, transition issue state to closed
	if res.ResolutionStatus != IssueResolutionDeferred {
		issue.State = GitHubIssueStateClosed
		now := res.ResolvedAt
		issue.ClosedAt = &now
		issue.UpdatedAt = now
		s.Issues[res.IssueNumber] = issue
	}

	return nil
}

// ListPendingGitHubIssues returns all open, unresolved issues sorted deterministically by issue number.
func (s *RepoResolverConvergenceSession) ListPendingGitHubIssues() []GitHubIssue {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listPendingGitHubIssuesLocked()
}

func (s *RepoResolverConvergenceSession) listPendingGitHubIssuesLocked() []GitHubIssue {
	if s.Issues == nil {
		return nil
	}

	var pending []GitHubIssue
	for num, issue := range s.Issues {
		if issue.State == GitHubIssueStateClosed {
			continue
		}
		if res, resolved := s.Resolutions[num]; resolved && res.ResolutionStatus != IssueResolutionDeferred {
			continue
		}
		pending = append(pending, issue)
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].IssueNumber < pending[j].IssueNumber
	})
	return pending
}

// ListResolvedGitHubIssues returns all recorded resolutions sorted deterministically by issue number.
func (s *RepoResolverConvergenceSession) ListResolvedGitHubIssues() []GitHubIssueResolution {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listResolvedGitHubIssuesLocked()
}

func (s *RepoResolverConvergenceSession) listResolvedGitHubIssuesLocked() []GitHubIssueResolution {
	if s.Resolutions == nil {
		return nil
	}

	var resolved []GitHubIssueResolution
	for _, res := range s.Resolutions {
		resolved = append(resolved, res)
	}

	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].IssueNumber < resolved[j].IssueNumber
	})
	return resolved
}

// GetIssueResolution retrieves the recorded resolution for a specific issue number.
func (s *RepoResolverConvergenceSession) GetIssueResolution(issueNumber int) (*GitHubIssueResolution, error) {
	if s == nil {
		return nil, ErrNilConvergenceSession
	}
	if issueNumber <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidIssueNumber, issueNumber)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Resolutions == nil {
		return nil, fmt.Errorf("%w: issue #%d", ErrIssueNotFound, issueNumber)
	}

	res, exists := s.Resolutions[issueNumber]
	if !exists {
		return nil, fmt.Errorf("%w: issue #%d", ErrIssueNotFound, issueNumber)
	}

	resCopy := res
	return &resCopy, nil
}

// CountPendingCriticalIssues returns the count of active unresolved critical/blocking issues.
func (s *RepoResolverConvergenceSession) CountPendingCriticalIssues() int {
	if s == nil {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.countPendingCriticalIssuesLocked()
}

func (s *RepoResolverConvergenceSession) countPendingCriticalIssuesLocked() int {
	pending := s.listPendingGitHubIssuesLocked()
	count := 0
	for _, p := range pending {
		if p.IsCritical() {
			count++
		}
	}
	return count
}
