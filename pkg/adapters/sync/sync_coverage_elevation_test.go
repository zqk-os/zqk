package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type failingGHClient struct {
	listErr   error
	updateErr error
}

func (f *failingGHClient) ListIssues(ctx context.Context) ([]GitHubIssue, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return []GitHubIssue{{Number: 10, Title: "GH 10", State: "open"}}, nil
}

func (f *failingGHClient) CreateIssue(ctx context.Context, issue GitHubIssue) (GitHubIssue, error) {
	return issue, nil
}

func (f *failingGHClient) UpdateIssue(ctx context.Context, number int, issue GitHubIssue) error {
	return f.updateErr
}

type failingLinearClient struct {
	listErr   error
	updateErr error
}

func (f *failingLinearClient) ListIssues(ctx context.Context) ([]LinearIssue, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return []LinearIssue{{ID: "ENG-10", Title: "Linear 10", State: "Todo"}}, nil
}

func (f *failingLinearClient) CreateIssue(ctx context.Context, issue LinearIssue) (LinearIssue, error) {
	return issue, nil
}

func (f *failingLinearClient) UpdateIssue(ctx context.Context, id string, issue LinearIssue) error {
	return f.updateErr
}

type failingStore struct {
	listErr   error
	getErr    error
	upsertErr error
	items     []*BacklogItemSyncData
}

func (f *failingStore) GetBacklogItemByExternalID(ctx context.Context, source ExternalSource, extID string) (*BacklogItemSyncData, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, it := range f.items {
		if it.ExternalID == extID && it.ExternalSource == source {
			return it, nil
		}
	}
	return nil, nil
}

func (f *failingStore) UpsertBacklogItem(ctx context.Context, item *BacklogItemSyncData) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.items = append(f.items, item)
	return nil
}

func (f *failingStore) ListBacklogItems(ctx context.Context) ([]*BacklogItemSyncData, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.items, nil
}

func TestMapGitHubIssueToKernel_Matrix(t *testing.T) {
	// Closed issue with P0 and assignees
	closedIssue := GitHubIssue{
		Number:    101,
		Title:     "P0 Security Flaw",
		Body:      "Details here",
		State:     "closed",
		Labels:    []string{"bug", "critical", "P0"},
		Assignees: []string{"alice", "bob"},
		HTMLURL:   "https://github.com/org/repo/issues/101",
		UpdatedAt: time.Now(),
	}
	mapped := MapGitHubIssueToKernel(closedIssue)
	if mapped.Status != StatusComplete {
		t.Errorf("expected complete, got %s", mapped.Status)
	}
	if mapped.Priority != PriorityCritical || mapped.PriorityTier != TierP0 {
		t.Errorf("expected P0/Critical, got %s / %s", mapped.Priority, mapped.PriorityTier)
	}
	if mapped.ExternalMetadata[metaKeyAssignees] != "alice,bob" {
		t.Errorf("unexpected assignees metadata: %v", mapped.ExternalMetadata)
	}

	// In-progress issue with P1
	inProgIssue := GitHubIssue{
		Number:    102,
		Title:     "Feature In Flight",
		State:     "open",
		Labels:    []string{"in-progress", "high"},
		UpdatedAt: time.Now(),
	}
	mapped2 := MapGitHubIssueToKernel(inProgIssue)
	if mapped2.Status != StatusInProgress {
		t.Errorf("expected in_progress, got %s", mapped2.Status)
	}
	if mapped2.PriorityTier != TierP1 || mapped2.Priority != PriorityHigh {
		t.Errorf("expected P1/High, got %s / %s", mapped2.Priority, mapped2.PriorityTier)
	}

	// Active issue with P3
	p3Issue := GitHubIssue{
		Number:    103,
		Title:     "Low priority cleanup",
		State:     "open",
		Labels:    []string{"active", "low"},
		UpdatedAt: time.Now(),
	}
	mapped3 := MapGitHubIssueToKernel(p3Issue)
	if mapped3.Status != StatusInProgress {
		t.Errorf("expected in_progress, got %s", mapped3.Status)
	}
	if mapped3.PriorityTier != TierP3 || mapped3.Priority != PriorityLow {
		t.Errorf("expected P3/Low, got %s / %s", mapped3.Priority, mapped3.PriorityTier)
	}
}

