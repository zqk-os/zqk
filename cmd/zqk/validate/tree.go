package validate

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type workflowStorage interface {
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error)
	Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error)
	Update(ctx context.Context, secCtx *storage.SecurityContext, id string, data map[string]any) error
}

// TreeNode represents a node in the dependency tree.
type TreeNode struct {
	ID       string      `json:"id" yaml:"id"`
	Type     string      `json:"type" yaml:"type"`
	Title    string      `json:"title" yaml:"title"`
	Status   string      `json:"status" yaml:"status"`
	Children []*TreeNode `json:"children,omitempty" yaml:"children,omitempty"`
}

// DependencyTreeResolver resolves and builds the dependency tree for a priority plan.
type DependencyTreeResolver struct {
	sp workflowStorage
}

// NewDependencyTreeResolver creates a new resolver instance.
func NewDependencyTreeResolver(sp workflowStorage) *DependencyTreeResolver {
	return &DependencyTreeResolver{sp: sp}
}

// NewTreeCmd returns workflow tree command.
func NewTreeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowTreeCommandBuilder()
	cli.BindAsyncProgress(cmd, runTree)
	cli.RequireStorage(cmd, true)
	return cmd
}

func runTree(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		sp := proc.Storage()

		var flags clipkg.FlagBag
		priFlag := flags.String(cmd, "priority-plan")
		depth := flags.Int(cmd, "depth")
		if err := flags.Err(); err != nil {
			return err
		}
		_ = depth

		planID, err := resolvePriorityPlan(ctx, sp, strings.TrimSpace(priFlag))
		if err != nil {
			return err
		}

		resolver := NewDependencyTreeResolver(sp)
		rootNode, err := resolver.Resolve(ctx, planID)
		if err != nil {
			return err
		}

		// Handle format output
		format := proc.Format()
		if format == cli.FormatJSON || format == cli.FormatJSONL || format == cli.FormatYAML {
			return cli.FormatOutput(cmd, rootNode)
		}

		// Default friendly tree-text view
		var builder strings.Builder
		printTree(&builder, rootNode, "", true, 0, depth)
		return cli.WriteOutput(cmd, []byte(builder.String()))
	})(cmd, args)
}

type treeEntities struct {
	backlogs []map[string]any
	goals    []map[string]any
	reqs     []map[string]any
	crits    []map[string]any
}

// Resolve fetches all related items and constructs the dependency tree.
func (r *DependencyTreeResolver) Resolve(ctx context.Context, planID string) (*TreeNode, error) {
	planObj, entities, err := r.fetchTreeEntities(ctx, planID)
	if err != nil {
		return nil, err
	}
	return assembleTree(planID, planObj, entities), nil
}

func (r *DependencyTreeResolver) fetchTreeEntities(ctx context.Context, planID string) (map[string]any, *treeEntities, error) {
	secCtx := pkgctx.NewSystemSecurityContext()

	// Read the priority plan
	planObj, err := r.sp.Read(ctx, secCtx, planID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read priority plan %s: %w", planID, err)
	}
	if planObj == nil {
		return nil, nil, fmt.Errorf("priority plan %s not found", planID)
	}

	backlogRes, err := r.sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyPriorityPlanRef: planID,
			objects.FieldKeyStatus: map[string]any{
				"$ne": objects.ObjectStatusArchived,
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyPriorityPlanRef, "requirement_ref", objects.FieldKeyRequirementRefs, "goal_ref", objects.FieldKeyGoalRefs},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list backlog items: %w", err)
	}

	goalRes, err := r.sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: objects.KindGoal,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$ne": objects.ObjectStatusArchived,
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyPriorityPlanRef, objects.FieldKeyPriorityPlanRefs},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list goals: %w", err)
	}

	reqRes, err := r.sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: objects.KindRequirement,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$ne": objects.ObjectStatusArchived,
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, "goal_ref", objects.FieldKeyGoalRefs},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list requirements: %w", err)
	}

	critRes, err := r.sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: objects.KindCriteria,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$ne": objects.ObjectStatusArchived,
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyBacklogItemRef, "requirement_ref", objects.FieldKeyRequirementRefs, "goal_ref", objects.FieldKeyGoalRefs},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list criteria: %w", err)
	}

	return planObj, &treeEntities{
		backlogs: backlogRes.Objects,
		goals:    goalRes.Objects,
		reqs:     reqRes.Objects,
		crits:    critRes.Objects,
	}, nil
}

