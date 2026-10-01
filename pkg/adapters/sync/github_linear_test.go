package sync

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

const (
	testGHOriginalTitle      = `Refactor cache eviction pipeline`
	testGHOriginalBody       = `Cache eviction blocks background flushes under load.`
	testGHResolvedTitle      = `Refactor cache eviction pipeline (Resolved)`
	testGHResolvedBody       = `Fixed via lock-free queue.`
	testGHNewerKernelTitle   = `Newer Kernel Data`
	testGHStaleExternalTitle = `Stale External Data`

	testLinOriginalTitle = `Original linear title`
	testLinOriginalDesc  = `Original description`
	testLinPushedTitle   = `Pushed title from zqk kernel`
	testLinPushedDesc    = `Pushed description from zqk kernel`

	testLinStudioTitle   = `Support Web Studio Embedded UI`
	testLinStudioDesc    = `Run lightweight web UI server from zqk ui --web.`
	testLinStudioProject = `Visual Studio Plane`
	testLinStudioPlanRef = `PRI-VISUAL-STUDIO-PLANE`

	testUpdatedKernelTitle = `Updated Title from Kernel`
	testUpdatedKernelBody  = `Updated Problem Statement from Kernel`

	testErrIssueNotFoundFmt   = "issue %d not found"
	testErrLinearNotFoundFmt  = "linear issue %s not found"
	testLinearEng204ID        = "linear-ENG-204"
	testLinearEng300ID        = "linear-ENG-300"
	testBliLinearEng300ID     = "BLI-LIN-ENG-300"
	testFileGHLinearSrc       = "./github_linear.go"
	testFileGHLinearTestSrc   = "./github_linear_test.go"
	testMsgGHZeroViolations   = "github_linear.go must have zero violations"
	testMsgTestZeroViolations = "github_linear_test.go must have zero violations"
)

// mockGitHubClient is an in-memory test double for GitHubClient.
type mockGitHubClient struct {
	mu     sync.Mutex
	issues map[int]GitHubIssue
}

func newMockGitHubClient() *mockGitHubClient {
	return &mockGitHubClient{issues: make(map[int]GitHubIssue)}
}

func (m *mockGitHubClient) ListIssues(ctx context.Context) ([]GitHubIssue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]GitHubIssue, 0, len(m.issues))
	for _, issue := range m.issues {
		res = append(res, issue)
	}
	return res, nil
}

func (m *mockGitHubClient) CreateIssue(ctx context.Context, issue GitHubIssue) (GitHubIssue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if issue.Number == 0 {
		issue.Number = len(m.issues) + 1
	}
	m.issues[issue.Number] = issue
	return issue, nil
}

func (m *mockGitHubClient) UpdateIssue(ctx context.Context, number int, issue GitHubIssue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.issues[number]
	if !ok {
		return fmt.Errorf(testErrIssueNotFoundFmt, number)
	}
	existing.Title = issue.Title
	existing.Body = issue.Body
	existing.State = issue.State
	existing.Labels = issue.Labels
	existing.UpdatedAt = time.Now()
	m.issues[number] = existing
	return nil
}

// mockLinearClient is an in-memory test double for LinearClient.
type mockLinearClient struct {
	mu     sync.Mutex
	issues map[string]LinearIssue
}

func newMockLinearClient() *mockLinearClient {
	return &mockLinearClient{issues: make(map[string]LinearIssue)}
}

func (m *mockLinearClient) ListIssues(ctx context.Context) ([]LinearIssue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]LinearIssue, 0, len(m.issues))
	for _, issue := range m.issues {
		res = append(res, issue)
	}
	return res, nil
}

func (m *mockLinearClient) CreateIssue(ctx context.Context, issue LinearIssue) (LinearIssue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issues[issue.ID] = issue
	return issue, nil
}

func (m *mockLinearClient) UpdateIssue(ctx context.Context, id string, issue LinearIssue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.issues[id]
	if !ok {
		return fmt.Errorf(testErrLinearNotFoundFmt, id)
	}
	existing.Title = issue.Title
	existing.Description = issue.Description
	existing.State = issue.State
	existing.Priority = issue.Priority
	existing.UpdatedAt = time.Now()
	m.issues[id] = existing
	return nil
}

// mockKernelStore is an in-memory test double for KernelStore.
type mockKernelStore struct {
	mu    sync.Mutex
	items map[string]*BacklogItemSyncData // keyed by extID
}

func newMockKernelStore() *mockKernelStore {
	return &mockKernelStore{items: make(map[string]*BacklogItemSyncData)}
}

