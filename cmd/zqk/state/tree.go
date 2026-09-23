package state

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type JournalMutation struct {
	ID          string `json:"id"`
	ChangeType  string `json:"change_type"`
	ObjectRef   string `json:"object_ref"`
	DiffSummary string `json:"diff_summary,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
	CreatedBy   string `json:"created_by,omitempty"`
}

type BacklogItemNode struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Priority    string `json:"priority,omitempty"`
	ClaimedBy   string `json:"claimed_by,omitempty"`
	CriteriaIDs []string `json:"criteria_ids,omitempty"`
}

type RequirementNode struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Status       string            `json:"status"`
	BacklogItems []BacklogItemNode `json:"backlog_items,omitempty"`
}

type PriorityPlanNode struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Status       string            `json:"status"`
	Workstreams  []string          `json:"workstreams,omitempty"`
	Requirements []RequirementNode `json:"requirements,omitempty"`
}

type StateTreePayload struct {
	Organization *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"organization,omitempty"`
	Mission *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"mission,omitempty"`
	Vision *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"vision,omitempty"`
	Goal *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"goal,omitempty"`
	ActivePlans     []PriorityPlanNode `json:"active_plans,omitempty"`
	RecentMutations []JournalMutation  `json:"recent_mutations,omitempty"`
}

func newTreeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewStateTreeCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			return runStateTree(cmd, proc)
		})(cmd, args)
	}
	return cmd
}

func runStateTree(cmd *cobra.Command, proc *cli.Processor) error {
	ctx := proc.OperationContext()
	sp := proc.Storage()
	projectRoot := proc.ProjectRoot()
	if projectRoot == "" {
		projectRoot = cli.ResolveProjectRoot(".")
	}

	payload := buildStateTreePayload(ctx, sp, projectRoot)

	format, _ := cmd.Flags().GetString("format")
	if format == "json" {
		return cli.FormatOutputAs(cmd, cli.FormatJSON, payload)
	}

	return renderStateTreeText(cmd, payload)
}

func buildStateTreePayload(ctx context.Context, sp storage.ObjectStorageProvider, projectRoot string) *StateTreePayload {
	payload := &StateTreePayload{}
	storageCtx := pkgctx.NewStorageContext()
	secCtx := pkgctx.GetSecurityContext(ctx)

	// 1. Organization
	if orgs, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindOrganization}); err == nil && len(orgs.Objects) > 0 {
		payload.Organization = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(orgs.Objects[0], objects.FieldKeyID),
			Title: getString(orgs.Objects[0], objects.FieldKeyTitle),
		}
	}

	// 2. Mission
	if missions, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindMission}); err == nil && len(missions.Objects) > 0 {
		payload.Mission = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(missions.Objects[0], objects.FieldKeyID),
			Title: getString(missions.Objects[0], objects.FieldKeyTitle),
		}
	}

	// 3. Vision
	if visions, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindVision}); err == nil && len(visions.Objects) > 0 {
		payload.Vision = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(visions.Objects[0], objects.FieldKeyID),
			Title: getString(visions.Objects[0], objects.FieldKeyTitle),
		}
	}

	// 4. Goal
	if goals, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindGoal}); err == nil && len(goals.Objects) > 0 {
		payload.Goal = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(goals.Objects[0], objects.FieldKeyID),
			Title: getString(goals.Objects[0], objects.FieldKeyTitle),
		}
	}

	// 5. Active Priority Plans
	planNodes := make([]PriorityPlanNode, 0)
	if plans, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindPriorityPlan}); err == nil {
		// Filter for open priority plans (skip complete, archived, cancelled)
		for _, p := range plans.Objects {
			status := getString(p, objects.FieldKeyStatus)
			if status == objects.ObjectStatusComplete || status == "archived" || status == "cancelled" {
				continue
			}
			planID := getString(p, objects.FieldKeyID)
			node := PriorityPlanNode{
				ID:     planID,
				Title:  getString(p, objects.FieldKeyTitle),
				Status: status,
			}
			// Workstreams
			if wsList, ok := p[objects.FieldKeyWorkstreamRefs].([]any); ok {
				for _, ws := range wsList {
					if wsStr, ok := ws.(string); ok {
						node.Workstreams = append(node.Workstreams, wsStr)
					}
				}
			}
			// Requirements
			reqMap := make(map[string]*RequirementNode)
			if reqs, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindRequirement}); err == nil {
				for _, r := range reqs.Objects {
					rID := getString(r, objects.FieldKeyID)
					reqMap[rID] = &RequirementNode{
						ID:     rID,
						Title:  getString(r, objects.FieldKeyTitle),
						Status: getString(r, objects.FieldKeyStatus),
					}
				}
			}
			// Backlog items linked to this plan
			if blis, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindBacklogItem}); err == nil {
				for _, b := range blis.Objects {
					planRef := getString(b, objects.FieldKeyPriorityPlanRef)
					if planRef != planID {
						continue
					}
					bNode := BacklogItemNode{
						ID:        getString(b, objects.FieldKeyID),
						Title:     getString(b, objects.FieldKeyTitle),
						Status:    getString(b, objects.FieldKeyStatus),
						Priority:  getString(b, "priority_tier"),
						ClaimedBy: getString(b, "claimed_by"),
					}
					// Find which requirement this BLI belongs to
					reqRefs, _ := b[objects.FieldKeyRequirementRefs].([]any)
					matched := false
					for _, rr := range reqRefs {
						if reqID, ok := rr.(string); ok {
							if rNode, exists := reqMap[reqID]; exists {
								rNode.BacklogItems = append(rNode.BacklogItems, bNode)
								matched = true
								break
							}
						}
					}
					if !matched && len(reqRefs) > 0 {
						if reqID, ok := reqRefs[0].(string); ok {
							if reqMap[reqID] == nil {
								reqMap[reqID] = &RequirementNode{ID: reqID, Title: reqID}
							}
							reqMap[reqID].BacklogItems = append(reqMap[reqID].BacklogItems, bNode)
						}
					}
				}
			}

			for _, rNode := range reqMap {
				if len(rNode.BacklogItems) > 0 {
					node.Requirements = append(node.Requirements, *rNode)
				}
			}
			planNodes = append(planNodes, node)
		}
	}
	payload.ActivePlans = planNodes

	// 6. Recent journal mutations from streams
	payload.RecentMutations = readRecentJournalMutations(projectRoot, 8)

	return payload
}

func readRecentJournalMutations(projectRoot string, limit int) []JournalMutation {
	if projectRoot == "" {
		return nil
	}
	streamDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	files, err := os.ReadDir(streamDir)
	if err != nil || len(files) == 0 {
		return nil
	}

	// Sort files by modification time descending
	type fileEntry struct {
		name    string
		modTime int64
	}
	entries := make([]fileEntry, 0, len(files))
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			if info, err := f.Info(); err == nil {
				entries = append(entries, fileEntry{name: f.Name(), modTime: info.ModTime().UnixNano()})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime > entries[j].modTime
	})

	mutations := make([]JournalMutation, 0, limit)
	for _, fe := range entries {
		path := filepath.Join(streamDir, fe.name)
		file, err := fileutil.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		fileLines := make([]string, 0, 64)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				fileLines = append(fileLines, line)
			}
		}
		file.Close()

		// Read in reverse order
		for i := len(fileLines) - 1; i >= 0; i-- {
			var m JournalMutation
			if err := json.Unmarshal([]byte(fileLines[i]), &m); err == nil && m.ID != "" {
				mutations = append(mutations, m)
				if len(mutations) >= limit {
					return mutations
				}
			}
		}
	}
	return mutations
}

func renderStateTreeText(cmd *cobra.Command, payload *StateTreePayload) error {
	return cli.WriteOutput(cmd, []byte(FormatStateTreeText(payload)))
}

// FormatStateTreeText returns the human-readable tree representation of the state payload.
func FormatStateTreeText(payload *StateTreePayload) string {
	var buf strings.Builder
	buf.WriteString("\n📍 ZQK Knowledge Kernel State Tree\n")

	// Organization
	orgName := "Default Organization"
	orgID := "ORG-DEFAULT"
	if payload.Organization != nil {
		orgName = payload.Organization.Title
		orgID = payload.Organization.ID
	}
	fmt.Fprintf(&buf, "├── Organization: %s [%s]\n", orgName, orgID)

	// Mission
	misName := "No Mission"
	misID := "MIS-NONE"
	if payload.Mission != nil {
		misName = payload.Mission.Title
		misID = payload.Mission.ID
	}
	fmt.Fprintf(&buf, "│   └── Mission: %s [%s]\n", misName, misID)

	// Vision
	visName := "No Vision"
	visID := "VIS-NONE"
	if payload.Vision != nil {
		visName = payload.Vision.Title
		visID = payload.Vision.ID
	}
	fmt.Fprintf(&buf, "│       └── Vision: %s [%s]\n", visName, visID)

	// Goal
	goalName := "No Goal"
	goalID := "GOAL-NONE"
	if payload.Goal != nil {
		goalName = payload.Goal.Title
		goalID = payload.Goal.ID
	}
	fmt.Fprintf(&buf, "│           └── Goal: %s [%s]\n", goalName, goalID)

	// Priority Plans
	if len(payload.ActivePlans) == 0 {
		buf.WriteString("│               └── Priority Plan: (none active or in_progress)\n")
	} else {
		for _, plan := range payload.ActivePlans {
			fmt.Fprintf(&buf, "│               └── Priority Plan: %s [%s] (%s)\n", plan.Title, plan.ID, plan.Status)
			for _, ws := range plan.Workstreams {
				fmt.Fprintf(&buf, "│                   ├── Workstream: %s\n", ws)
			}
			for _, req := range plan.Requirements {
				fmt.Fprintf(&buf, "│                   └── Requirement: %s [%s]\n", req.Title, req.ID)
				for _, bli := range req.BacklogItems {
					claimed := ""
					if bli.ClaimedBy != "" {
						claimed = fmt.Sprintf(" (leased: %s)", bli.ClaimedBy)
					}
					fmt.Fprintf(&buf, "│                       └── BLI: %s [%s] (%s)%s\n", bli.Title, bli.ID, bli.Status, claimed)
				}
			}
		}
	}

	// Recent Mutations
	buf.WriteString("└── Recent Journal Mutations:\n")
	if len(payload.RecentMutations) == 0 {
		buf.WriteString("    └── (no recent stream journal entries)\n")
	} else {
		for i, m := range payload.RecentMutations {
			prefix := "    ├──"
			if i == len(payload.RecentMutations)-1 {
				prefix = "    └──"
			}
			diff := m.DiffSummary
			if diff == "" {
				diff = m.ChangeType
			}
			fmt.Fprintf(&buf, "%s [%s] %s: %s\n", prefix, m.ID, m.ObjectRef, diff)
		}
	}
	buf.WriteString("\n")

	return buf.String()
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
