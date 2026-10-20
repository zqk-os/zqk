package state

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
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
	StreamKind  string `json:"stream_kind,omitempty"`
	Actor       string `json:"actor,omitempty"`
	Operation   string `json:"operation,omitempty"`
}

type BacklogItemNode struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority,omitempty"`
	ClaimedBy   string   `json:"claimed_by,omitempty"`
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
	projectRoot := proc.ResolvedProjectRoot()

	payload := buildStateTreePayload(ctx, sp, projectRoot)

	format, _ := cmd.Flags().GetString("format")
	if format == "json" {
		return cli.FormatOutputAs(cmd, cli.FormatJSON, payload)
	}

	return renderStateTreeText(cmd, payload)
}

func loadStateCoreEntities(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, sp storage.ObjectStorageProvider, payload *StateTreePayload) {
	if orgs, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.IdTitle(objects.KindOrganization).Limit(1).Build()); err == nil && len(orgs.Objects) > 0 {
		payload.Organization = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(orgs.Objects[0], objects.FieldKeyID),
			Title: getString(orgs.Objects[0], objects.FieldKeyTitle),
		}
	}
	if missions, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.IdTitle(objects.KindMission).Limit(1).Build()); err == nil && len(missions.Objects) > 0 {
		payload.Mission = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(missions.Objects[0], objects.FieldKeyID),
			Title: getString(missions.Objects[0], objects.FieldKeyTitle),
		}
	}
	if visions, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.IdTitle(objects.KindVision).Limit(1).Build()); err == nil && len(visions.Objects) > 0 {
		payload.Vision = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(visions.Objects[0], objects.FieldKeyID),
			Title: getString(visions.Objects[0], objects.FieldKeyTitle),
		}
	}
	if goals, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.IdTitle(objects.KindGoal).Limit(1).Build()); err == nil && len(goals.Objects) > 0 {
		payload.Goal = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(goals.Objects[0], objects.FieldKeyID),
			Title: getString(goals.Objects[0], objects.FieldKeyTitle),
		}
	}
}

func loadBaseRequirements(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, sp storage.ObjectStorageProvider) map[string]RequirementNode {
	baseReqMap := make(map[string]RequirementNode)
	if reqs, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
		NotArchived(objects.KindRequirement).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus).
		Build()); err == nil {
		for _, r := range reqs.Objects {
			rID := getString(r, objects.FieldKeyID)
			baseReqMap[rID] = RequirementNode{
				ID:     rID,
				Title:  getString(r, objects.FieldKeyTitle),
				Status: getString(r, objects.FieldKeyStatus),
			}
		}
	}
	return baseReqMap
}

func attachBacklogItemsToPlanRequirements(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, sp storage.ObjectStorageProvider, planID string, reqMap map[string]*RequirementNode) {
	blis, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
		ForPlan(objects.KindBacklogItem, planID).
		StatusNot(objects.ObjectStatusArchived).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, "priority_tier", "claimed_by", objects.FieldKeyRequirementRefs).
		Build())
	if err != nil {
		return
	}
	for _, b := range blis.Objects {
		bNode := BacklogItemNode{
			ID:        getString(b, objects.FieldKeyID),
			Title:     getString(b, objects.FieldKeyTitle),
			Status:    getString(b, objects.FieldKeyStatus),
			Priority:  getString(b, "priority_tier"),
			ClaimedBy: getString(b, "claimed_by"),
		}
		reqRefs := extractStringSlice(b, objects.FieldKeyRequirementRefs)
		matched := false
		for _, reqID := range reqRefs {
			if rNode, exists := reqMap[reqID]; exists {
				rNode.BacklogItems = append(rNode.BacklogItems, bNode)
				matched = true
				break
			}
		}
		if !matched {
			reqID := "REQ-UNASSIGNED"
			if len(reqRefs) > 0 {
				reqID = reqRefs[0]
			}
			if reqMap[reqID] == nil {
				reqMap[reqID] = &RequirementNode{ID: reqID, Title: reqID}
			}
			reqMap[reqID].BacklogItems = append(reqMap[reqID].BacklogItems, bNode)
		}
	}
}