func (m *mockKernelStore) GetBacklogItemByExternalID(ctx context.Context, source ExternalSource, extID string) (*BacklogItemSyncData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[extID]
	if !ok || item.ExternalSource != source {
		return nil, nil
	}
	cp := *item
	return &cp, nil
}

func (m *mockKernelStore) UpsertBacklogItem(ctx context.Context, item *BacklogItemSyncData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *item
	m.items[item.ExternalID] = &cp
	return nil
}

func (m *mockKernelStore) ListBacklogItems(ctx context.Context) ([]*BacklogItemSyncData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]*BacklogItemSyncData, 0, len(m.items))
	for _, it := range m.items {
		cp := *it
		res = append(res, &cp)
	}
	return res, nil
}

func TestGitHubIngestAndIdempotency(t *testing.T) {
	ctx := context.Background()
	gh := newMockGitHubClient()
	store := newMockKernelStore()
	engine := NewSyncEngine(gh, nil, store)

	t0 := time.Now().Add(-10 * time.Minute)
	gh.issues[42] = GitHubIssue{
		Number:    42,
		Title:     testGHOriginalTitle,
		Body:      testGHOriginalBody,
		State:     "open",
		Labels:    []string{"P1", "performance", "active"},
		Assignees: []string{"alice"},
		HTMLURL:   "https://github.com/zqk-os/zqk/issues/42",
		UpdatedAt: t0,
	}

	// First ingest: creates 1 item
	stats1, err := engine.IngestGitHub(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats1.IngestedCount)
	assert.Equal(t, 1, stats1.CreatedCount)
	assert.Equal(t, 0, stats1.SkippedCount)

	item, err := store.GetBacklogItemByExternalID(ctx, SourceGitHub, "gh-42")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, testGHOriginalTitle, item.Title)
	assert.Equal(t, StatusInProgress, item.Status) // from "active" label
	assert.Equal(t, TierP1, item.PriorityTier)
	assert.Equal(t, PriorityHigh, item.Priority)
	assert.Equal(t, "gh-42", item.ExternalID)

	// Second ingest without changes: skipped idempotently
	stats2, err := engine.IngestGitHub(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats2.IngestedCount)
	assert.Equal(t, 0, stats2.CreatedCount)
	assert.Equal(t, 1, stats2.SkippedCount)

	// Update issue in GitHub with newer timestamp
	t1 := time.Now()
	gh.issues[42] = GitHubIssue{
		Number:    42,
		Title:     testGHResolvedTitle,
		Body:      testGHResolvedBody,
		State:     "closed",
		Labels:    []string{"P1"},
		HTMLURL:   "https://github.com/zqk-os/zqk/issues/42",
		UpdatedAt: t1,
	}

	stats3, err := engine.IngestGitHub(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats3.UpdatedCount)

	updatedItem, err := store.GetBacklogItemByExternalID(ctx, SourceGitHub, "gh-42")
	require.NoError(t, err)
	assert.Equal(t, StatusComplete, updatedItem.Status) // mapped from closed
	assert.Equal(t, testGHResolvedTitle, updatedItem.Title)
}

func TestLinearIngestAndPlanMapping(t *testing.T) {
	ctx := context.Background()
	lin := newMockLinearClient()
	store := newMockKernelStore()
	engine := NewSyncEngine(nil, lin, store)

	t0 := time.Now().Add(-5 * time.Minute)
	lin.issues["ENG-204"] = LinearIssue{
		ID:          "ENG-204",
		Title:       testLinStudioTitle,
		Description: testLinStudioDesc,
		State:       "In Progress",
		Assignee:    "bob",
		ProjectName: testLinStudioProject,
		Priority:    1, // Urgent / P0
		URL:         "https://linear.app/zqk/issue/ENG-204",
		UpdatedAt:   t0,
	}

	stats, err := engine.IngestLinear(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.CreatedCount)

	item, err := store.GetBacklogItemByExternalID(ctx, SourceLinear, testLinearEng204ID)
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, testLinStudioTitle, item.Title)
	assert.Equal(t, StatusInProgress, item.Status)
	assert.Equal(t, TierP0, item.PriorityTier)
	assert.Equal(t, PriorityCritical, item.Priority)
	assert.Equal(t, testLinStudioPlanRef, item.PriorityPlanRef)
	assert.Equal(t, "bob", item.ExternalMetadata[metaKeyAssignee])

	// Ingest again: verified idempotent
	stats2, err := engine.IngestLinear(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats2.SkippedCount)
	assert.Equal(t, 0, stats2.CreatedCount)
}

