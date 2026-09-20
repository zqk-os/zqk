package git

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// CommitIntegrationService integrates Git commits with work items
type CommitIntegrationService struct {
	analyzer      *CommitAnalyzer
	storage       storage.ObjectStorageProvider
	secCtx        *pkgctx.SecurityContext
	logger        *logging.EventLogger
	linkTimeout   time.Duration
	maxConcurrent int
	mu            sync.Mutex
}

// NewCommitIntegrationService creates a new commit integration service
func NewCommitIntegrationService(
	repoPath string,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
) *CommitIntegrationService {
	return &CommitIntegrationService{
		analyzer:      NewCommitAnalyzer(repoPath),
		storage:       storageProvider,
		secCtx:        secCtx,
		logger:        logging.NewEventLogger(pkgctx.NewSystemContext()), // Long-lived component, use system context
		linkTimeout:   2 * time.Minute,                                   // Default timeout for linking operations
		maxConcurrent: 10,                                                // Max concurrent link operations
	}
}

// SetLogger sets the logger for the integration service
func (cis *CommitIntegrationService) SetLogger(logger *logging.EventLogger) {
	_ = concurrency.RunInLockWithLogger(
		&cis.mu, LockNameGitIntegrationSetLogger, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			cis.logger = logger
			return nil
		},
	)
}

// LinkCommitToWorkItems links a commit to work items based on extracted references
// Uses non-blocking operations with timeout and retry support
func (cis *CommitIntegrationService) LinkCommitToWorkItems(ctx context.Context, commit *Commit) error {
	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, cis.linkTimeout)
	defer cancel()

	// Use channel for non-blocking error collection
	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("git_commit_linker", "linking commit to work items").
		StartSimple(func() {
			errChan <- cis.linkCommitToWorkItemsSync(timeoutCtx, commit)
		})

	// Wait for completion or timeout
	select {
	case err := <-errChan:
		return err
	case <-timeoutCtx.Done():
		RecordTimeout()
		RecordLinking(cis.linkTimeout, false)
		return errfmt.Errorf("linking commit %s timed out after %v", commit.ShortHash, cis.linkTimeout)
	}
}

// linkCommitToWorkItemsSync performs the actual linking (synchronous)
func (cis *CommitIntegrationService) linkCommitToWorkItemsSync(ctx context.Context, commit *Commit) error {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		RecordLinking(duration, true) // Will be updated if error occurs
	}()
	// Create code_reference objects for each file changed (non-blocking batch)
	type codeRefResult struct {
		codeRef map[string]any
		err     error
	}

	refChan := make(chan codeRefResult, len(commit.FilesChanged))
	var wg sync.WaitGroup

	for _, fileChange := range commit.FilesChanged {
		fc := fileChange
		goroutinelabels.NewGoroutine("git_code_reference_creator", fmt.Sprintf("creating code reference for %s", fc.Path)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				ref, err := cis.createCodeReference(ctx, commit, fc)
				refChan <- codeRefResult{codeRef: ref, err: err}
			})
	}

	// Wait for all code references to be created
	goroutinelabels.NewGoroutine("git_ref_collector", "collecting git reference results").
		WithCleanup(func() {
			close(refChan)
		}).
		StartSimple(func() {
			wg.Wait()
		})

	// Collect results
	var firstErr error
	for result := range refChan {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			// Continue processing other files
		}
	}

	if firstErr != nil {
		return errfmt.Newf("failed to create code references").Wrap(firstErr)
	}

	// Update work items with commit references
	if err := cis.updateWorkItemsWithCommit(ctx, commit); err != nil {
		return errfmt.Newf("failed to update work items").Wrap(err)
	}

	return nil
}

