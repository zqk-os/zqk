package system

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/git"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

// GitAnalyzeContext groups state for git analyze operation
type GitAnalyzeContext struct {
	Proc     *cli.Processor
	Ctx      context.Context
	SecCtx   *pkgctx.SecurityContext
	Logger   *logging.EventLogger
	RepoPath string
	Analyzer *git.CommitAnalyzer
	Hash     string
	Recent   int
	Link     bool
	Format   cli.OutputFormat
}

// initializeGitAnalyzeContext sets up the git analyze context
func initializeGitAnalyzeContext(cmd *cobra.Command) (*GitAnalyzeContext, error) {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, errfmt.Newf("failed to create processor").Wrap(err)
	}

	ctx := proc.OperationContext()
	secCtx := proc.SecurityContext()
	logger := proc.Logger()

	repoPath, err := findGitRepository()
	if err != nil {
		return nil, err
	}

	analyzerConfig := &git.CommitAnalysisConfig{
		Timeout:     30 * time.Second,
		MaxWorkers:  5,
		RetryConfig: git.DefaultRetryConfig(),
	}
	analyzer := git.NewCommitAnalyzerWithConfig(repoPath, analyzerConfig)

	hash, _ := cmd.Flags().GetString("hash")
	recent, _ := cmd.Flags().GetInt("recent")
	link, _ := cmd.Flags().GetBool("link")
	format := proc.Format()

	return &GitAnalyzeContext{
		Proc:     proc,
		Ctx:      ctx,
		SecCtx:   secCtx,
		Logger:   logger,
		RepoPath: repoPath,
		Analyzer: analyzer,
		Hash:     hash,
		Recent:   recent,
		Link:     link,
		Format:   format,
	}, nil
}

// findGitRepository finds the git repository by walking up the directory tree
func findGitRepository() (string, error) {
	repoPath, err := os.Getwd()
	if err != nil {
		return "", errfmt.Newf("failed to get current directory").Wrap(err)
	}

	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		dir := repoPath
		for {
			parent := filepath.Dir(dir)
			if parent == dir {
				return "", errfmt.Errorf("not a Git repository")
			}
			dir = parent
			gitDir = filepath.Join(dir, ".git")
			if _, err := os.Stat(gitDir); err == nil {
				return dir, nil
			}
		}
	}

	return repoPath, nil
}

// analyzeCommits determines which commits to analyze based on flags and args
func analyzeCommits(analyzeCtx *GitAnalyzeContext, args []string) ([]*git.Commit, error) {
	if analyzeCtx.Hash != emptyValue {
		return analyzeCommitByHash(analyzeCtx)
	}
	if analyzeCtx.Recent > 0 {
		return analyzeCtx.Analyzer.AnalyzeRecent(analyzeCtx.Ctx, analyzeCtx.Recent)
	}
	if len(args) > 0 {
		return analyzeCtx.Analyzer.AnalyzeRange(analyzeCtx.Ctx, args[0])
	}
	// Default: analyze last 10 commits
	return analyzeCtx.Analyzer.AnalyzeRecent(analyzeCtx.Ctx, 10)
}

// analyzeCommitByHash analyzes a specific commit by hash
func analyzeCommitByHash(analyzeCtx *GitAnalyzeContext) ([]*git.Commit, error) {
	commit, err := analyzeCtx.Analyzer.AnalyzeCommit(analyzeCtx.Ctx, analyzeCtx.Hash)
	if err != nil {
		return nil, errfmt.Newf("failed to analyze commit").Wrap(err)
	}
	return []*git.Commit{commit}, nil
}

