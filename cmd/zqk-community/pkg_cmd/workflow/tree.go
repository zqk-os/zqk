package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

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

		priFlag, _ := cmd.Flags().GetString("priority-plan")
		depth, _ := cmd.Flags().GetInt("depth")

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

// Resolve fetches all related items and constructs the dependency tree.
func (r *DependencyTreeResolver) Resolve(ctx context.Context, planID string) (*TreeNode, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Read the priority plan
	planObj, err := r.sp.Read(ctx, secCtx, planID)
	if err != nil {
		return nil, errfmt.Errorf("failed to read priority plan %s: %w", planID, err)
	}

	var (
		backlogRes *storage.QueryResult
		goalRes    *storage.QueryResult
		reqRes     *storage.QueryResult
		critRes    *storage.QueryResult
	)

	g, gCtx := errgroup.WithContext(ctx)

	// Fetch backlog items
	g.Go(func() error {
		var err error
		backlogRes, err = r.sp.List(gCtx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindBacklogItem})
		if err != nil {
			return errfmt.Errorf("failed to list backlog items: %w", err)
		}
		return nil
	})

	// Fetch goals
	g.Go(func() error {
		var err error
		goalRes, err = r.sp.List(gCtx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindGoal})
		if err != nil {
			return errfmt.Errorf("failed to list goals: %w", err)
		}
		return nil
	})

	// Fetch requirements
	g.Go(func() error {
		var err error
		reqRes, err = r.sp.List(gCtx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindRequirement})
		if err != nil {
			return errfmt.Errorf("failed to list requirements: %w", err)
		}
		return nil
	})

	// Fetch criteria
	g.Go(func() error {
		var err error
		critRes, err = r.sp.List(gCtx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindCriteria})
		if err != nil {
			return errfmt.Errorf("failed to list criteria: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Build index maps
	goalMap := indexObjectsByID(goalRes.Objects)
	critMap := indexObjectsByID(critRes.Objects)

	// Filter backlog items and goals belonging to this plan
	planBacklogs := filterBacklogsByPlan(backlogRes.Objects, planID)
	planGoalsSet := resolvePlanGoals(goalRes.Objects, planBacklogs, planID)

	// Map relationships
	reqsByGoal := mapRequirementsToGoals(reqRes.Objects)
	backlogsByReq := mapBacklogsToRequirements(planBacklogs)
	backlogsByGoal := mapBacklogsToGoals(planBacklogs)
	orphanBacklogs := findOrphanBacklogs(planBacklogs)
	criteriaByParent := mapCriteriaToParents(planBacklogs, reqRes.Objects, critMap)

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

	return rootNode, nil
}

func resolvePriorityPlan(ctx context.Context, sp workflowStorage, explicit string) (string, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	if explicit != "" {
		obj, err := sp.Read(ctx, secCtx, explicit)
		if err == nil && obj != nil {
			return explicit, nil
		}
		res, lerr := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
			Kind:    objects.KindPriorityPlan,
			Filters: map[string]any{objects.FieldKeyID: explicit},
		})
		if lerr == nil && len(res.Objects) > 0 {
			return objects.GetString(res.Objects[0], objects.FieldKeyID), nil
		}
		return "", errfmt.Errorf("explicit priority plan %s not found", explicit)
	}

	// Try in_progress
	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress},
	})
	if err == nil && len(res.Objects) > 0 {
		return objects.GetString(res.Objects[0], objects.FieldKeyID), nil
	}

	// Try active
	res, err = sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive},
	})
	if err == nil && len(res.Objects) > 0 {
		return objects.GetString(res.Objects[0], objects.FieldKeyID), nil
	}

	// Try paused
	res, err = sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusPaused},
	})
	if err == nil && len(res.Objects) > 0 {
		return objects.GetString(res.Objects[0], objects.FieldKeyID), nil
	}

	return "", errfmt.Errorf("no active, in_progress, or paused priority plan found; please specify one with --priority-plan")
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
		refsRaw, hasRefs := item["priority_plan_refs"]
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