func assembleTree(planID string, planObj map[string]any, entities *treeEntities) *TreeNode {
	// Build index maps
	goalMap := indexObjectsByID(entities.goals)
	critMap := indexObjectsByID(entities.crits)

	// Filter backlog items and goals belonging to this plan
	planBacklogs := filterBacklogsByPlan(entities.backlogs, planID)
	planGoalsSet := resolvePlanGoals(entities.goals, planBacklogs, planID)

	// Map relationships
	reqsByGoal := mapRequirementsToGoals(entities.reqs)
	backlogsByReq := mapBacklogsToRequirements(planBacklogs)
	backlogsByGoal := mapBacklogsToGoals(planBacklogs)
	orphanBacklogs := findOrphanBacklogs(planBacklogs)
	criteriaByParent := mapCriteriaToParents(planBacklogs, entities.reqs, critMap)

	// Construct tree starting from Priority Plan
	rootNode := &TreeNode{
		ID:     planID,
		Type:   objects.KindPriorityPlan,
		Title:  objects.GetString(planObj, objects.FieldKeyTitle),
		Status: objects.GetString(planObj, objects.FieldKeyStatus),
	}

	// 1. Add Goals to the tree
	for goalID := range planGoalsSet {
		goalObj, exists := goalMap[goalID]
		if !exists {
			continue
		}
		goalNode := &TreeNode{
			ID:     goalID,
			Type:   objects.KindGoal,
			Title:  objects.GetString(goalObj, objects.FieldKeyTitle),
			Status: objects.GetString(goalObj, objects.FieldKeyStatus),
		}

		// Add Requirements for Goal
		for _, reqObj := range reqsByGoal[goalID] {
			reqID := objects.GetString(reqObj, objects.FieldKeyID)
			reqNode := &TreeNode{
				ID:     reqID,
				Type:   objects.KindRequirement,
				Title:  objects.GetString(reqObj, objects.FieldKeyTitle),
				Status: objects.GetString(reqObj, objects.FieldKeyStatus),
			}

			// Add Backlog Items for Requirement
			for _, bliObj := range backlogsByReq[reqID] {
				bliID := objects.GetString(bliObj, objects.FieldKeyID)
				bliNode := &TreeNode{
					ID:     bliID,
					Type:   objects.KindBacklogItem,
					Title:  objects.GetString(bliObj, objects.FieldKeyTitle),
					Status: objects.GetString(bliObj, objects.FieldKeyStatus),
				}
				bliNode.Children = buildCriteriaNodes(criteriaByParent[bliID])
				reqNode.Children = append(reqNode.Children, bliNode)
			}

			// Add Criteria directly linked to Requirement
			reqNode.Children = append(reqNode.Children, buildCriteriaNodes(criteriaByParent[reqID])...)

			goalNode.Children = append(goalNode.Children, reqNode)
		}

		// Add Backlog Items directly linked to Goal
		for _, bliObj := range backlogsByGoal[goalID] {
			bliID := objects.GetString(bliObj, objects.FieldKeyID)
			bliNode := &TreeNode{
				ID:     bliID,
				Type:   objects.KindBacklogItem,
				Title:  objects.GetString(bliObj, objects.FieldKeyTitle),
				Status: objects.GetString(bliObj, objects.FieldKeyStatus),
			}
			bliNode.Children = buildCriteriaNodes(criteriaByParent[bliID])
			goalNode.Children = append(goalNode.Children, bliNode)
		}

		rootNode.Children = append(rootNode.Children, goalNode)
	}

	// 2. Add Orphan Backlog Items directly under root
	for _, bliObj := range orphanBacklogs {
		bliID := objects.GetString(bliObj, objects.FieldKeyID)
		bliNode := &TreeNode{
			ID:     bliID,
			Type:   objects.KindBacklogItem,
			Title:  objects.GetString(bliObj, objects.FieldKeyTitle),
			Status: objects.GetString(bliObj, objects.FieldKeyStatus),
		}
		bliNode.Children = buildCriteriaNodes(criteriaByParent[bliID])
		rootNode.Children = append(rootNode.Children, bliNode)
	}

	return rootNode
}

