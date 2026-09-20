package git

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

const emptyValue = ""

// Commit represents a Git commit with extracted work item references
type Commit struct {
	Hash        string    // Full commit hash
	ShortHash   string    // Short commit hash (first 7 chars)
	Author      string    // Author name
	AuthorEmail string    // Author email
	Date        time.Time // Commit date
	Message     string    // Full commit message
	Subject     string    // First line of commit message
	Body        string    // Rest of commit message (if any)

	// Extracted work item references
	BacklogItemRefs []string // BLI-* references
	MilestoneRefs   []string // MIL-* references
	GoalRefs        []string // GOAL-* references
	WorkstreamRefs  []string // WS-* references
	RequirementRefs []string // REQ-* references

	// File changes
	FilesChanged []FileChange

	// Statistics
	LinesAdded    int
	LinesRemoved  int
	FilesAdded    int
	FilesRemoved  int
	FilesModified int
}

// FileChange represents a file change in a commit
type FileChange struct {
	Path         string // File path
	Status       string // Added, Modified, Deleted, Renamed
	LinesAdded   int
	LinesRemoved int
}

// CommitAnalyzer analyzes Git commits and extracts work item references
type CommitAnalyzer struct {
	repoPath       string
	defaultTimeout time.Duration
	maxWorkers     int
}

// CommitAnalysisConfig configures commit analysis behavior
type CommitAnalysisConfig struct {
	Timeout     time.Duration // Timeout for Git operations (default: 30s)
	MaxWorkers  int           // Maximum concurrent commit analyses (default: 5)
	RetryConfig *RetryConfig  // Retry configuration for failed operations
}

// RetryConfig configures retry behavior for Git operations
type RetryConfig struct {
	MaxAttempts   int           // Maximum number of attempts (default: 3)
	InitialDelay  time.Duration // Initial delay before retry (default: 100ms)
	MaxDelay      time.Duration // Maximum delay between retries (default: 5s)
	BackoffFactor float64       // Exponential backoff factor (default: 2.0)
}

// NewCommitAnalyzer creates a new commit analyzer for the given repository path
func NewCommitAnalyzer(repoPath string) *CommitAnalyzer {
	return &CommitAnalyzer{
		repoPath:       repoPath,
		defaultTimeout: 30 * time.Second,
		maxWorkers:     5,
	}
}

// NewCommitAnalyzerWithConfig creates a new commit analyzer with custom configuration
func NewCommitAnalyzerWithConfig(repoPath string, config *CommitAnalysisConfig) *CommitAnalyzer {
	analyzer := &CommitAnalyzer{
		repoPath: repoPath,
	}
	if config != nil {
		if config.Timeout > 0 {
			analyzer.defaultTimeout = config.Timeout
		} else {
			analyzer.defaultTimeout = 30 * time.Second
		}
		if config.MaxWorkers > 0 {
			analyzer.maxWorkers = config.MaxWorkers
		} else {
			analyzer.maxWorkers = 5
		}
	}
	return analyzer
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
	}
}

// AnalyzeCommit analyzes a single commit by hash with timeout and retry support
func (ca *CommitAnalyzer) AnalyzeCommit(ctx context.Context, hash string) (*Commit, error) {
	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, ca.defaultTimeout)
	defer cancel()

	// Execute with retry if configured
	return ca.analyzeCommitWithRetry(timeoutCtx, hash, nil)
}

// analyzeCommitWithRetry analyzes a commit with retry logic
func (ca *CommitAnalyzer) analyzeCommitWithRetry(ctx context.Context, hash string, retryConfig *RetryConfig) (*Commit, error) {
	if retryConfig == nil {
		retryConfig = DefaultRetryConfig()
	}

	var lastErr error
	delay := retryConfig.InitialDelay

	for attempt := 0; attempt < retryConfig.MaxAttempts; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			RecordTimeout()
			return nil, ctx.Err()
		}

		// Execute the analysis
		commit, err := ca.analyzeCommitOnce(ctx, hash)
		if err == nil {
			return commit, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableGitError(err) {
			return nil, err
		}

		// Last attempt, don't wait
		if attempt == retryConfig.MaxAttempts-1 {
			break
		}

		// Record retry
		RecordRetry()

		// Wait before retry
		select {
		case <-ctx.Done():
			RecordTimeout()
			return nil, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * retryConfig.BackoffFactor)
		if delay > retryConfig.MaxDelay {
			delay = retryConfig.MaxDelay
		}
	}

	return nil, errfmt.Errorf("failed to analyze commit after %d attempts: %w", retryConfig.MaxAttempts, lastErr)
}

