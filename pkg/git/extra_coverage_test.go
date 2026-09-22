// BLI-STARTER-COMMUNITY-050 / PRI-STARTER-COMMUNITY-050 coverage elevation
package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type extraGitStore struct {
	storage.NoopObjectStorage
	exists bool
}

func (s extraGitStore) Exists(context.Context, *pkgctx.SecurityContext, string) (bool, error) {
	return s.exists, nil
}

func (extraGitStore) Create(context.Context, *pkgctx.SecurityContext, map[string]any) error {
	return nil
}

func (extraGitStore) Update(context.Context, *pkgctx.SecurityContext, string, map[string]any) error {
	return nil
}

func (extraGitStore) Read(context.Context, *pkgctx.SecurityContext, string) (map[string]any, error) {
	return map[string]any{
		objects.FieldKeyCommitHashes:    []any{"old"},
		objects.FieldKeyBacklogItemRefs: []any{"BLI-1"},
		objects.FieldKeyMilestoneRefs:   []any{"MIL-2"},
		objects.FieldKeyGoalRefs:        []any{"GOAL-3"},
		objects.FieldKeyWorkstreamRefs:  []any{"WS-4"},
		objects.FieldKeyRequirementRefs: []any{"REQ-5"},
	}, nil
}

func TestExtraFacadeAnalyzerAndHelpers(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	work := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := execwrap.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := fileutil.WriteFile(filepath.Join(work, "a"), []byte("1"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	f := NewFacade(work)
	if _, err := f.StatusShort(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.AddAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit("BLI-1 MIL-2 GOAL-3 WS-4 REQ-5 REQU-6 extra"); err != nil {
		t.Fatal(err)
	}
	br, err := f.CurrentBranch()
	if err != nil || br == "" {
		t.Fatalf("branch %q %v", br, err)
	}
	if !f.BranchExists(br) || f.BranchExists("no-such-extra-branch") {
		t.Fatal("exists")
	}
	if _, err := f.CreateBranch("extra-br"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.CheckoutBranch(br); err != nil {
		t.Fatal(err)
	}
	_, _ = f.DiffCachedStat()
	_, _ = f.CommitsAheadOneline(br)
	_, _ = f.OriginURL()
	_, _ = f.Pull(false)
	_, _ = f.Pull(true)
	_, _ = f.Push(false)
	_, _ = f.Push(true)
	_ = f.CountAheadUpstream()
	_ = NewFacade("").CountAheadUpstream()
	var nilF *Facade
	_ = nilF.CountAheadUpstream()

	if err := fileutil.WriteFile(filepath.Join(work, "b"), []byte("2"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.AddAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit("second extra"); err != nil {
		t.Fatal(err)
	}

	hashCmd := execwrap.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = work
	raw, err := hashCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimSpace(string(raw))
	ca := NewCommitAnalyzer(work)
	_ = NewCommitAnalyzerWithConfig(work, nil)
	_ = NewCommitAnalyzerWithConfig(work, &CommitAnalysisConfig{})
	_ = NewCommitAnalyzerWithConfig(work, &CommitAnalysisConfig{Timeout: time.Second, MaxWorkers: 1})
	ctx := context.Background()
	c, err := ca.AnalyzeCommit(ctx, hash)
	if err != nil || c == nil {
		t.Fatalf("analyze %v %v", c, err)
	}
	empty, err := ca.AnalyzeCommits(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty %v %v", empty, err)
	}
	if _, err := ca.AnalyzeCommits(ctx, []string{hash}); err != nil {
		t.Fatal(err)
	}
	if _, err := ca.AnalyzeRecent(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ca.AnalyzeRange(ctx, "HEAD"); err != nil {
		t.Fatal(err)
	}
	c.BacklogItemRefs = []string{"BLI-1"}
	c.MilestoneRefs = []string{"MIL-2"}
	c.GoalRefs = []string{"GOAL-3"}
	c.WorkstreamRefs = []string{"WS-4"}
	c.RequirementRefs = []string{"REQ-5"}
	if len(c.FilesChanged) == 0 {
		c.FilesChanged = []FileChange{{Path: "a", Status: "added", LinesAdded: 1}}
	}
	sec := pkgctx.NewSecurityContext("acct", nil, nil)
	cis := NewCommitIntegrationService(work, extraGitStore{}, sec)
	cis.SetLogger(logging.NewEventLogger(pkgctx.NewSystemContext()))
	if err := cis.LinkCommitToWorkItems(ctx, c); err != nil {
		t.Fatal(err)
	}
	cis2 := NewCommitIntegrationService(work, extraGitStore{exists: true}, sec)
	if err := cis2.LinkCommitToWorkItems(ctx, c); err != nil {
		t.Fatal(err)
	}
	_, _ = ca.analyzeCommitWithRetry(ctx, hash, &RetryConfig{MaxAttempts: 1, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, BackoffFactor: 2})
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = ca.AnalyzeCommit(cancelCtx, hash)
	_, _ = ca.AnalyzeCommit(ctx, "not-a-hash")
	_ = DefaultRetryConfig()
	_ = mergeStringLists([]any{"a", "a", 1}, []string{"a", "b"})
	_ = sanitizeForID("Hello World!! extra-long-name-that-should-be-truncated-for-id-use")
	_ = uniqueStrings([]string{"a", "a", "b"})
	_ = isRetryableGitError(nil)
	_ = isRetryableGitError(context.Canceled)
	_ = isRetryableGitError(context.DeadlineExceeded)
	_ = isRetryableGitError(errfmtTimeout("timeout deadline connection network temporary lock permission denied"))
	ResetMetrics()
	RecordAnalysis(time.Millisecond, true)
	RecordAnalysis(time.Millisecond, false)
	RecordLinking(time.Millisecond, true)
	RecordLinking(time.Millisecond, false)
	RecordRetry()
	RecordTimeout()
	_ = GetMetrics()
}

type extraTimeoutErr struct{ s string }

func errfmtTimeout(s string) extraTimeoutErr { return extraTimeoutErr{s: s} }
func (e extraTimeoutErr) Error() string      { return e.s }