func buildPriorityPlanNode(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, sp storage.ObjectStorageProvider, p map[string]any, baseReqMap map[string]RequirementNode) PriorityPlanNode {
	planID := getString(p, objects.FieldKeyID)
	node := PriorityPlanNode{
		ID:          planID,
		Title:       getString(p, objects.FieldKeyTitle),
		Status:      getString(p, objects.FieldKeyStatus),
		Workstreams: extractStringSlice(p, objects.FieldKeyWorkstreamRefs),
	}
	reqMap := make(map[string]*RequirementNode, len(baseReqMap))
	for rID, rn := range baseReqMap {
		rnCopy := rn
		reqMap[rID] = &rnCopy
	}
	attachBacklogItemsToPlanRequirements(ctx, secCtx, storageCtx, sp, planID, reqMap)

	sortedReqIDs := make([]string, 0, len(reqMap))
	for rID := range reqMap {
		sortedReqIDs = append(sortedReqIDs, rID)
	}
	sort.Strings(sortedReqIDs)

	for _, rID := range sortedReqIDs {
		rNode := reqMap[rID]
		if len(rNode.BacklogItems) > 0 {
			node.Requirements = append(node.Requirements, *rNode)
		}
	}
	return node
}

func loadActivePriorityPlans(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, sp storage.ObjectStorageProvider, baseReqMap map[string]RequirementNode) []PriorityPlanNode {
	plans, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
		ActiveNotComplete(objects.KindPriorityPlan).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyWorkstreamRefs).
		Build())
	if err != nil {
		return nil
	}
	planNodes := make([]PriorityPlanNode, 0, len(plans.Objects))
	for _, p := range plans.Objects {
		planNodes = append(planNodes, buildPriorityPlanNode(ctx, secCtx, storageCtx, sp, p, baseReqMap))
	}
	return planNodes
}

func buildStateTreePayload(ctx context.Context, sp storage.ObjectStorageProvider, projectRoot string) *StateTreePayload {
	payload := &StateTreePayload{}
	storageCtx := pkgctx.NewStorageContext()
	secCtx := pkgctx.GetSecurityContext(ctx)

	loadStateCoreEntities(ctx, secCtx, storageCtx, sp, payload)
	baseReqMap := loadBaseRequirements(ctx, secCtx, storageCtx, sp)
	payload.ActivePlans = loadActivePriorityPlans(ctx, secCtx, storageCtx, sp, baseReqMap)
	payload.RecentMutations = readRecentJournalMutations(projectRoot, 8)

	return payload
}

var (
	streamFileCacheMu sync.RWMutex
	streamFileCache   = make(map[string]cachedStreamFile)
)

type cachedStreamFile struct {
	stamp     stampmemo.Stamp
	mutations []JournalMutation
}

var trackedStreamKinds = []string{
	"change_journal_entry",
	"audit_event",
	"agent_instruction",
	"process_lifecycle",
}

