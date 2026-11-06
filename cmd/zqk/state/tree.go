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

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format == "json" {
		return cli.FormatOutputAs(cmd, cli.FormatJSON, payload)
	}

	return renderStateTreeText(cmd, payload)
}

func loadStateCoreEntities(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, payload *StateTreePayload) {
	if orgs, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.IdTitle(objects.KindOrganization).Limit(1).Build()); err == nil && len(orgs.Objects) > 0 {
		payload.Organization = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(orgs.Objects[0], objects.FieldKeyID),
			Title: getString(orgs.Objects[0], objects.FieldKeyTitle),
		}
	}
	if missions, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.IdTitle(objects.KindMission).Limit(1).Build()); err == nil && len(missions.Objects) > 0 {
		payload.Mission = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(missions.Objects[0], objects.FieldKeyID),
			Title: getString(missions.Objects[0], objects.FieldKeyTitle),
		}
	}
	if visions, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.IdTitle(objects.KindVision).Limit(1).Build()); err == nil && len(visions.Objects) > 0 {
		payload.Vision = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(visions.Objects[0], objects.FieldKeyID),
			Title: getString(visions.Objects[0], objects.FieldKeyTitle),
		}
	}
	if goals, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.IdTitle(objects.KindGoal).Limit(1).Build()); err == nil && len(goals.Objects) > 0 {
		payload.Goal = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    getString(goals.Objects[0], objects.FieldKeyID),
			Title: getString(goals.Objects[0], objects.FieldKeyTitle),
		}
	}
}

func loadBaseRequirements(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider) map[string]RequirementNode {
	baseReqMap := make(map[string]RequirementNode)
	if reqs, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.
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

func attachBacklogItemsToPlanRequirements(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, planID string, reqMap map[string]*RequirementNode) {
	blis, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.
		ForPlan(objects.KindBacklogItem, planID).
		StatusNot(objects.ObjectStatusArchived).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, "priority_tier", "claimed_by", objects.FieldKeyRequirementRefs).
		Build())

	var objectsList []map[string]any
	if err == nil && blis != nil && len(blis.Objects) > 0 {
		objectsList = blis.Objects
	} else {
		// Robust fallback: if index-based ForPlan returns empty or fails (e.g. unindexed files or reference index sync lag),
		// query all non-archived backlog items and match plan reference directly.
		fallbackBlis, fallbackErr := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.
			NotArchived(objects.KindBacklogItem).
			IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, "priority_tier", "claimed_by", objects.FieldKeyRequirementRefs, objects.FieldKeyPriorityPlanRef, objects.FieldKeyPriorityPlanRefs).
			Build())
		if fallbackErr == nil && fallbackBlis != nil {
			for _, b := range fallbackBlis.Objects {
				if getString(b, objects.FieldKeyPriorityPlanRef) == planID {
					objectsList = append(objectsList, b)
					continue
				}
				for _, r := range extractStringSlice(b, objects.FieldKeyPriorityPlanRefs) {
					if r == planID {
						objectsList = append(objectsList, b)
						break
					}
				}
			}
		}
	}

	for _, b := range objectsList {
		bNode := BacklogItemNode{
			ID:        getString(b, objects.FieldKeyID),
			Title:     getString(b, objects.FieldKeyTitle),
			Status:    getString(b, objects.FieldKeyStatus),
			Priority:  getString(b, "priority_tier"),
			ClaimedBy: getString(b, "claimed_by"),
		}
		reqRefs := extractStringSlice(b, objects.FieldKeyRequirementRefs)
		if len(reqRefs) == 0 {
			if singleReq := getString(b, "requirement_ref"); singleReq != "" {
				reqRefs = []string{singleReq}
			}
		}
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

func buildPriorityPlanNode(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, p map[string]any, baseReqMap map[string]RequirementNode) PriorityPlanNode {
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
	attachBacklogItemsToPlanRequirements(ctx, secCtx, sp, planID, reqMap)

	sortedReqIDs := make([]string, 0, len(reqMap))
	for rID := range reqMap {
		sortedReqIDs = append(sortedReqIDs, rID)
	}
	sort.Strings(sortedReqIDs)

	for _, rID := range sortedReqIDs {
		rNode := reqMap[rID]
		if len(rNode.BacklogItems) > 0 {
			sort.Slice(rNode.BacklogItems, func(i, j int) bool {
				return rNode.BacklogItems[i].ID < rNode.BacklogItems[j].ID
			})
			node.Requirements = append(node.Requirements, *rNode)
		}
	}
	return node
}

func loadActivePriorityPlans(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, baseReqMap map[string]RequirementNode) []PriorityPlanNode {
	plans, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.
		ActiveNotComplete(objects.KindPriorityPlan).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyWorkstreamRefs).
		Build())
	if err != nil {
		return nil
	}
	planNodes := make([]PriorityPlanNode, 0, len(plans.Objects))
	for _, p := range plans.Objects {
		planNodes = append(planNodes, buildPriorityPlanNode(ctx, secCtx, sp, p, baseReqMap))
	}
	sort.Slice(planNodes, func(i, j int) bool {
		return planNodes[i].ID < planNodes[j].ID
	})
	return planNodes
}