func TestMapLinearIssueToKernel_Matrix(t *testing.T) {
	states := []struct {
		state      string
		priority   int
		wantStatus string
		wantTier   string
	}{
		{"Done", 1, StatusComplete, TierP0},
		{"completed", 2, StatusComplete, TierP1},
		{"started", 3, StatusInProgress, TierP2},
		{"In Progress", 4, StatusInProgress, TierP3},
		{"canceled", 0, StatusArchived, TierP2},
		{"cancelled", 2, StatusArchived, TierP1},
		{"Backlog", 3, StatusPlanned, TierP2},
	}

	for _, s := range states {
		issue := LinearIssue{
			ID:          "ENG-99",
			Title:       "Sample",
			Description: "Desc",
			State:       s.state,
			Priority:    s.priority,
			Assignee:    "charlie",
			ProjectName: "Core Platform",
			URL:         "https://linear.app/issue/ENG-99",
			UpdatedAt:   time.Now(),
		}
		mapped := MapLinearIssueToKernel(issue)
		if mapped.Status != s.wantStatus {
			t.Errorf("state %s: got status %s, want %s", s.state, mapped.Status, s.wantStatus)
		}
		if mapped.PriorityTier != s.wantTier {
			t.Errorf("priority %d: got tier %s, want %s", s.priority, mapped.PriorityTier, s.wantTier)
		}
		if mapped.PriorityPlanRef != "PRI-CORE-PLATFORM" {
			t.Errorf("unexpected plan ref: %s", mapped.PriorityPlanRef)
		}
	}
}

func TestMapKernelToOutgoing_Matrix(t *testing.T) {
	// 1. MapKernelToGitHub
	completeItem := &BacklogItemSyncData{
		Title:            "Completed item",
		ProblemStatement: "Was an issue",
		Status:           StatusComplete,
		PriorityTier:     TierP0,
		ExternalMetadata: map[string]string{metaKeyLabels: "bug,P0"},
	}
	gh := MapKernelToGitHub(completeItem)
	if gh.State != "closed" {
		t.Errorf("expected closed state, got %s", gh.State)
	}

	// 2. MapKernelToLinear states and priorities
	testCases := []struct {
		status   string
		tier     string
		wantLin  string
		wantPrio int
	}{
		{StatusComplete, TierP0, "Done", 1},
		{StatusInProgress, TierP1, "In Progress", 2},
		{StatusTesting, TierP2, "In Progress", 3},
		{StatusArchived, TierP3, "Canceled", 4},
		{StatusPlanned, TierP4, "Todo", 4},
		{"other", "P9", "Todo", 3},
	}

	for _, tc := range testCases {
		item := &BacklogItemSyncData{
			Title:        "Item",
			Description:  "Body",
			Status:       tc.status,
			PriorityTier: tc.tier,
			ExternalID:   "linear-XYZ-1",
		}
		lin := MapKernelToLinear(item)
		if lin.State != tc.wantLin {
			t.Errorf("status %s: got state %s, want %s", tc.status, lin.State, tc.wantLin)
		}
		if lin.Priority != tc.wantPrio {
			t.Errorf("tier %s: got priority %d, want %d", tc.tier, lin.Priority, tc.wantPrio)
		}
		if lin.ID != "XYZ-1" {
			t.Errorf("expected lin.ID XYZ-1, got %s", lin.ID)
		}
	}

	// Non-prefixed external ID
	itemNoPrefix := &BacklogItemSyncData{
		Title:      "Item",
		ExternalID: "plain-id",
	}
	lin2 := MapKernelToLinear(itemNoPrefix)
	if lin2.ID != "" {
		t.Errorf("expected empty linID for non-linear prefix, got %s", lin2.ID)
	}
}

func TestIsEquivalent_Matrix(t *testing.T) {
	if isEquivalent(nil, nil) {
		t.Error("expected false for nil, nil")
	}
	itemA := &BacklogItemSyncData{
		Title:           "T1",
		Description:     "D1",
		Status:          StatusPlanned,
		Priority:        PriorityMedium,
		PriorityTier:    TierP2,
		PriorityPlanRef: "PRI-1",
	}
	if isEquivalent(itemA, nil) || isEquivalent(nil, itemA) {
		t.Error("expected false for nil comparisons")
	}

	itemB := *itemA
	if !isEquivalent(itemA, &itemB) {
		t.Error("expected identical items to be equivalent")
	}

	diffs := []func(*BacklogItemSyncData){
		func(i *BacklogItemSyncData) { i.Title = "diff" },
		func(i *BacklogItemSyncData) { i.Description = "diff" },
		func(i *BacklogItemSyncData) { i.Status = "diff" },
		func(i *BacklogItemSyncData) { i.Priority = "diff" },
		func(i *BacklogItemSyncData) { i.PriorityTier = "diff" },
		func(i *BacklogItemSyncData) { i.PriorityPlanRef = "diff" },
	}

	for _, mutate := range diffs {
		copyItem := *itemA
		mutate(&copyItem)
		if isEquivalent(itemA, &copyItem) {
			t.Error("expected mutated item not to be equivalent")
		}
	}
}

