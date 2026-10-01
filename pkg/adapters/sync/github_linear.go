package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ExternalSource defines the origin provider of an external issue.
type ExternalSource string

const (
	SourceGitHub ExternalSource = "github"
	SourceLinear ExternalSource = "linear"

	// Status constants
	StatusPlanned    = "planned"
	StatusInProgress = "in_progress"
	StatusTesting    = "testing"
	StatusComplete   = "complete"
	StatusArchived   = "archived"

	// Priority constants
	PriorityCritical = "critical"
	PriorityHigh     = "high"
	PriorityMedium   = "medium"
	PriorityLow      = "low"

	// Priority Tier constants
	TierP0 = "P0"
	TierP1 = "P1"
	TierP2 = "P2"
	TierP3 = "P3"
	TierP4 = "P4"

	// Error messages
	errMsgGHClientNotConfigured     = "github client not configured"
	errMsgLinearClientNotConfigured = "linear client not configured"
	fmtErrListGitHubIssues          = "listing github issues: %w"
	fmtErrQueryKernelStore          = "querying store: %w"
	fmtErrInsertMappedItem          = "inserting mapped item: %w"
	fmtErrUpdateExistingItem        = "updating existing item: %w"
	fmtErrListLinearIssues          = "listing linear issues: %w"
	fmtErrInsertLinearItem          = "inserting linear item: %w"
	fmtErrUpdateLinearItem          = "updating existing linear item: %w"
	fmtErrListBacklogItems          = "listing backlog items: %w"
	fmtErrUpdateGitHubIssue         = "updating github issue %d: %w"
	fmtErrUpdateLinearIssue         = "updating linear issue %s: %w"

	// External metadata keys
	metaKeyNumber    = "number"
	metaKeyLabels    = "labels"
	metaKeyAssignees = "assignees"
	metaKeyLinearID  = "linear_id"
	metaKeyState     = "state"
	metaKeyAssignee  = "assignee"
	metaKeyProject   = "project"
)

// GitHubIssue models an issue retrieved from or sent to GitHub Issues API.
type GitHubIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"` // "open" | "closed"
	Labels    []string  `json:"labels,omitempty"`
	Assignees []string  `json:"assignees,omitempty"`
	HTMLURL   string    `json:"html_url"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LinearIssue models an issue retrieved from or sent to Linear GraphQL/REST API.
type LinearIssue struct {
	ID          string    `json:"id"` // e.g. "ENG-101"
	Title       string    `json:"title"`
	Description string    `json:"description"`
	State       string    `json:"state"` // "Triage" | "Backlog" | "Todo" | "In Progress" | "Done" | "Canceled"
	Assignee    string    `json:"assignee,omitempty"`
	ProjectName string    `json:"project_name,omitempty"`
	Priority    int       `json:"priority"` // 1: Urgent, 2: High, 3: Normal, 4: Low, 0: None
	URL         string    `json:"url"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BacklogItemSyncData is the kernel-compatible representation of a Backlog Item (BLI)