func resolvePlanGoals(goals []map[string]any, planBacklogs []map[string]any, planID string) map[string]bool {
	set := make(map[string]bool)
	for _, goal := range goals {
		ref := objects.GetString(goal, objects.FieldKeyPriorityPlanRef)
		refsRaw, hasRefs := goal["priority_plan_refs"]
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
		goalRefsRaw, ok := item[objects.FieldKeyGoalRefs]
		if ok {
			if goalRefs, ok := goalRefsRaw.([]any); ok {
				for _, g := range goalRefs {
					if gStr, ok := g.(string); ok && gStr != "" {
						set[gStr] = true
					}
				}
			}
		}
	}
	return set
}

func mapRequirementsToGoals(reqs []map[string]any) map[string][]map[string]any {
	m := make(map[string][]map[string]any)
	for _, req := range reqs {
		goalRefsRaw, ok := req[objects.FieldKeyGoalRefs]
		if ok {
			if goalRefs, ok := goalRefsRaw.([]any); ok {
				for _, g := range goalRefs {
					if gStr, ok := g.(string); ok {
						m[gStr] = append(m[gStr], req)
					}
				}
			}
		}
	}
	return m
}

func mapBacklogsToRequirements(backlogs []map[string]any) map[string][]map[string]any {
	m := make(map[string][]map[string]any)
	for _, item := range backlogs {
		reqRefsRaw, ok := item[objects.FieldKeyRequirementRefs]
		if ok {
			if reqRefs, ok := reqRefsRaw.([]any); ok {
				for _, r := range reqRefs {
					if rStr, ok := r.(string); ok {
						m[rStr] = append(m[rStr], item)
					}
				}
			}
		}
	}
	return m
}

func mapBacklogsToGoals(backlogs []map[string]any) map[string][]map[string]any {
	m := make(map[string][]map[string]any)
	for _, item := range backlogs {
		reqRefsRaw, okR := item[objects.FieldKeyRequirementRefs]
		hasReq := okR && reqRefsRaw != nil && len(reqRefsRaw.([]any)) > 0
		if !hasReq {
			goalRefsRaw, ok := item[objects.FieldKeyGoalRefs]
			if ok {
				if goalRefs, ok := goalRefsRaw.([]any); ok {
					for _, g := range goalRefs {
						if gStr, ok := g.(string); ok {
							m[gStr] = append(m[gStr], item)
						}
					}
				}
			}
		}
	}
	return m
}

func findOrphanBacklogs(backlogs []map[string]any) []map[string]any {
	var out []map[string]any
	for _, item := range backlogs {
		goalRefsRaw, okG := item[objects.FieldKeyGoalRefs]
		reqRefsRaw, okR := item[objects.FieldKeyRequirementRefs]
		hasGoal := okG && goalRefsRaw != nil && len(goalRefsRaw.([]any)) > 0
		hasReq := okR && reqRefsRaw != nil && len(reqRefsRaw.([]any)) > 0
		if !hasGoal && !hasReq {
			out = append(out, item)
		}
	}
	return out
}

func mapCriteriaToParents(backlogs []map[string]any, reqs []map[string]any, critMap map[string]map[string]any) map[string][]map[string]any {
	m := make(map[string][]map[string]any)
	for _, item := range backlogs {
		id := objects.GetString(item, objects.FieldKeyID)
		critRefsRaw, ok := item[objects.FieldKeyAcceptanceCriteria]
		if ok {
			if critRefs, ok := critRefsRaw.([]any); ok {
				for _, c := range critRefs {
					if cStr, ok := c.(string); ok {
						if critObj, exists := critMap[cStr]; exists {
							m[id] = append(m[id], critObj)
						}
					}
				}
			}
		}
	}
	for _, req := range reqs {
		id := objects.GetString(req, objects.FieldKeyID)
		critRefsRaw, ok := req[objects.FieldKeyCriteriaRefs]
		if ok {
			if critRefs, ok := critRefsRaw.([]any); ok {
				for _, c := range critRefs {
					if cStr, ok := c.(string); ok {
						if critObj, exists := critMap[cStr]; exists {
							m[id] = append(m[id], critObj)
						}
					}
				}
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
