package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/pkg/objects"
)

type contextKey string

const lockContextKey contextKey = "branchLock"

// ProvisionObjectBranchOptions holds configuration for ProvisionObjectBranchStage
type ProvisionObjectBranchOptions struct {
	// ObjectReader provides a way to read an object without depending on pkg/storage
	ObjectReader func(ctx context.Context, id string) (map[string]any, error)
	ProjectRoot  string
	// TryLock attempts to acquire a file-based or process-based lock for the payload ID.
	// Returns a release function, a bool indicating if lock was acquired, and an error.
	// Injected by caller to avoid import cycle between pkg/pipeline and pkg/storage.
	TryLock func(ctx context.Context, id string) (release func() error, acquired bool, err error)
}

// ProvisionObjectBranchStage returns a StageFunc that natively provisions git worktrees
// and enforces strict pre-flight branch execution locks based on payload IDs.
func ProvisionObjectBranchStage(opts ProvisionObjectBranchOptions) StageFunc {
	return func(ctx *Context, payload any) (any, error) {
		// Extract target ID from payload
		var targetID string
		switch p := payload.(type) {
		case *Envelope:
			if strID, ok := p.Payload.(string); ok {
				targetID = strID
			} else if mapPayload, ok := p.Payload.(map[string]any); ok {
				if id, ok := mapPayload[objects.FieldKeyID].(string); ok {
					targetID = id
				}
			}
		case map[string]any:
			if id, ok := p[objects.FieldKeyID].(string); ok {
				targetID = id
			}
		case string:
			targetID = p
		}

		if targetID == "" {
			return nil, fmt.Errorf("ProvisionObjectBranchStage: could not extract target object ID from payload")
		}

		lowerID := strings.ToLower(targetID)

		// 1. Try to acquire the execution lock if TryLock is provided
		if opts.TryLock != nil {
			release, acquired, err := opts.TryLock(ctx.Ctx, targetID)
			if err != nil {
				return nil, fmt.Errorf("ProvisionObjectBranchStage: failed to acquire lock: %w", err)
			}
			if !acquired {
				return nil, fmt.Errorf("ProvisionObjectBranchStage: branch execution lock for %s is already held by another process", targetID)
			}
			// Store the release function in the context context so it can be called by ReleaseObjectBranchStage
			ctx.Ctx = context.WithValue(ctx.Ctx, lockContextKey, release)
		}

		baseAnchor, err := resolveBaseAnchor(ctx.Ctx, opts.ObjectReader, targetID)
		if err != nil {
			// Release lock if error occurs
			if lockVal := ctx.Ctx.Value(lockContextKey); lockVal != nil {
				if release, ok := lockVal.(func() error); ok {
					_ = release()
				}
			}
			return nil, fmt.Errorf("ProvisionObjectBranchStage: failed to resolve base anchor: %w", err)
		}

		var branchName string
		if strings.HasPrefix(lowerID, "tsk-") || strings.HasPrefix(lowerID, "bli-") || strings.HasPrefix(lowerID, "hot-") {
			prefix := strings.Split(lowerID, "-")[0]
			branchName = fmt.Sprintf("%s/%s", prefix, lowerID)
		} else {
			branchName = fmt.Sprintf("task/tsk-%s", lowerID)
			lowerID = "tsk-" + lowerID
		}

		// 2. Fetch latest from origin to ensure we know about remote branches
		fetchCmd := execwrap.CommandContext(ctx.Ctx, "git", "fetch", "origin")
		fetchCmd.Dir = opts.ProjectRoot
		_ = fetchCmd.Run() // Ignore errors, it might be an offline environment

		worktreeDir := filepath.Join(opts.ProjectRoot, "..", fmt.Sprintf("zqk-%s", lowerID))

		// 3. Check if branch exists locally or remotely
		branchExists := false
		checkCmd := execwrap.CommandContext(ctx.Ctx, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
		checkCmd.Dir = opts.ProjectRoot
		if err := checkCmd.Run(); err == nil {
			branchExists = true
		} else {
			// Check remote
			checkRemCmd := execwrap.CommandContext(ctx.Ctx, "git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branchName)
			checkRemCmd.Dir = opts.ProjectRoot
			if err := checkRemCmd.Run(); err == nil {
				branchExists = true
			}
		}

		// 4. Provision worktree
		var cmd *exec.Cmd
		if branchExists {
			// Branch exists, just check it out
			cmd = execwrap.CommandContext(ctx.Ctx, "git", "worktree", "add", worktreeDir, branchName)
		} else {
			// Branch does not exist, create new from baseAnchor
			cmd = execwrap.CommandContext(ctx.Ctx, "git", "worktree", "add", worktreeDir, "-b", branchName, baseAnchor)
		}

		cmd.Dir = opts.ProjectRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(output), "already exists") {
				// Release lock if error occurs
				if lockVal := ctx.Ctx.Value(lockContextKey); lockVal != nil {
					if release, ok := lockVal.(func() error); ok {
						_ = release()
					}
				}
				return nil, fmt.Errorf("ProvisionObjectBranchStage: failed to provision worktree %s (branch %s, anchor %s): %s, %v", worktreeDir, branchName, baseAnchor, string(output), err)
			}
		}

		return payload, nil
	}
}

// ReleaseObjectBranchStage returns a StageFunc that releases the branch execution lock stored in the context
func ReleaseObjectBranchStage() StageFunc {
	return func(ctx *Context, payload any) (any, error) {
		if lockVal := ctx.Ctx.Value(lockContextKey); lockVal != nil {
			if release, ok := lockVal.(func() error); ok {
				_ = release()
			}
		}
		return payload, nil
	}
}

func resolveBaseAnchor(ctx context.Context, reader func(context.Context, string) (map[string]any, error), currentID string) (string, error) {
	if reader == nil {
		return "main", nil
	}
	visited := make(map[string]bool)

	for currentID != "" {
		if visited[currentID] {
			break
		}
		visited[currentID] = true

		obj, err := reader(ctx, currentID)
		if err != nil {
			return "", err
		}

		kind, _ := obj[objects.FieldKeyKind].(string)

		if kind == "hotfix" {
			return "main", nil
		}

		if kind == "priority_plan" {
			status, _ := obj[objects.FieldKeyStatus].(string)
			if isTerminal, _ := objects.GetGlobalLifecycleLoader().IsTerminalStatusForKind("priority_plan", status); isTerminal {
				return "main", nil
			}
			return fmt.Sprintf("integration/%s", strings.ToLower(currentID)), nil
		}

		if priorityPlanRef, ok := obj[objects.FieldKeyPriorityPlanRef].(string); ok && priorityPlanRef != "" {
			currentID = priorityPlanRef
			continue
		}
		if workstreamRef, ok := obj[objects.FieldKeyWorkstreamRef].(string); ok && workstreamRef != "" {
			currentID = workstreamRef
			continue
		}
		if backlogItemRef, ok := obj["backlog_item_ref"].(string); ok && backlogItemRef != "" {
			currentID = backlogItemRef
			continue
		}

		return "main", nil
	}

	return "main", nil
}