func resolvePriorityPlan(ctx context.Context, sp workflowStorage, explicit string) (string, error) {
	secCtx := pkgctx.NewSystemSecurityContext()

	if explicit != "" {
		obj, err := sp.Read(ctx, secCtx, explicit)
		if err == nil && obj != nil {
			return explicit, nil
		}
		res, lerr := sp.List(ctx, secCtx, nil, storage.ListFilter{
			Kind:    objects.KindPriorityPlan,
			Filters: map[string]any{objects.FieldKeyID: explicit},
			Limit:   1,
			Fields:  []string{objects.FieldKeyID, objects.FieldKeyStatus},
		})
		if lerr == nil && len(res.Objects) > 0 {
			return objects.GetString(res.Objects[0], objects.FieldKeyID), nil
		}
		return "", fmt.Errorf("explicit priority plan %s not found", explicit)
	}

	// Try in_progress
	res, err := sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress},
		Limit:   1,
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyStatus},
	})
	if err == nil && len(res.Objects) > 0 {
		return objects.GetString(res.Objects[0], objects.FieldKeyID), nil
	}

	// Try active
	res, err = sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive},
		Limit:   1,
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyStatus},
	})
	if err == nil && len(res.Objects) > 0 {
		return objects.GetString(res.Objects[0], objects.FieldKeyID), nil
	}

	// Try paused
	res, err = sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusPaused},
		Limit:   1,
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyStatus},
	})
	if err == nil && len(res.Objects) > 0 {
		return objects.GetString(res.Objects[0], objects.FieldKeyID), nil
	}

	return "", fmt.Errorf("no active, in_progress, or paused priority plan found; please specify one with --priority-plan")
}

func indexObjectsByID(objs []map[string]any) map[string]map[string]any {
	m := make(map[string]map[string]any)
	for _, obj := range objs {
		m[objects.GetString(obj, objects.FieldKeyID)] = obj
	}
	return m
}

func filterBacklogsByPlan(objs []map[string]any, planID string) []map[string]any {
	var out []map[string]any
	for _, item := range objs {
		ref := objects.GetString(item, objects.FieldKeyPriorityPlanRef)
		refsRaw, hasRefs := item[objects.FieldKeyPriorityPlanRefs]
		inRefs := false
		if hasRefs {
			if refs, ok := refsRaw.([]any); ok {
				for _, r := range refs {
					if rStr, ok := r.(string); ok && rStr == planID {
						inRefs = true
						break
					}
				}
			}
		}
		if ref == planID || inRefs {
			out = append(out, item)
		}
	}
	return out
}

func toStringSlice(val any) []string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		res := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				res = append(res, s)
			}
		}
		return res
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return nil
}

func resolvePlanGoals(goals []map[string]any, planBacklogs []map[string]any, planID string) map[string]bool {
	set := make(map[string]bool)
	for _, goal := range goals {
		ref := objects.GetString(goal, objects.FieldKeyPriorityPlanRef)
		refsRaw, hasRefs := goal[objects.FieldKeyPriorityPlanRefs]
		inRefs := false
		if hasRefs {
			if refs, ok := refsRaw.([]any); ok {
				for _, r := range refs {
					if rStr, ok := r.(string); ok && rStr == planID {
						inRefs = true
						break
					}
				}
			}
		}
		if ref == planID || inRefs {
			set[objects.GetString(goal, objects.FieldKeyID)] = true
		}
	}
	for _, item := range planBacklogs {
		for _, gStr := range toStringSlice(item[objects.FieldKeyGoalRefs]) {
			if gStr != "" {
				set[gStr] = true
			}
		}
	}
	return set
}

