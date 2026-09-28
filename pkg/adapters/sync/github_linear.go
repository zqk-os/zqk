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
	ghClient    GitHubClient
	linearClient LinearClient
	store       KernelStore
	mu          sync.Mutex
}

// NewSyncEngine constructs an initialized SyncEngine.
func NewSyncEngine(gh GitHubClient, lin LinearClient, store KernelStore) *SyncEngine {
	return &SyncEngine{
		ghClient:    gh,
		linearClient: lin,
		store:       store,
	}
}

// MapGitHubIssueToKernel maps a GitHubIssue into a canonical kernel BacklogItemSyncData.
func MapGitHubIssueToKernel(gh GitHubIssue) *BacklogItemSyncData {
	status := "planned"
	if gh.State == "closed" {
		status = "complete"
	} else {
		for _, l := range gh.Labels {
			lower := strings.ToLower(l)
			if lower == "in-progress" || lower == "in progress" || lower == "active" {
				status = "in_progress"
				break
			}
		}
	}

	priority := "medium"
	priorityTier := "P2"
	for _, l := range gh.Labels {
		lower := strings.ToLower(l)
		if strings.Contains(lower, "p0") || strings.Contains(lower, "critical") {
			priority = "critical"
			priorityTier = "P0"
			break
		} else if strings.Contains(lower, "p1") || strings.Contains(lower, "high") {
			priority = "high"
			priorityTier = "P1"
			break
		} else if strings.Contains(lower, "p3") || strings.Contains(lower, "low") {
			priority = "low"
			priorityTier = "P3"
			break
		}
	}

	metadata := map[string]string{
		"number": fmt.Sprintf("%d", gh.Number),
	}
	if len(gh.Labels) > 0 {
		metadata["labels"] = strings.Join(gh.Labels, ",")
	}
	if len(gh.Assignees) > 0 {
		metadata["assignees"] = strings.Join(gh.Assignees, ",")
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
	status := "planned"
	switch strings.ToLower(lin.State) {
	case "done", "completed":
		status = "complete"
	case "in progress", "started":
		status = "in_progress"
	case "canceled", "cancelled":
		status = "archived"
	default:
		status = "planned"
	}

	priority := "medium"
	priorityTier := "P2"
	switch lin.Priority {
	case 1:
		priority = "critical"
		priorityTier = "P0"
	case 2:
		priority = "high"
		priorityTier = "P1"
	case 3:
		priority = "medium"
		priorityTier = "P2"
	case 4:
		priority = "low"
		priorityTier = "P3"
	}

	metadata := map[string]string{
		"linear_id": lin.ID,
		"state":     lin.State,
	}
	if lin.Assignee != "" {
		metadata["assignee"] = lin.Assignee
	}
	if lin.ProjectName != "" {
		metadata["project"] = lin.ProjectName
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
	if item.Status == "complete" {
		state = "closed"
	}

	var labels []string
	if rawLabels, ok := item.ExternalMetadata["labels"]; ok && rawLabels != "" {
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
	case "complete":
		state = "Done"
	case "in_progress", "testing":
		state = "In Progress"
	case "archived":
		state = "Canceled"
	case "planned":
		state = "Todo"
	}

	priority := 3
	switch item.PriorityTier {
	case "P0":
		priority = 1
	case "P1":
		priority = 2
	case "P2":
		priority = 3
	case "P3", "P4":
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
		return stats, errors.New("github client not configured")
	}

	issues, err := e.ghClient.ListIssues(ctx)
	if err != nil {
		return stats, fmt.Errorf("listing github issues: %w", err)
	}

	for _, gh := range issues {
		stats.IngestedCount++
		mapped := MapGitHubIssueToKernel(gh)

		existing, err := e.store.GetBacklogItemByExternalID(ctx, SourceGitHub, mapped.ExternalID)
		if err != nil {
			return stats, fmt.Errorf("querying store: %w", err)
		}

		if existing == nil {
			if err := e.store.UpsertBacklogItem(ctx, mapped); err != nil {
				return stats, fmt.Errorf("inserting mapped item: %w", err)
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
			return stats, fmt.Errorf("updating existing item: %w", err)
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
		return stats, errors.New("linear client not configured")
	}

	issues, err := e.linearClient.ListIssues(ctx)
	if err != nil {
		return stats, fmt.Errorf("listing linear issues: %w", err)
	}

	for _, lin := range issues {
		stats.IngestedCount++
		mapped := MapLinearIssueToKernel(lin)

		existing, err := e.store.GetBacklogItemByExternalID(ctx, SourceLinear, mapped.ExternalID)
		if err != nil {
			return stats, fmt.Errorf("querying store: %w", err)
		}

		if existing == nil {
			if err := e.store.UpsertBacklogItem(ctx, mapped); err != nil {
				return stats, fmt.Errorf("inserting linear item: %w", err)
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
			return stats, fmt.Errorf("updating existing linear item: %w", err)
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
		return stats, errors.New("github client not configured")
	}

	items, err := e.store.ListBacklogItems(ctx)
	if err != nil {
		return stats, fmt.Errorf("listing backlog items: %w", err)
	}

	for _, item := range items {
		if item.ExternalSource != SourceGitHub {
			continue
		}

		var num int
		if _, scanErr := fmt.Sscanf(item.ExternalID, "gh-%d", &num); scanErr != nil || num <= 0 {
			continue
		}

		ghPayload := MapKernelToGitHub(item)
		if err := e.ghClient.UpdateIssue(ctx, num, ghPayload); err != nil {
			return stats, fmt.Errorf("updating github issue %d: %w", num, err)
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
		return stats, errors.New("linear client not configured")
	}

	items, err := e.store.ListBacklogItems(ctx)
	if err != nil {
		return stats, fmt.Errorf("listing backlog items: %w", err)
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
			return stats, fmt.Errorf("updating linear issue %s: %w", linID, err)
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