func TestBidirectionalPushToGitHub(t *testing.T) {
	ctx := context.Background()
	gh := newMockGitHubClient()
	store := newMockKernelStore()
	engine := NewSyncEngine(gh, nil, store)

	gh.issues[99] = GitHubIssue{
		Number:    99,
		Title:     "Old Title",
		Body:      "Old Body",
		State:     "open",
		Labels:    []string{TierP2},
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	// Store has updated item with status complete
	store.items["gh-99"] = &BacklogItemSyncData{
		ID:               "BLI-GH-99",
		Title:            testUpdatedKernelTitle,
		ProblemStatement: testUpdatedKernelBody,
		Status:           StatusComplete,
		PriorityTier:     TierP0,
		ExternalSource:   SourceGitHub,
		ExternalID:       "gh-99",
		ExternalMetadata: map[string]string{metaKeyLabels: "bug"},
		UpdatedAt:        time.Now(),
	}

	pushStats, err := engine.PushKernelToGitHub(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, pushStats.PushedCount)

	ghIssue := gh.issues[99]
	assert.Equal(t, testUpdatedKernelTitle, ghIssue.Title)
	assert.Equal(t, testUpdatedKernelBody, ghIssue.Body)
	assert.Equal(t, "closed", ghIssue.State) // mapped from complete
	assert.Contains(t, ghIssue.Labels, TierP0)
}

func TestBidirectionalPushToLinear(t *testing.T) {
	ctx := context.Background()
	lin := newMockLinearClient()
	store := newMockKernelStore()
	engine := NewSyncEngine(nil, lin, store)

	lin.issues["ENG-300"] = LinearIssue{
		ID:          "ENG-300",
		Title:       testLinOriginalTitle,
		Description: testLinOriginalDesc,
		State:       "Todo",
		Priority:    3,
		UpdatedAt:   time.Now().Add(-2 * time.Hour),
	}

	store.items[testLinearEng300ID] = &BacklogItemSyncData{
		ID:             testBliLinearEng300ID,
		Title:          testLinPushedTitle,
		Description:    testLinPushedDesc,
		Status:         StatusComplete,
		PriorityTier:   TierP1,
		ExternalSource: SourceLinear,
		ExternalID:     testLinearEng300ID,
		UpdatedAt:      time.Now(),
	}

	pushStats, err := engine.PushKernelToLinear(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, pushStats.PushedCount)

	linIssue := lin.issues["ENG-300"]
	assert.Equal(t, testLinPushedTitle, linIssue.Title)
	assert.Equal(t, "Done", linIssue.State) // mapped from complete
	assert.Equal(t, 2, linIssue.Priority)   // mapped from P1
}

func TestConflictResolutionMonotonicPrecedence(t *testing.T) {
	ctx := context.Background()
	gh := newMockGitHubClient()
	store := newMockKernelStore()
	engine := NewSyncEngine(gh, nil, store)

	baseTime := time.Now()

	// Kernel store has newer modification
	store.items["gh-15"] = &BacklogItemSyncData{
		ID:             "BLI-GH-15",
		Title:          testGHNewerKernelTitle,
		Status:         StatusInProgress,
		PriorityTier:   TierP1,
		ExternalSource: SourceGitHub,
		ExternalID:     "gh-15",
		UpdatedAt:      baseTime.Add(10 * time.Minute),
	}

	// GitHub sends stale modification
	gh.issues[15] = GitHubIssue{
		Number:    15,
		Title:     testGHStaleExternalTitle,
		State:     "open",
		UpdatedAt: baseTime,
	}

	stats, err := engine.IngestGitHub(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.ConflictCount)
	assert.Equal(t, 1, stats.SkippedCount)
	assert.Equal(t, 0, stats.UpdatedCount)

	// Kernel data was preserved
	cur, getErr := store.GetBacklogItemByExternalID(ctx, SourceGitHub, "gh-15")
	require.NoError(t, getErr)
	assert.Equal(t, testGHNewerKernelTitle, cur.Title)
}

func TestASTCompliance(t *testing.T) {
	auditor := qa.NewASTAuditor()
	v1, err := auditor.AuditFile(testFileGHLinearSrc)
	require.NoError(t, err)
	assert.Empty(t, v1, testMsgGHZeroViolations)

	v2, err := auditor.AuditFile(testFileGHLinearTestSrc)
	require.NoError(t, err)
	assert.Empty(t, v2, testMsgTestZeroViolations)
}