func mapRequirementsToGoals(reqs []map[string]any) map[string][]map[string]any {
	m := make(map[string][]map[string]any)
	for _, req := range reqs {
		for _, gStr := range toStringSlice(req[objects.FieldKeyGoalRefs]) {
			if gStr != "" {
				m[gStr] = append(m[gStr], req)
			}
		}
	}
	return m
}

func mapBacklogsToRequirements(backlogs []map[string]any) map[string][]map[string]any {
	m := make(map[string][]map[string]any)
	for _, item := range backlogs {
		for _, rStr := range toStringSlice(item[objects.FieldKeyRequirementRefs]) {
			if rStr != "" {
				m[rStr] = append(m[rStr], item)
			}
		}
	}
	return m
}

func mapBacklogsToGoals(backlogs []map[string]any) map[string][]map[string]any {
	m := make(map[string][]map[string]any)
	for _, item := range backlogs {
		reqRefs := toStringSlice(item[objects.FieldKeyRequirementRefs])
		if len(reqRefs) == 0 {
			for _, gStr := range toStringSlice(item[objects.FieldKeyGoalRefs]) {
				if gStr != "" {
					m[gStr] = append(m[gStr], item)
				}
			}
		}
	}
	return m
}

func findOrphanBacklogs(backlogs []map[string]any) []map[string]any {
	var out []map[string]any
	for _, item := range backlogs {
		goalRefs := toStringSlice(item[objects.FieldKeyGoalRefs])
		reqRefs := toStringSlice(item[objects.FieldKeyRequirementRefs])
		if len(goalRefs) == 0 && len(reqRefs) == 0 {
			out = append(out, item)
		}
	}
	return out
}

func mapCriteriaToParents(backlogs []map[string]any, reqs []map[string]any, critMap map[string]map[string]any) map[string][]map[string]any {
	m := make(map[string][]map[string]any)
	for _, item := range backlogs {
		id := objects.GetString(item, objects.FieldKeyID)
		for _, cStr := range toStringSlice(item[objects.FieldKeyCriteriaRefs]) {
			if critObj, exists := critMap[cStr]; exists {
				m[id] = append(m[id], critObj)
			}
		}
	}
	for _, req := range reqs {
		id := objects.GetString(req, objects.FieldKeyID)
		for _, cStr := range toStringSlice(req[objects.FieldKeyCriteriaRefs]) {
			if critObj, exists := critMap[cStr]; exists {
				m[id] = append(m[id], critObj)
			}
		}
	}
	return m
}

func buildCriteriaNodes(critObjs []map[string]any) []*TreeNode {
	var out []*TreeNode
	for _, obj := range critObjs {
		out = append(out, &TreeNode{
			ID:     objects.GetString(obj, objects.FieldKeyID),
			Type:   objects.KindCriteria,
			Title:  objects.GetString(obj, objects.FieldKeyTitle),
			Status: objects.GetString(obj, objects.FieldKeyStatus),
		})
	}
	return out
}

func printTree(b *strings.Builder, node *TreeNode, prefix string, isLast bool, currentDepth int, maxDepth int) {
	if currentDepth > maxDepth {
		totalDeps := countTotalDescendants(node)
		if totalDeps > 0 {
			connector := "└── "
			if !isLast {
				connector = "├── "
			}
			b.WriteString(fmt.Sprintf("%s%s... (+%d rolled up dependencies)\n", prefix, connector, totalDeps))
		}
		return
	}

	connector := "├── "
	if isLast {
		connector = "└── "
	}

	if prefix == "" {
		b.WriteString(fmt.Sprintf("%s [%s] %s\n", node.ID, node.Status, node.Title))
	} else {
		b.WriteString(fmt.Sprintf("%s%s%s [%s] %s\n", prefix, connector, node.ID, node.Status, node.Title))
	}

	newPrefix := prefix
	if currentDepth > 0 {
		if isLast {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}
	}

	numChildren := len(node.Children)
	for i, child := range node.Children {
		printTree(b, child, newPrefix, i == numChildren-1, currentDepth+1, maxDepth)
	}
}

func countTotalDescendants(node *TreeNode) int {
	count := len(node.Children)
	for _, child := range node.Children {
		count += countTotalDescendants(child)
	}
	return count
}