// createCodeReference creates a code reference for a file change
func (cis *CommitIntegrationService) createCodeReference(ctx context.Context, commit *Commit, fileChange FileChange) (map[string]any, error) {
	// Generate ID following pattern: COD-\d{3,} (e.g., COD-100, COD-123456)
	// Use a numeric suffix based on commit hash and file path hash
	// Use uint64 to avoid overflow issues
	pathHash := uint64(0)
	for i := 0; i < len(fileChange.Path); i++ {
		pathHash = pathHash*31 + uint64(fileChange.Path[i])
	}

	// Combine commit hash and path hash for unique numeric ID
	commitHashNum := uint64(0)
	for i := 0; i < len(commit.Hash); i++ {
		commitHashNum = commitHashNum*31 + uint64(commit.Hash[i])
	}

	// Generate numeric ID (at least 3 digits, pattern: ^[A-Z]+-\d{3,}$)
	// Use modulo to keep it in a reasonable range, ensure minimum 3 digits
	// Range: 100-999996 (always at least 3 digits)
	combinedHash := (pathHash + commitHashNum) % 999897
	combinedHash += 100 // Ensure minimum 100 (3 digits)
	codeRefID := fmt.Sprintf("COD-%d", combinedHash)

	codeRef := map[string]any{
		objects.FieldKeyID:            codeRefID,
		objects.FieldKeyKind:          objects.KindCodeReference,
		objects.FieldKeyTitle:         fmt.Sprintf("Code reference: %s in %s", fileChange.Path, commit.ShortHash),
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		// code_reference extends base_object, whose statuses are proposed, in_progress,
		// implemented, approved, archived, error. "active" is not among them, so both the create
		// and the promote below failed validation ("lifecycle status unknown") and
		// commit-to-work-item linking never worked at all.
		//
		// proposed is stated here because it is what actually gets stored: the create path enters
		// the draft plane at proposed whatever this field asks for. Naming anything else would
		// describe a status the object never holds.
		objects.FieldKeyStatus:       objects.ObjectStatusConceptual,
		objects.FieldKeyFilePath:     fileChange.Path,
		objects.FieldKeyLineStart:    float64(1), // For now, we'll use 1 as default
		objects.FieldKeyLineEnd:      float64(fileChange.LinesAdded + fileChange.LinesRemoved),
		objects.FieldKeyCommitHash:   commit.Hash,
		objects.FieldKeyCommitDate:   commit.Date.Format(time.RFC3339),
		objects.FieldKeyAuthor:       commit.Author,
		objects.FieldKeyAuthorEmail:  commit.AuthorEmail,
		objects.FieldKeyChangeType:   fileChange.Status,
		objects.FieldKeyLinesAdded:   float64(fileChange.LinesAdded),
		objects.FieldKeyLinesRemoved: float64(fileChange.LinesRemoved),
	}

	// Add work item references
	if len(commit.BacklogItemRefs) > 0 {
		codeRef[objects.FieldKeyBacklogItemRefs] = commit.BacklogItemRefs
	}
	if len(commit.MilestoneRefs) > 0 {
		codeRef[objects.FieldKeyMilestoneRefs] = commit.MilestoneRefs
	}
	if len(commit.GoalRefs) > 0 {
		codeRef[objects.FieldKeyGoalRefs] = commit.GoalRefs
	}
	if len(commit.WorkstreamRefs) > 0 {
		codeRef[objects.FieldKeyWorkstreamRefs] = commit.WorkstreamRefs
	}
	if len(commit.RequirementRefs) > 0 {
		codeRef[objects.FieldKeyRequirementRefs] = commit.RequirementRefs
	}

	// Check if code reference already exists
	exists, err := cis.storage.Exists(ctx, cis.secCtx, codeRefID)
	if err != nil {
		return nil, errfmt.Newf("failed to check if code reference exists").Wrap(err)
	}

	if !exists {
		// Create new code reference
		if err := cis.storage.Create(ctx, cis.secCtx, codeRef); err != nil {
			return nil, errfmt.Newf("failed to create code reference").Wrap(err)
		}
		// Promote code reference from conceptual to originated (CAS membrane crossed)
		if err := cis.storage.Update(ctx, cis.secCtx, codeRefID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusOriginated}); err != nil {
			return nil, errfmt.Newf("promote code reference %s", codeRefID).Wrap(err)
		}
	} else {
		// Update existing code reference (merge references)
		existing, err := cis.storage.Read(ctx, cis.secCtx, codeRefID)
		if err != nil {
			return nil, errfmt.Newf("failed to read existing code reference").Wrap(err)
		}

		// Merge work item references
		updates := map[string]any{}
		if backlogRefs, ok := existing[objects.FieldKeyBacklogItemRefs].([]any); ok {
			updates[objects.FieldKeyBacklogItemRefs] = mergeStringLists(backlogRefs, commit.BacklogItemRefs)
		} else if len(commit.BacklogItemRefs) > 0 {
			updates[objects.FieldKeyBacklogItemRefs] = commit.BacklogItemRefs
		}
		if milestoneRefs, ok := existing[objects.FieldKeyMilestoneRefs].([]any); ok {
			updates[objects.FieldKeyMilestoneRefs] = mergeStringLists(milestoneRefs, commit.MilestoneRefs)
		} else if len(commit.MilestoneRefs) > 0 {
			updates[objects.FieldKeyMilestoneRefs] = commit.MilestoneRefs
		}
		if goalRefs, ok := existing[objects.FieldKeyGoalRefs].([]any); ok {
			updates[objects.FieldKeyGoalRefs] = mergeStringLists(goalRefs, commit.GoalRefs)
		} else if len(commit.GoalRefs) > 0 {
			updates[objects.FieldKeyGoalRefs] = commit.GoalRefs
		}
		if workstreamRefs, ok := existing[objects.FieldKeyWorkstreamRefs].([]any); ok {
			updates[objects.FieldKeyWorkstreamRefs] = mergeStringLists(workstreamRefs, commit.WorkstreamRefs)
		} else if len(commit.WorkstreamRefs) > 0 {
			updates[objects.FieldKeyWorkstreamRefs] = commit.WorkstreamRefs
		}
		if requirementRefs, ok := existing[objects.FieldKeyRequirementRefs].([]any); ok {
			updates[objects.FieldKeyRequirementRefs] = mergeStringLists(requirementRefs, commit.RequirementRefs)
		} else if len(commit.RequirementRefs) > 0 {
			updates[objects.FieldKeyRequirementRefs] = commit.RequirementRefs
		}

		if len(updates) > 0 {
			if err := cis.storage.Update(ctx, cis.secCtx, codeRefID, updates); err != nil {
				return nil, errfmt.Newf("failed to update code reference").Wrap(err)
			}
		}
	}

	return codeRef, nil
}