// analyzeCommitOnce performs a single commit analysis attempt
func (ca *CommitAnalyzer) analyzeCommitOnce(ctx context.Context, hash string) (*Commit, error) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		RecordAnalysis(duration, true) // Will be updated if error occurs
	}()

	// Get commit details
	cmd := execwrap.CommandContext(ctx, "git", "log", "-1", "--format=%H|%an|%ae|%ad|%s|%b", "--date=iso", hash)
	cmd.Dir = ca.repoPath
	output, err := cmd.Output()
	if err != nil {
		duration := time.Since(startTime)
		RecordAnalysis(duration, false)
		if err == context.DeadlineExceeded || err == context.Canceled {
			RecordTimeout()
		}
		return nil, errfmt.Newf("failed to get commit details").Wrap(err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return nil, errfmt.Errorf("no commit data found for hash %s", hash)
	}

	// Parse commit line (format: HASH|AUTHOR|EMAIL|DATE|SUBJECT|BODY)
	parts := strings.SplitN(lines[0], "|", 6)
	if len(parts) < 5 {
		return nil, errfmt.Errorf("invalid commit format for hash %s", hash)
	}

	commit := &Commit{
		Hash:        parts[0],
		ShortHash:   parts[0][:7],
		Author:      parts[1],
		AuthorEmail: parts[2],
		Subject:     parts[4],
	}

	// Parse date
	if len(parts) >= 4 && parts[3] != emptyValue {
		if date, err := time.Parse("2006-01-02 15:04:05 -0700", parts[3]); err == nil {
			commit.Date = date
		} else if date, err := time.Parse(time.RFC3339, parts[3]); err == nil {
			commit.Date = date
		}
	}

	// Parse body (if present)
	if len(parts) >= 6 && parts[5] != emptyValue {
		commit.Body = parts[5]
		commit.Message = commit.Subject + "\n\n" + commit.Body
	} else {
		commit.Message = commit.Subject
	}

	// Extract work item references from message
	ca.extractWorkItemReferences(commit)

	// Get file changes
	if err := ca.getFileChanges(ctx, hash, commit); err != nil {
		return nil, errfmt.Newf("failed to get file changes").Wrap(err)
	}

	return commit, nil
}

// AnalyzeCommits analyzes multiple commits concurrently with worker pool
func (ca *CommitAnalyzer) AnalyzeCommits(ctx context.Context, hashes []string) ([]*Commit, error) {
	if len(hashes) == 0 {
		return []*Commit{}, nil
	}

	// Create context with timeout for entire operation
	timeoutCtx, cancel := context.WithTimeout(ctx, ca.defaultTimeout*time.Duration(len(hashes)))
	defer cancel()

	// Use worker pool for concurrent analysis
	type result struct {
		commit *Commit
		err    error
		index  int
	}

	hashChan := make(chan int, len(hashes))
	resultChan := make(chan result, len(hashes))

	// Populate hash channel
	for i := range hashes {
		hashChan <- i
	}
	close(hashChan)

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < ca.maxWorkers && w < len(hashes); w++ {
		goroutinelabels.NewGoroutine(fmt.Sprintf("git_commit_analyzer_%d", w), fmt.Sprintf("analyzing commits (worker %d)", w)).
			WithWaitGroup(&wg).
			StartWithContext(timeoutCtx, func(ctx context.Context) error {
				for i := range hashChan {
					select {
					case <-ctx.Done():
						resultChan <- result{err: ctx.Err(), index: i}
						return ctx.Err()
					default:
						commit, err := ca.AnalyzeCommit(ctx, hashes[i])
						resultChan <- result{commit: commit, err: err, index: i}
					}
				}
				return nil
			})
	}

	// Wait for all workers to complete
	goroutinelabels.NewGoroutine("git_commit_result_collector", "collecting commit analysis results").
		WithCleanup(func() {
			close(resultChan)
		}).
		StartSimple(func() {
			wg.Wait()
		})

	// Collect results
	commits := make([]*Commit, len(hashes))
	for res := range resultChan {
		if res.err != nil {
			return nil, errfmt.Errorf("failed to analyze commit %s: %w", hashes[res.index], res.err)
		}
		commits[res.index] = res.commit
	}

	return commits, nil
}