func TestSyncEngine_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	// 1. IngestGitHub unconfigured client
	engNoGH := NewSyncEngine(nil, nil, nil)
	_, err := engNoGH.IngestGitHub(ctx)
	if err == nil {
		t.Fatal("expected error for unconfigured gh client")
	}

	// IngestGitHub list error
	failingGH := &failingGHClient{listErr: errors.New("gh list error")}
	engGHErr := NewSyncEngine(failingGH, nil, nil)
	_, err = engGHErr.IngestGitHub(ctx)
	if err == nil {
		t.Fatal("expected error when gh list fails")
	}

	// IngestGitHub store query error
	storeErr := &failingStore{getErr: errors.New("db read fail")}
	engStoreErr := NewSyncEngine(&failingGHClient{}, nil, storeErr)
	_, err = engStoreErr.IngestGitHub(ctx)
	if err == nil {
		t.Fatal("expected error when store query fails")
	}

	// IngestGitHub store upsert error
	storeUpsertErr := &failingStore{upsertErr: errors.New("db insert fail")}
	engUpsertErr := NewSyncEngine(&failingGHClient{}, nil, storeUpsertErr)
	_, err = engUpsertErr.IngestGitHub(ctx)
	if err == nil {
		t.Fatal("expected error when store upsert fails")
	}

	// 2. IngestLinear unconfigured client
	engNoLin := NewSyncEngine(nil, nil, nil)
	_, err = engNoLin.IngestLinear(ctx)
	if err == nil {
		t.Fatal("expected error for unconfigured linear client")
	}

	// IngestLinear list error
	failingLin := &failingLinearClient{listErr: errors.New("linear list error")}
	engLinErr := NewSyncEngine(nil, failingLin, nil)
	_, err = engLinErr.IngestLinear(ctx)
	if err == nil {
		t.Fatal("expected error when linear list fails")
	}

	// IngestLinear store upsert error
	engLinUpsertErr := NewSyncEngine(nil, &failingLinearClient{}, storeUpsertErr)
	_, err = engLinUpsertErr.IngestLinear(ctx)
	if err == nil {
		t.Fatal("expected error when linear store upsert fails")
	}

	// 3. PushKernelToGitHub errors
	_, err = engNoGH.PushKernelToGitHub(ctx)
	if err == nil {
		t.Fatal("expected error for unconfigured gh client in push")
	}

	engStoreListErr := NewSyncEngine(&failingGHClient{}, nil, &failingStore{listErr: errors.New("list failed")})
	_, err = engStoreListErr.PushKernelToGitHub(ctx)
	if err == nil {
		t.Fatal("expected error when listBacklogItems fails in push")
	}

	// Push with invalid external ID format (skipped)
	skippedStore := &failingStore{
		items: []*BacklogItemSyncData{
			{ExternalSource: SourceGitHub, ExternalID: "gh-invalid"},
			{ExternalSource: SourceGitHub, ExternalID: "gh--5"},
			{ExternalSource: SourceLinear, ExternalID: "linear-1"},
		},
	}
	engSkip := NewSyncEngine(&failingGHClient{}, nil, skippedStore)
	stats, err := engSkip.PushKernelToGitHub(ctx)
	if err != nil || stats.PushedCount != 0 {
		t.Fatalf("expected 0 pushed and no error, got %v / %d", err, stats.PushedCount)
	}

	// Push update error
	validStore := &failingStore{
		items: []*BacklogItemSyncData{
			{ExternalSource: SourceGitHub, ExternalID: "gh-44"},
		},
	}
	engPushErr := NewSyncEngine(&failingGHClient{updateErr: errors.New("gh update failed")}, nil, validStore)
	_, err = engPushErr.PushKernelToGitHub(ctx)
	if err == nil {
		t.Fatal("expected error when gh update fails")
	}

	// 4. PushKernelToLinear errors
	_, err = engNoLin.PushKernelToLinear(ctx)
	if err == nil {
		t.Fatal("expected error for unconfigured linear client in push")
	}

	engLinStoreListErr := NewSyncEngine(nil, &failingLinearClient{}, &failingStore{listErr: errors.New("list failed")})
	_, err = engLinStoreListErr.PushKernelToLinear(ctx)
	if err == nil {
		t.Fatal("expected error when listBacklogItems fails in push linear")
	}

	// Push linear with empty ID
	linSkipStore := &failingStore{
		items: []*BacklogItemSyncData{
			{ExternalSource: SourceLinear, ExternalID: "linear-"},
			{ExternalSource: SourceGitHub, ExternalID: "gh-1"},
		},
	}
	engLinSkip := NewSyncEngine(nil, &failingLinearClient{}, linSkipStore)
	statsLin, err := engLinSkip.PushKernelToLinear(ctx)
	if err != nil || statsLin.PushedCount != 0 {
		t.Fatalf("expected 0 pushed linear and no error, got %v / %d", err, statsLin.PushedCount)
	}

	// Push linear update error
	validLinStore := &failingStore{
		items: []*BacklogItemSyncData{
			{ExternalSource: SourceLinear, ExternalID: "linear-ENG-1"},
		},
	}
	engLinPushErr := NewSyncEngine(nil, &failingLinearClient{updateErr: errors.New("lin update failed")}, validLinStore)
	_, err = engLinPushErr.PushKernelToLinear(ctx)
	if err == nil {
		t.Fatal("expected error when linear update fails")
	}
}

