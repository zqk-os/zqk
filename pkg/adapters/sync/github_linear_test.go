package sync

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		return fmt.Errorf("issue %d not found", number)
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
		return fmt.Errorf("linear issue %s not found", id)
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
		Title:     "Refactor cache eviction pipeline",
		Body:      "Cache eviction blocks background flushes under load.",
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
	assert.Equal(t, "Refactor cache eviction pipeline", item.Title)
	assert.Equal(t, "in_progress", item.Status) // from "active" label
	assert.Equal(t, "P1", item.PriorityTier)
	assert.Equal(t, "high", item.Priority)
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
		Title:     "Refactor cache eviction pipeline (Resolved)",
		Body:      "Fixed via lock-free queue.",
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
	assert.Equal(t, "complete", updatedItem.Status) // mapped from closed
	assert.Equal(t, "Refactor cache eviction pipeline (Resolved)", updatedItem.Title)
}

func TestLinearIngestAndPlanMapping(t *testing.T) {
	ctx := context.Background()
	lin := newMockLinearClient()
	store := newMockKernelStore()
	engine := NewSyncEngine(nil, lin, store)

	t0 := time.Now().Add(-5 * time.Minute)
	lin.issues["ENG-204"] = LinearIssue{
		ID:          "ENG-204",
		Title:       "Support Web Studio Embedded UI",
		Description: "Run lightweight web UI server from zqk ui --web.",
		State:       "In Progress",
		Assignee:    "bob",
		ProjectName: "Visual Studio Plane",
		Priority:    1, // Urgent / P0
		URL:         "https://linear.app/zqk/issue/ENG-204",
		UpdatedAt:   t0,
	}

	stats, err := engine.IngestLinear(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.CreatedCount)

	item, err := store.GetBacklogItemByExternalID(ctx, SourceLinear, "linear-ENG-204")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "Support Web Studio Embedded UI", item.Title)
	assert.Equal(t, "in_progress", item.Status)
	assert.Equal(t, "P0", item.PriorityTier)
	assert.Equal(t, "critical", item.Priority)
	assert.Equal(t, "PRI-VISUAL-STUDIO-PLANE", item.PriorityPlanRef)
	assert.Equal(t, "bob", item.ExternalMetadata["assignee"])

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
		Labels:    []string{"P2"},
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	// Store has updated item with status complete
	store.items["gh-99"] = &BacklogItemSyncData{
		ID:               "BLI-GH-99",
		Title:            "Updated Title from Kernel",
		ProblemStatement: "Updated Problem Statement from Kernel",
		Status:           "complete",
		PriorityTier:     "P0",
		ExternalSource:   SourceGitHub,
		ExternalID:       "gh-99",
		ExternalMetadata: map[string]string{"labels": "bug"},
		UpdatedAt:        time.Now(),
	}

	pushStats, err := engine.PushKernelToGitHub(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, pushStats.PushedCount)

	ghIssue := gh.issues[99]
	assert.Equal(t, "Updated Title from Kernel", ghIssue.Title)
	assert.Equal(t, "Updated Problem Statement from Kernel", ghIssue.Body)
	assert.Equal(t, "closed", ghIssue.State) // mapped from complete
	assert.Contains(t, ghIssue.Labels, "P0")
}

func TestBidirectionalPushToLinear(t *testing.T) {
	ctx := context.Background()
	lin := newMockLinearClient()
	store := newMockKernelStore()
	engine := NewSyncEngine(nil, lin, store)

	lin.issues["ENG-300"] = LinearIssue{
		ID:          "ENG-300",
		Title:       "Original linear title",
		Description: "Original description",
		State:       "Todo",
		Priority:    3,
		UpdatedAt:   time.Now().Add(-2 * time.Hour),
	}

	store.items["linear-ENG-300"] = &BacklogItemSyncData{
		ID:             "BLI-LIN-ENG-300",
		Title:          "Pushed title from zqk kernel",
		Description:    "Pushed description from zqk kernel",
		Status:         "complete",
		PriorityTier:   "P1",
		ExternalSource: SourceLinear,
		ExternalID:     "linear-ENG-300",
		UpdatedAt:      time.Now(),
	}

	pushStats, err := engine.PushKernelToLinear(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, pushStats.PushedCount)

	linIssue := lin.issues["ENG-300"]
	assert.Equal(t, "Pushed title from zqk kernel", linIssue.Title)
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
		ID:               "BLI-GH-15",
		Title:            "Newer Kernel Data",
		Status:           "in_progress",
		PriorityTier:     "P1",
		ExternalSource:   SourceGitHub,
		ExternalID:       "gh-15",
		UpdatedAt:        baseTime.Add(10 * time.Minute),
	}

	// GitHub sends stale modification
	gh.issues[15] = GitHubIssue{
		Number:    15,
		Title:     "Stale External Data",
		State:     "open",
		UpdatedAt: baseTime,
	}

	stats, err := engine.IngestGitHub(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.ConflictCount)
	assert.Equal(t, 1, stats.SkippedCount)
	assert.Equal(t, 0, stats.UpdatedCount)

	// Kernel data was preserved
	cur, _ := store.GetBacklogItemByExternalID(ctx, SourceGitHub, "gh-15")
	assert.Equal(t, "Newer Kernel Data", cur.Title)
}