// linkCommitsToWorkItems links commits to work items concurrently
func linkCommitsToWorkItems(analyzeCtx *GitAnalyzeContext, commits []*git.Commit) error {
	integrationService := git.NewCommitIntegrationService(analyzeCtx.RepoPath, analyzeCtx.Proc.Storage(), analyzeCtx.SecCtx)
	integrationService.SetLogger(analyzeCtx.Logger)

	type linkResult struct {
		commit *git.Commit
		err    error
	}
	linkChan := make(chan linkResult, len(commits))
	var wg sync.WaitGroup

	for _, commit := range commits {
		c := commit // Capture loop variable
		goroutinelabels.NewGoroutine("git_commit_linker", fmt.Sprintf("linking commit %s to work items", c.Hash[:8])).
			WithWaitGroup(&wg).
			StartSimple(func() {
				err := integrationService.LinkCommitToWorkItems(analyzeCtx.Ctx, c)
				linkChan <- linkResult{commit: c, err: err}
			})
	}

	goroutinelabels.NewGoroutine("git_commit_linker_collector", "collecting commit linking results").
		WithCleanup(func() {
			close(linkChan)
		}).
		StartSimple(func() {
			wg.Wait()
		})

	linkedCount := 0
	failedCount := 0
	for result := range linkChan {
		if result.err != nil {
			failedCount++
			logging.FluentEvent(analyzeCtx.Logger).Warn("Failed to link commit to work items").
				String("hash", result.commit.ShortHash).
				WithError(result.err).
				Log()
		} else {
			linkedCount++
			logging.FluentEvent(analyzeCtx.Logger).Info("Linked commit to work items").
				String("hash", result.commit.ShortHash).
				Int("backlog_items", len(result.commit.BacklogItemRefs)).
				Int("milestones", len(result.commit.MilestoneRefs)).
				Int("goals", len(result.commit.GoalRefs)).
				Log()
		}
	}

	logging.FluentEvent(analyzeCtx.Logger).Info("Commit linking completed").
		Int("linked", linkedCount).
		Int("failed", failedCount).
		Log()

	return nil
}

// outputCommitsStructured writes commits using internal/cli.FormatOutput (json, jsonl, yaml).
func outputCommitsStructured(cmd *cobra.Command, commits []*git.Commit) error {
	output := map[string]any{
		"commits": commits,
	}
	return cli.FormatOutput(cmd, output)
}

// outputCommitsTable outputs commits as a table
func outputCommitsTable(cmd *cobra.Command, commits []*git.Commit) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Analyzed %d commits:\n\n", len(commits))
	for _, commit := range commits {
		writeCommitDetails(&buf, commit)
	}
	return cli.WriteOutput(cmd, buf.Bytes())
}

// writeCommitDetails writes details for a single commit.
// POLICY-CODE-007: buffer output via WriteString(Sprintf); preferFprint disabled for this function.
//
//nolint:gocritic // preferFprint
func writeCommitDetails(buf *bytes.Buffer, commit *git.Commit) {
	fmt.Fprintf(buf, "Commit: %s\n", commit.ShortHash)
	fmt.Fprintf(buf, "  Author: %s <%s>\n", commit.Author, commit.AuthorEmail)
	fmt.Fprintf(buf, "  Date: %s\n", commit.Date.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(buf, "  Message: %s\n", commit.Subject)
	if len(commit.BacklogItemRefs) > 0 {
		fmt.Fprintf(buf, "  Backlog Items: %v\n", commit.BacklogItemRefs)
	}
	if len(commit.MilestoneRefs) > 0 {
		fmt.Fprintf(buf, "  Milestones: %v\n", commit.MilestoneRefs)
	}
	if len(commit.GoalRefs) > 0 {
		fmt.Fprintf(buf, "  Goals: %v\n", commit.GoalRefs)
	}
	fmt.Fprintf(buf, "  Files: %d changed, +%d -%d lines\n\n",
		len(commit.FilesChanged), commit.LinesAdded, commit.LinesRemoved)
}

// logGitMetrics logs git operation metrics
func logGitMetrics(logger *logging.EventLogger) {
	metrics := git.GetMetrics()
	logging.FluentEvent(logger).Info("Git operation metrics").
		Int("commits_analyzed", int(metrics.CommitsAnalyzed)).
		Int("commits_linked", int(metrics.CommitsLinked)).
		Int("commits_failed", int(metrics.CommitsFailed)).
		String("avg_analysis_time", metrics.AvgAnalysisTime.String()).
		String("avg_linking_time", metrics.AvgLinkingTime.String()).
		Int("retries", int(metrics.RetryCount)).
		Int("timeouts", int(metrics.TimeoutCount)).
		Log()
}