// synchronized with an external system.
type BacklogItemSyncData struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	ProblemStatement string            `json:"problem_statement"`
	Status           string            `json:"status"` // "planned", "in_progress", "testing", "complete", "archived"
	Priority         string            `json:"priority"`
	PriorityTier     string            `json:"priority_tier"`
	PriorityPlanRef  string            `json:"priority_plan_ref,omitempty"`
	ExternalSource   ExternalSource    `json:"external_source"`
	ExternalID       string            `json:"external_id"`
	ExternalURL      string            `json:"external_url"`
	ExternalMetadata map[string]string `json:"external_metadata"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// GitHubClient abstracts interactions with GitHub's issues API.
type GitHubClient interface {
	ListIssues(ctx context.Context) ([]GitHubIssue, error)
	CreateIssue(ctx context.Context, issue GitHubIssue) (GitHubIssue, error)
	UpdateIssue(ctx context.Context, number int, issue GitHubIssue) error
}

// LinearClient abstracts interactions with Linear's issues API.
type LinearClient interface {
	ListIssues(ctx context.Context) ([]LinearIssue, error)
	CreateIssue(ctx context.Context, issue LinearIssue) (LinearIssue, error)
	UpdateIssue(ctx context.Context, id string, issue LinearIssue) error
}

// KernelStore abstracts persistence and retrieval of kernel sync objects.
type KernelStore interface {
	GetBacklogItemByExternalID(ctx context.Context, source ExternalSource, extID string) (*BacklogItemSyncData, error)
	UpsertBacklogItem(ctx context.Context, item *BacklogItemSyncData) error
	ListBacklogItems(ctx context.Context) ([]*BacklogItemSyncData, error)
}

// SyncStats captures telemetry of an ingest or push sync cycle.
type SyncStats struct {
	IngestedCount int
	CreatedCount  int
	UpdatedCount  int
	SkippedCount  int
	ConflictCount int
	PushedCount   int
}

// SyncEngine coordinates bidirectional synchronization between external trackers and the kernel.
type SyncEngine struct {
	ghClient     GitHubClient
	linearClient LinearClient
	store        KernelStore
	mu           sync.Mutex
}

// NewSyncEngine constructs an initialized SyncEngine.
func NewSyncEngine(gh GitHubClient, lin LinearClient, store KernelStore) *SyncEngine {
	return &SyncEngine{
		ghClient:     gh,
		linearClient: lin,
		store:        store,
	}
}

// MapGitHubIssueToKernel maps a GitHubIssue into a canonical kernel BacklogItemSyncData.
func MapGitHubIssueToKernel(gh GitHubIssue) *BacklogItemSyncData {
	status := StatusPlanned
	if gh.State == "closed" {
		status = StatusComplete
	} else {
		for _, l := range gh.Labels {
			lower := strings.ToLower(l)
			if lower == "in-progress" || lower == "in progress" || lower == "active" {
				status = StatusInProgress
				break
			}
		}
	}

	priority := PriorityMedium
	priorityTier := TierP2
	for _, l := range gh.Labels {
		lower := strings.ToLower(l)
		switch {
		case strings.Contains(lower, "p0") || strings.Contains(lower, "critical"):
			priority = PriorityCritical
			priorityTier = TierP0
		case strings.Contains(lower, "p1") || strings.Contains(lower, "high"):
			priority = PriorityHigh
			priorityTier = TierP1
		case strings.Contains(lower, "p3") || strings.Contains(lower, "low"):
			priority = PriorityLow
			priorityTier = TierP3
		}
	}

	metadata := map[string]string{
		metaKeyNumber: fmt.Sprintf("%d", gh.Number),
	}
	if len(gh.Labels) > 0 {
		metadata[metaKeyLabels] = strings.Join(gh.Labels, ",")
	}
	if len(gh.Assignees) > 0 {
		metadata[metaKeyAssignees] = strings.Join(gh.Assignees, ",")
	}

	extID := fmt.Sprintf("gh-%d", gh.Number)
	return &BacklogItemSyncData{
		ID:               fmt.Sprintf("BLI-GH-%d", gh.Number),
		Title:            gh.Title,
		Description:      gh.Body,
		ProblemStatement: gh.Body,
		Status:           status,
		Priority:         priority,
		PriorityTier:     priorityTier,
		ExternalSource:   SourceGitHub,
		ExternalID:       extID,
		ExternalURL:      gh.HTMLURL,
		ExternalMetadata: metadata,
		UpdatedAt:        gh.UpdatedAt,
	}
}

// MapLinearIssueToKernel maps a LinearIssue into a canonical kernel BacklogItemSyncData.
func MapLinearIssueToKernel(lin LinearIssue) *BacklogItemSyncData {
	status := StatusPlanned
	switch strings.ToLower(lin.State) {
	case "done", "completed":
		status = StatusComplete
	case "in progress", "started":
		status = StatusInProgress
	case "canceled", "cancelled":
		status = StatusArchived
	default:
		status = StatusPlanned
	}

	priority := PriorityMedium
	priorityTier := TierP2
	switch lin.Priority {
	case 1:
		priority = PriorityCritical
		priorityTier = TierP0
	case 2:
		priority = PriorityHigh
		priorityTier = TierP1
	case 3:
		priority = PriorityMedium
		priorityTier = TierP2
	case 4:
		priority = PriorityLow
		priorityTier = TierP3
	}

	metadata := map[string]string{
		metaKeyLinearID: lin.ID,
		metaKeyState:    lin.State,
	}
	if lin.Assignee != "" {
		metadata[metaKeyAssignee] = lin.Assignee
	}
	if lin.ProjectName != "" {
		metadata[metaKeyProject] = lin.ProjectName
	}

	var planRef string
	if lin.ProjectName != "" {
		clean := strings.ToUpper(strings.ReplaceAll(lin.ProjectName, " ", "-"))
		planRef = fmt.Sprintf("PRI-%s", clean)
	}

	extID := fmt.Sprintf("linear-%s", lin.ID)
	return &BacklogItemSyncData{
		ID:               fmt.Sprintf("BLI-LIN-%s", lin.ID),
		Title:            lin.Title,
		Description:      lin.Description,
		ProblemStatement: lin.Description,
		Status:           status,
		Priority:         priority,
		PriorityTier:     priorityTier,
		PriorityPlanRef:  planRef,
		ExternalSource:   SourceLinear,
		ExternalID:       extID,
		ExternalURL:      lin.URL,
		ExternalMetadata: metadata,
		UpdatedAt:        lin.UpdatedAt,
	}
}

// MapKernelToGitHub maps a kernel BacklogItemSyncData to an outgoing GitHubIssue.
func MapKernelToGitHub(item *BacklogItemSyncData) GitHubIssue {
	state := "open"
	if item.Status == StatusComplete {
		state = "closed"
	}

	var labels []string
	if rawLabels, ok := item.ExternalMetadata[metaKeyLabels]; ok && rawLabels != "" {
		labels = strings.Split(rawLabels, ",")
	}
	if item.PriorityTier != "" {
		hasTier := false
		for _, l := range labels {
			if strings.EqualFold(l, item.PriorityTier) {
				hasTier = true
				break
			}
		}
		if !hasTier {
			labels = append(labels, item.PriorityTier)
		}
	}

	return GitHubIssue{
		Title:     item.Title,
		Body:      item.ProblemStatement,
		State:     state,
		Labels:    labels,
		HTMLURL:   item.ExternalURL,
		UpdatedAt: item.UpdatedAt,
	}
}

// MapKernelToLinear maps a kernel BacklogItemSyncData to an outgoing LinearIssue.
func MapKernelToLinear(item *BacklogItemSyncData) LinearIssue {
	state := "Todo"
	switch item.Status {
	case StatusComplete:
		state = "Done"
	case StatusInProgress, StatusTesting:
		state = "In Progress"
	case StatusArchived:
		state = "Canceled"
	case StatusPlanned:
		state = "Todo"
	}

	priority := 3
	switch item.PriorityTier {
	case TierP0:
		priority = 1
	case TierP1:
		priority = 2
	case TierP2:
		priority = 3
	case TierP3, TierP4:
		priority = 4
	}

	var linID string
	if strings.HasPrefix(item.ExternalID, "linear-") {
		linID = strings.TrimPrefix(item.ExternalID, "linear-")
	}

	return LinearIssue{
		ID:          linID,
		Title:       item.Title,
		Description: item.Description,
		State:       state,
		Priority:    priority,
		URL:         item.ExternalURL,
		UpdatedAt:   item.UpdatedAt,
	}
}

// IngestGitHub synchronizes all issues from GitHub into the kernel store.
// It resolves conflicts idempotently using monotonic timestamp resolution.
func (e *SyncEngine) IngestGitHub(ctx context.Context) (SyncStats, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var stats SyncStats
	if e.ghClient == nil {
		return stats, errors.New(errMsgGHClientNotConfigured)
	}

	issues, err := e.ghClient.ListIssues(ctx)
	if err != nil {
		return stats, fmt.Errorf(fmtErrListGitHubIssues, err)
	}

	for _, gh := range issues {
		stats.IngestedCount++
		mapped := MapGitHubIssueToKernel(gh)

		existing, err := e.store.GetBacklogItemByExternalID(ctx, SourceGitHub, mapped.ExternalID)
		if err != nil {
			return stats, fmt.Errorf(fmtErrQueryKernelStore, err)
		}

		if existing == nil {
			if err := e.store.UpsertBacklogItem(ctx, mapped); err != nil {
				return stats, fmt.Errorf(fmtErrInsertMappedItem, err)
			}
			stats.CreatedCount++
			continue
		}

		// Check idempotency: if no substantive changes, skip.
		if isEquivalent(existing, mapped) {
			stats.SkippedCount++
			continue
		}

		// Conflict resolution: last-write-wins by UpdatedAt.
		if mapped.UpdatedAt.Before(existing.UpdatedAt) {
			stats.ConflictCount++
			stats.SkippedCount++
			continue
		}

		// Preserve internal kernel properties if set
		if existing.ID != "" {
			mapped.ID = existing.ID
		}
		if err := e.store.UpsertBacklogItem(ctx, mapped); err != nil {
			return stats, fmt.Errorf(fmtErrUpdateExistingItem, err)
		}
		stats.UpdatedCount++
	}

	return stats, nil
}

// IngestLinear synchronizes all issues from Linear into the kernel store.
func (e *SyncEngine) IngestLinear(ctx context.Context) (SyncStats, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var stats SyncStats
	if e.linearClient == nil {
		return stats, errors.New(errMsgLinearClientNotConfigured)
	}

	issues, err := e.linearClient.ListIssues(ctx)
	if err != nil {
		return stats, fmt.Errorf(fmtErrListLinearIssues, err)
	}

	for _, lin := range issues {
		stats.IngestedCount++
		mapped := MapLinearIssueToKernel(lin)

		existing, err := e.store.GetBacklogItemByExternalID(ctx, SourceLinear, mapped.ExternalID)
		if err != nil {
			return stats, fmt.Errorf(fmtErrQueryKernelStore, err)
		}

		if existing == nil {
			if err := e.store.UpsertBacklogItem(ctx, mapped); err != nil {
				return stats, fmt.Errorf(fmtErrInsertLinearItem, err)
			}
			stats.CreatedCount++
			continue
		}

		if isEquivalent(existing, mapped) {
			stats.SkippedCount++
			continue
		}

		if mapped.UpdatedAt.Before(existing.UpdatedAt) {
			stats.ConflictCount++
			stats.SkippedCount++
			continue
		}

		if existing.ID != "" {
			mapped.ID = existing.ID
		}
		if err := e.store.UpsertBacklogItem(ctx, mapped); err != nil {
			return stats, fmt.Errorf(fmtErrUpdateLinearItem, err)
		}
		stats.UpdatedCount++
	}

	return stats, nil
}

// PushKernelToGitHub pushes updated kernel backlog items originating from GitHub back to GitHub.
func (e *SyncEngine) PushKernelToGitHub(ctx context.Context) (SyncStats, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var stats SyncStats
	if e.ghClient == nil {
		return stats, errors.New(errMsgGHClientNotConfigured)
	}

	items, err := e.store.ListBacklogItems(ctx)
	if err != nil {
		return stats, fmt.Errorf(fmtErrListBacklogItems, err)
	}

	for _, item := range items {
		if item.ExternalSource != SourceGitHub {
			continue
		}

		var num int
		n, scanErr := fmt.Sscanf(item.ExternalID, "gh-%d", &num)
		if scanErr != nil || n != 1 || num <= 0 {
			continue
		}

		ghPayload := MapKernelToGitHub(item)
		if err := e.ghClient.UpdateIssue(ctx, num, ghPayload); err != nil {
			return stats, fmt.Errorf(fmtErrUpdateGitHubIssue, num, err)
		}
		stats.PushedCount++
	}

	return stats, nil
}

// PushKernelToLinear pushes updated kernel backlog items originating from Linear back to Linear.
func (e *SyncEngine) PushKernelToLinear(ctx context.Context) (SyncStats, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var stats SyncStats
	if e.linearClient == nil {
		return stats, errors.New(errMsgLinearClientNotConfigured)
	}

	items, err := e.store.ListBacklogItems(ctx)
	if err != nil {
		return stats, fmt.Errorf(fmtErrListBacklogItems, err)
	}

	for _, item := range items {
		if item.ExternalSource != SourceLinear {
			continue
		}

		linID := strings.TrimPrefix(item.ExternalID, "linear-")
		if linID == "" {
			continue
		}

		linPayload := MapKernelToLinear(item)
		if err := e.linearClient.UpdateIssue(ctx, linID, linPayload); err != nil {
			return stats, fmt.Errorf(fmtErrUpdateLinearIssue, linID, err)
		}
		stats.PushedCount++
	}

	return stats, nil
}

func isEquivalent(a, b *BacklogItemSyncData) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Title == b.Title &&
		a.Description == b.Description &&
		a.Status == b.Status &&
		a.Priority == b.Priority &&
		a.PriorityTier == b.PriorityTier &&
		a.PriorityPlanRef == b.PriorityPlanRef
}