// updateWorkItemsWithCommit updates work items to reference the commit
func (cis *CommitIntegrationService) updateWorkItemsWithCommit(ctx context.Context, commit *Commit) error {
	// Update backlog items
	for _, bliID := range commit.BacklogItemRefs {
		if err := cis.addCommitToWorkItem(ctx, objects.KindBacklogItem, bliID, commit); err != nil {
			return errfmt.Errorf("failed to update backlog item %s: %w", bliID, err)
		}
	}

	// Update milestones
	for _, milID := range commit.MilestoneRefs {
		if err := cis.addCommitToWorkItem(ctx, objects.KindMilestone, milID, commit); err != nil {
			return errfmt.Errorf("failed to update milestone %s: %w", milID, err)
		}
	}

	// Update goals
	for _, goalID := range commit.GoalRefs {
		if err := cis.addCommitToWorkItem(ctx, objects.KindGoal, goalID, commit); err != nil {
			return errfmt.Errorf("failed to update goal %s: %w", goalID, err)
		}
	}

	return nil
}

// addCommitToWorkItem adds a commit reference to a work item
func (cis *CommitIntegrationService) addCommitToWorkItem(ctx context.Context, _, id string, commit *Commit) error {
	// Read work item
	workItem, err := cis.storage.Read(ctx, cis.secCtx, id)
	if err != nil {
		// Work item doesn't exist, skip
		return nil
	}

	// Get existing commit hashes (if any)
	commitHashes := []string{}
	existingRefs, ok := workItem[objects.FieldKeyCommitHashes].([]any)
	if ok {
		for _, ref := range existingRefs {
			if refStr, ok := ref.(string); ok {
				commitHashes = append(commitHashes, refStr)
			}
		}
	}

	// Add commit hash if not already present
	commitHash := commit.Hash
	found := false
	for _, hash := range commitHashes {
		if hash == commitHash {
			found = true
			break
		}
	}
	if !found {
		commitHashes = append(commitHashes, commitHash)
	}

	// Update work item
	updates := map[string]any{
		objects.FieldKeyCommitHashes: commitHashes,
	}

	return cis.storage.Update(ctx, cis.secCtx, id, updates)
}

// mergeStringLists merges two string lists, removing duplicates
func mergeStringLists(existing []any, newItems []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	// Add existing
	for _, item := range existing {
		if str, ok := item.(string); ok {
			if !seen[str] {
				seen[str] = true
				result = append(result, str)
			}
		}
	}

	// Add new
	for _, item := range newItems {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// sanitizeForID sanitizes a string for use in an ID
//
//nolint:unused // Helper function - reserved for future use
func sanitizeForID(s string) string {
	// Replace invalid characters with dashes
	result := ""
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			result += string(r)
		} else {
			result += "-"
		}
	}
	// Limit length
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}