func buildStateTreePayload(ctx context.Context, sp storage.ObjectStorageProvider, projectRoot string) *StateTreePayload {
	payload := &StateTreePayload{}
	secCtx := pkgctx.GetSecurityContext(ctx)

	loadStateCoreEntities(ctx, secCtx, sp, payload)
	baseReqMap := loadBaseRequirements(ctx, secCtx, sp)
	payload.ActivePlans = loadActivePriorityPlans(ctx, secCtx, sp, baseReqMap)
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
		switch {
		case raw.EventType != "":
			cType = raw.EventType
		case raw.Kind != "":
			cType = raw.Kind
		default:
			cType = "event"
		}
	}

	ref := raw.ObjectRef
	if ref == "" {
		switch {
		case raw.Kind != "":
			ref = raw.Kind + ":" + raw.ID
		default:
			ref = raw.ID
		}
	}

	summary := raw.DiffSummary
	if summary == "" {
		switch {
		case raw.Operation != "":
			summary = raw.Operation
		case raw.Title != "":
			summary = raw.Title
		case raw.Instruction != "":
			summary = raw.Instruction
		default:
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

const (
	treePrefixOrg         = "├── Organization: %s [%s]\n"
	treePrefixMission     = "│   └── Mission: %s [%s]\n"
	treePrefixVision      = "│       └── Vision: %s [%s]\n"
	treePrefixGoal        = "│           └── Goal: %s [%s]\n"
	treePrefixPlan        = "│               └── Priority Plan: %s [%s] (%s)\n"
	treePrefixPlanEmpty   = "│               └── Priority Plan: (none active or in_progress)\n"
	treePrefixWorkstream  = "│                   ├── Workstream: %s\n"
	treePrefixRequirement = "│                   └── Requirement: %s [%s]\n"
	treePrefixBLI         = "│                       └── BLI: %s [%s] (%s)%s\n"

	treeHeaderMutations   = "└── Recent Journal Mutations:\n"
	treeEmptyMutations    = "    └── (no recent stream journal entries)\n"
	treePrefixMutationMid = "    ├──"
	treePrefixMutationEnd = "    └──"

	defaultOrgName     = "Default Organization"
	defaultOrgID       = "ORG-DEFAULT"
	defaultMissionName = "No Mission"
	defaultMissionID   = "MIS-NONE"
	defaultVisionName  = "No Vision"
	defaultVisionID    = "VIS-NONE"
	defaultGoalName    = "No Goal"
	defaultGoalID      = "GOAL-NONE"
)

func renderStateTreeText(cmd *cobra.Command, payload *StateTreePayload) error {
	return cli.WriteOutput(cmd, []byte(FormatStateTreeText(payload)))
}

func formatCoreEntities(buf *strings.Builder, payload *StateTreePayload) {
	orgName := defaultOrgName
	orgID := defaultOrgID
	if payload.Organization != nil {
		orgName = payload.Organization.Title
		orgID = payload.Organization.ID
	}
	fmt.Fprintf(buf, treePrefixOrg, orgName, orgID)

	misName := defaultMissionName
	misID := defaultMissionID
	if payload.Mission != nil {
		misName = payload.Mission.Title
		misID = payload.Mission.ID
	}
	fmt.Fprintf(buf, treePrefixMission, misName, misID)

	visName := defaultVisionName
	visID := defaultVisionID
	if payload.Vision != nil {
		visName = payload.Vision.Title
		visID = payload.Vision.ID
	}
	fmt.Fprintf(buf, treePrefixVision, visName, visID)

	goalName := defaultGoalName
	goalID := defaultGoalID
	if payload.Goal != nil {
		goalName = payload.Goal.Title
		goalID = payload.Goal.ID
	}
	fmt.Fprintf(buf, treePrefixGoal, goalName, goalID)
}

func formatPriorityPlans(buf *strings.Builder, payload *StateTreePayload) {
	if len(payload.ActivePlans) == 0 {
		buf.WriteString(treePrefixPlanEmpty)
		return
	}
	for _, plan := range payload.ActivePlans {
		fmt.Fprintf(buf, treePrefixPlan, plan.Title, plan.ID, plan.Status)
		for _, ws := range plan.Workstreams {
			fmt.Fprintf(buf, treePrefixWorkstream, ws)
		}
		for _, req := range plan.Requirements {
			fmt.Fprintf(buf, treePrefixRequirement, req.Title, req.ID)
			for _, bli := range req.BacklogItems {
				claimed := ""
				if bli.ClaimedBy != "" {
					claimed = fmt.Sprintf(" (leased: %s)", bli.ClaimedBy)
				}
				fmt.Fprintf(buf, treePrefixBLI, bli.Title, bli.ID, bli.Status, claimed)
			}
		}
	}
}

func formatRecentMutations(buf *strings.Builder, payload *StateTreePayload) {
	buf.WriteString(treeHeaderMutations)
	if len(payload.RecentMutations) == 0 {
		buf.WriteString(treeEmptyMutations)
		return
	}
	for i, m := range payload.RecentMutations {
		prefix := treePrefixMutationMid
		if i == len(payload.RecentMutations)-1 {
			prefix = treePrefixMutationEnd
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