type rawStreamRecord struct {
	ID          string `json:"id"`
	Kind        string `json:"kind,omitempty"`
	ChangeType  string `json:"change_type,omitempty"`
	EventType   string `json:"event_type,omitempty"`
	ObjectRef   string `json:"object_ref,omitempty"`
	DiffSummary string `json:"diff_summary,omitempty"`
	Operation   string `json:"operation,omitempty"`
	Title       string `json:"title,omitempty"`
	Instruction string `json:"instruction,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
	CreatedBy   string `json:"created_by,omitempty"`
	Actor       string `json:"actor,omitempty"`
}

type streamFileRef struct {
	path       string
	streamKind string
	modTime    int64
}

func readRecentJournalMutations(projectRoot string, limit int) []JournalMutation {
	return ReadRecentJournalMutations(projectRoot, limit)
}

// ReadRecentJournalMutations reads and aggregates recent mutations across all active kernel streams.
func ReadRecentJournalMutations(projectRoot string, limit int) []JournalMutation {
	return ReadRecentStreamMutations(projectRoot, limit, trackedStreamKinds...)
}

func collectStreamFiles(projectRoot string, streamKinds []string) []streamFileRef {
	var allFiles []streamFileRef
	for _, kind := range streamKinds {
		streamDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir, kind)
		files, err := fileutil.ReadDir(streamDir)
		if err != nil || len(files) == 0 {
			continue
		}
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".json") {
				p := filepath.Join(streamDir, f.Name())
				stamp := stampmemo.Of(p)
				if stamp > 0 {
					allFiles = append(allFiles, streamFileRef{path: p, streamKind: kind, modTime: int64(stamp)})
				}
			}
		}
	}
	return allFiles
}

func parseStreamMutationRecord(raw rawStreamRecord, streamKind string) JournalMutation {
	cType := raw.ChangeType
	if cType == "" {
		if raw.EventType != "" {
			cType = raw.EventType
		} else if raw.Kind != "" {
			cType = raw.Kind
		} else {
			cType = "event"
		}
	}

	ref := raw.ObjectRef
	if ref == "" {
		if raw.Kind != "" {
			ref = raw.Kind + ":" + raw.ID
		} else {
			ref = raw.ID
		}
	}

	summary := raw.DiffSummary
	if summary == "" {
		if raw.Operation != "" {
			summary = raw.Operation
		} else if raw.Title != "" {
			summary = raw.Title
		} else if raw.Instruction != "" {
			summary = raw.Instruction
		} else {
			summary = cType
		}
	}

	creator := raw.CreatedBy
	if creator == "" && raw.Actor != "" {
		creator = raw.Actor
	}

	return JournalMutation{
		ID:          raw.ID,
		ChangeType:  cType,
		ObjectRef:   ref,
		DiffSummary: summary,
		CreatedAt:   raw.CreatedAt,
		CreatedBy:   creator,
		StreamKind:  streamKind,
		Actor:       raw.Actor,
		Operation:   raw.Operation,
	}
}

func readStreamFileMutations(fileRef streamFileRef) []JournalMutation {
	stamp := stampmemo.Stamp(fileRef.modTime)

	streamFileCacheMu.RLock()
	cached, ok := streamFileCache[fileRef.path]
	streamFileCacheMu.RUnlock()

	if ok && cached.stamp == stamp {
		return cached.mutations
	}

	file, err := fileutil.Open(fileRef.path)
	if err != nil {
		return nil
	}
	defer func() {
		if cErr := file.Close(); cErr != nil {
			return
		}
	}()

	scanner := bufio.NewScanner(file)
	fileLines := make([]string, 0, 64)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			fileLines = append(fileLines, line)
		}
	}

	fileMutations := make([]JournalMutation, 0, len(fileLines))
	for _, line := range fileLines {
		var raw rawStreamRecord
		if err := json.Unmarshal([]byte(line), &raw); err == nil && raw.ID != "" {
			fileMutations = append(fileMutations, parseStreamMutationRecord(raw, fileRef.streamKind))
		}
	}

	streamFileCacheMu.Lock()
	streamFileCache[fileRef.path] = cachedStreamFile{
		stamp:     stamp,
		mutations: fileMutations,
	}
	streamFileCacheMu.Unlock()

	return fileMutations
}

// ReadRecentStreamMutations reads and aggregates recent mutations across specified kernel streams.
func ReadRecentStreamMutations(projectRoot string, limit int, streamKinds ...string) []JournalMutation {
	if projectRoot == "" {
		return nil
	}
	if len(streamKinds) == 0 {
		streamKinds = trackedStreamKinds
	}

	allFiles := collectStreamFiles(projectRoot, streamKinds)
	if len(allFiles) == 0 {
		return nil
	}

	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].modTime > allFiles[j].modTime
	})

	var allMutations []JournalMutation
	for _, fileRef := range allFiles {
		allMutations = append(allMutations, readStreamFileMutations(fileRef)...)
	}

	sort.Slice(allMutations, func(i, j int) bool {
		if allMutations[i].CreatedAt != allMutations[j].CreatedAt {
			return allMutations[i].CreatedAt > allMutations[j].CreatedAt
		}
		return allMutations[i].ID > allMutations[j].ID
	})

	if len(allMutations) > limit {
		allMutations = allMutations[:limit]
	}

	return allMutations
}

func renderStateTreeText(cmd *cobra.Command, payload *StateTreePayload) error {
	return cli.WriteOutput(cmd, []byte(FormatStateTreeText(payload)))
}

func formatCoreEntities(buf *strings.Builder, payload *StateTreePayload) {
	orgName := "Default Organization"
	orgID := "ORG-DEFAULT"
	if payload.Organization != nil {
		orgName = payload.Organization.Title
		orgID = payload.Organization.ID
	}
	fmt.Fprintf(buf, "├── Organization: %s [%s]\n", orgName, orgID)

	misName := "No Mission"
	misID := "MIS-NONE"
	if payload.Mission != nil {
		misName = payload.Mission.Title
		misID = payload.Mission.ID
	}
	fmt.Fprintf(buf, "│   └── Mission: %s [%s]\n", misName, misID)

	visName := "No Vision"
	visID := "VIS-NONE"
	if payload.Vision != nil {
		visName = payload.Vision.Title
		visID = payload.Vision.ID
	}
	fmt.Fprintf(buf, "│       └── Vision: %s [%s]\n", visName, visID)

	goalName := "No Goal"
	goalID := "GOAL-NONE"
	if payload.Goal != nil {
		goalName = payload.Goal.Title
		goalID = payload.Goal.ID
	}
	fmt.Fprintf(buf, "│           └── Goal: %s [%s]\n", goalName, goalID)
}

func formatPriorityPlans(buf *strings.Builder, payload *StateTreePayload) {
	if len(payload.ActivePlans) == 0 {
		buf.WriteString("│               └── Priority Plan: (none active or in_progress)\n")
		return
	}
	for _, plan := range payload.ActivePlans {
		fmt.Fprintf(buf, "│               └── Priority Plan: %s [%s] (%s)\n", plan.Title, plan.ID, plan.Status)
		for _, ws := range plan.Workstreams {
			fmt.Fprintf(buf, "│                   ├── Workstream: %s\n", ws)
		}
		for _, req := range plan.Requirements {
			fmt.Fprintf(buf, "│                   └── Requirement: %s [%s]\n", req.Title, req.ID)
			for _, bli := range req.BacklogItems {
				claimed := ""
				if bli.ClaimedBy != "" {
					claimed = fmt.Sprintf(" (leased: %s)", bli.ClaimedBy)
				}
				fmt.Fprintf(buf, "│                       └── BLI: %s [%s] (%s)%s\n", bli.Title, bli.ID, bli.Status, claimed)
			}
		}
	}
}

func formatRecentMutations(buf *strings.Builder, payload *StateTreePayload) {
	buf.WriteString("└── Recent Journal Mutations:\n")
	if len(payload.RecentMutations) == 0 {
		buf.WriteString("    └── (no recent stream journal entries)\n")
		return
	}
	for i, m := range payload.RecentMutations {
		prefix := "    ├──"
		if i == len(payload.RecentMutations)-1 {
			prefix = "    └──"
		}
		diff := m.DiffSummary
		if diff == "" {
			diff = m.ChangeType
		}
		fmt.Fprintf(buf, "%s [%s] %s: %s\n", prefix, m.ID, m.ObjectRef, diff)
	}
}

// FormatStateTreeText returns the human-readable tree representation of the state payload.
func FormatStateTreeText(payload *StateTreePayload) string {
	var buf strings.Builder
	buf.WriteString("\n📍 ZQK Knowledge Kernel State Tree\n")

	formatCoreEntities(&buf, payload)
	formatPriorityPlans(&buf, payload)
	formatRecentMutations(&buf, payload)
	buf.WriteString("\n")

	return buf.String()
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func extractStringSlice(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	val, ok := m[key]
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		res := make([]string, len(v))
		copy(res, v)
		return res
	case []any:
		var res []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				res = append(res, s)
			}
		}
		return res
	default:
		return nil
	}
}