// AnalyzeRange analyzes commits in a range (e.g., "HEAD~10..HEAD" or "main..feature")
func (ca *CommitAnalyzer) AnalyzeRange(ctx context.Context, rangeSpec string) ([]*Commit, error) {
	// Get commit hashes in range
	cmd := execwrap.CommandContext(ctx, "git", "log", "--format=%H", rangeSpec)
	cmd.Dir = ca.repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, errfmt.Newf("failed to get commits in range").Wrap(err)
	}

	hashes := strings.Fields(string(output))
	if len(hashes) == 0 {
		return []*Commit{}, nil
	}

	return ca.AnalyzeCommits(ctx, hashes)
}

// AnalyzeRecent analyzes recent commits (last N commits)
func (ca *CommitAnalyzer) AnalyzeRecent(ctx context.Context, count int) ([]*Commit, error) {
	rangeSpec := fmt.Sprintf("HEAD~%d..HEAD", count)
	return ca.AnalyzeRange(ctx, rangeSpec)
}

// extractWorkItemReferences extracts work item references from commit message
func (ca *CommitAnalyzer) extractWorkItemReferences(commit *Commit) {
	// Patterns for different work item types
	patterns := map[string]*regexp.Regexp{
		objects.KindBacklogItem: regexp.MustCompile(`\bBLI-\d+\b`),
		objects.KindMilestone:   regexp.MustCompile(`\bMIL-\d+\b`),
		objects.KindGoal:        regexp.MustCompile(`\bGOAL-\d+\b`),
		objects.KindWorkstream:  regexp.MustCompile(`\bWS-\d+\b`),
		objects.KindRequirement: regexp.MustCompile(`\b(REQ-|REQU-)\d+\b`),
	}

	// Combine subject and body for searching
	fullText := commit.Subject + " " + commit.Body

	// Extract references
	if matches := patterns[objects.KindBacklogItem].FindAllString(fullText, -1); len(matches) > 0 {
		commit.BacklogItemRefs = uniqueStrings(matches)
	}
	if matches := patterns[objects.KindMilestone].FindAllString(fullText, -1); len(matches) > 0 {
		commit.MilestoneRefs = uniqueStrings(matches)
	}
	if matches := patterns[objects.KindGoal].FindAllString(fullText, -1); len(matches) > 0 {
		commit.GoalRefs = uniqueStrings(matches)
	}
	if matches := patterns[objects.KindWorkstream].FindAllString(fullText, -1); len(matches) > 0 {
		commit.WorkstreamRefs = uniqueStrings(matches)
	}
	if matches := patterns[objects.KindRequirement].FindAllString(fullText, -1); len(matches) > 0 {
		commit.RequirementRefs = uniqueStrings(matches)
	}
}