type errStorageProvider struct {
	storage.ObjectStorageProvider
	listErr   error
	createErr error
	updateErr error
}

func (e *errStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if e.listErr != nil {
		return nil, e.listErr
	}
	return &storage.QueryResult{}, nil
}

func (e *errStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if id == "existing-id" {
		return map[string]any{objects.FieldKeyID: "existing-id"}, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (e *errStorageProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return e.createErr
}

func (e *errStorageProvider) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return e.updateErr
}

func TestKernelSyncStore_EdgeCases(t *testing.T) {
	ctx := context.Background()

	// 1. Nil storage provider
	nilStore := NewKernelSyncStore(nil, nil)
	if items, err := nilStore.ListBacklogItems(ctx); err != nil || items != nil {
		t.Errorf("expected nil, nil for nil storage provider, got %v, %v", items, err)
	}
	if err := nilStore.UpsertBacklogItem(ctx, nil); err != nil {
		t.Errorf("expected nil error for nil item, got %v", err)
	}
	if err := nilStore.UpsertBacklogItem(ctx, &BacklogItemSyncData{}); err != nil {
		t.Errorf("expected nil error for nil storage provider, got %v", err)
	}
	if item, err := nilStore.GetBacklogItemByExternalID(ctx, SourceGitHub, "123"); err != nil || item != nil {
		t.Errorf("expected nil, nil for nil storage provider, got %v, %v", item, err)
	}

	// 2. Storage provider errors
	errSp := &errStorageProvider{listErr: errors.New("list failed")}
	errStore := NewKernelSyncStore(errSp, nil)
	if _, err := errStore.ListBacklogItems(ctx); err == nil {
		t.Error("expected error when List fails")
	}
	if _, err := errStore.GetBacklogItemByExternalID(ctx, SourceGitHub, "1"); err == nil {
		t.Error("expected error when List fails in GetBacklogItemByExternalID")
	}

	// 3. Create failure
	createErrSp := &errStorageProvider{createErr: errors.New("create failed")}
	createErrStore := NewKernelSyncStore(createErrSp, nil)
	err := createErrStore.UpsertBacklogItem(ctx, &BacklogItemSyncData{ID: "new-item", PriorityPlanRef: "PRI-1"})
	if err == nil {
		t.Error("expected create error")
	}

	// 4. Update failure
	updateErrSp := &errStorageProvider{updateErr: errors.New("update failed")}
	updateErrStore := NewKernelSyncStore(updateErrSp, nil)
	err = updateErrStore.UpsertBacklogItem(ctx, &BacklogItemSyncData{ID: "existing-id", PriorityPlanRef: "PRI-1"})
	if err == nil {
		t.Error("expected update error")
	}

	// 5. mapObjectToSyncData fallback parsing
	mockSP := newMockStorageProvider()
	realStore := NewKernelSyncStore(mockSP, nil)

	mockObj := map[string]any{
		objects.FieldKeyID:           "BLI-TEST-999",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyOriginSystem: string(SourceGitHub),
		objects.FieldKeyUpdatedAt:    "invalid-timestamp",
	}
	mockSP.items["BLI-TEST-999"] = mockObj

	found, err := realStore.GetBacklogItemByExternalID(ctx, SourceGitHub, "999")
	if err != nil || found == nil {
		t.Fatalf("expected item found, got %v, %v", found, err)
	}
	if found.ExternalID != "999" {
		t.Errorf("expected fallback external ID 999, got %s", found.ExternalID)
	}
}