// getFileChanges gets file changes for a commit
func (ca *CommitAnalyzer) getFileChanges(ctx context.Context, hash string, commit *Commit) error {
	// Get file stats
	cmd := execwrap.CommandContext(ctx, "git", "show", "--stat", "--format=", hash)
	cmd.Dir = ca.repoPath
	output, err := cmd.Output()
	if err != nil {
		return errfmt.Newf("failed to get file stats").Wrap(err)
	}

	for line := range strings.SplitSeq(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == emptyValue || strings.HasPrefix(line, "---") {
			continue
		}

		// Parse stat line (e.g., "file.go | 10 +5 -3")
		// Format: "file.go | 10 +5 -3" or "file.go | 10 +" or "file.go | Bin 100 -> 200 bytes"
		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}

		path := strings.TrimSpace(parts[0])
		stats := strings.TrimSpace(parts[1])

		// Determine status
		status := "modified"
		added := 0
		removed := 0

		if strings.Contains(stats, "Bin") {
			// Binary file - parse "Bin 100 -> 200 bytes"
			// status already set to "modified" above
		} else {
			// Parse additions/deletions from format like "10 +5 -3" or "10 +" or "10 -"
			// First number is total changes, then +N for additions, -M for deletions
			var total int
			if n, err := fmt.Sscanf(stats, "%d", &total); n == 1 && err == nil {
				// Try to parse "+N -M" format
				if strings.Contains(stats, "+") && strings.Contains(stats, "-") {
					_ , _ = fmt.Sscanf(stats, "%d +%d -%d", &total, &added, &removed)
				} else if strings.Contains(stats, "+") {
					_ , _ = fmt.Sscanf(stats, "%d +%d", &total, &added)
				} else if strings.Contains(stats, "-") {
					_ , _ = fmt.Sscanf(stats, "%d -%d", &total, &removed)
				}
			}

			// Determine status - check if file was added or deleted
			// Use git diff --name-status to get accurate status
			statusCmd := execwrap.CommandContext(ctx, "git", "diff-tree", "--no-commit-id", "--name-status", "-r", hash)
			statusCmd.Dir = ca.repoPath
			statusOutput, err := statusCmd.Output()
			if err == nil {
				// Parse status lines (format: "A\tfile.go" or "M\tfile.go" or "D\tfile.go")
				for statusLine := range strings.SplitSeq(string(statusOutput), "\n") {
					if strings.HasPrefix(statusLine, "A\t") && strings.TrimPrefix(statusLine, "A\t") == path {
						status = "added"
						commit.FilesAdded++
						break
					} else if strings.HasPrefix(statusLine, "D\t") && strings.TrimPrefix(statusLine, "D\t") == path {
						status = "deleted"
						commit.FilesRemoved++
						break
					} else if strings.HasPrefix(statusLine, "M\t") && strings.TrimPrefix(statusLine, "M\t") == path {
						status = "modified"
						commit.FilesModified++
						break
					}
				}
			} else {
				// Fallback: guess based on lines
				if added > 0 && removed == 0 {
					status = "added"
					commit.FilesAdded++
				} else if removed > 0 && added == 0 {
					status = "deleted"
					commit.FilesRemoved++
				} else {
					status = "modified"
					commit.FilesModified++
				}
			}

			commit.LinesAdded += added
			commit.LinesRemoved += removed

			commit.FilesChanged = append(commit.FilesChanged, FileChange{
				Path:         path,
				Status:       status,
				LinesAdded:   added,
				LinesRemoved: removed,
			})
		}
	}

	return nil
}

// uniqueStrings returns unique strings from a slice
func uniqueStrings(strs []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, s := range strs {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// isRetryableGitError determines if a Git operation error is retryable
func isRetryableGitError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Context/timeout errors are retryable
	if err == context.DeadlineExceeded || err == context.Canceled {
		return true
	}

	// Check for timeout in error message
	if strings.Contains(strings.ToLower(errStr), "timeout") ||
		strings.Contains(strings.ToLower(errStr), "deadline") ||
		strings.Contains(strings.ToLower(errStr), "context canceled") {
		return true
	}

	// Network/connection errors are retryable
	if strings.Contains(strings.ToLower(errStr), "connection") ||
		strings.Contains(strings.ToLower(errStr), "network") ||
		strings.Contains(strings.ToLower(errStr), "temporary") {
		return true
	}

	// Git-specific retryable errors
	if strings.Contains(strings.ToLower(errStr), "lock") ||
		strings.Contains(strings.ToLower(errStr), "permission denied") {
		return true
	}

	// By default, don't retry (safer)
	return false
}
