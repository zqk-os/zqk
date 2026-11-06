package workflow

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/scenario"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

// criteriaFacet defines a standard verification facet for multi-criteria base structure.
type criteriaFacet struct {
	suffix      string
	category    string
	description string
	testSuffix  string
}

// defaultCriteriaFacets provides the base multi-criteria structure for requirements.
// A requirement to criteria ratio is NOT 1:1; multiple orthogonal criteria are necessary
// to verify functional acceptance, boundary conditions/negative cases, documentation, and integration/conformance.
// TRACK: POL-DOC-001 mandates that every requirement must have a documentation criterion with doc_entry refs.
var defaultCriteriaFacets = []criteriaFacet{
	{
		suffix:      "Functional Acceptance",
		category:    "acceptance",
		description: "Primary functional acceptance criteria and core capability deliverables.",
		testSuffix:  "Functional Tests",
	},
	{
		suffix:      "Boundary & Error Handling",
		category:    "functional",
		description: "Boundary condition validation, negative testing, invalid input rejection, and failure recovery.",
		testSuffix:  "Boundary & Error Tests",
	},
	{
		suffix:      "Documentation & Knowledge Base Entry",
		category:    "compliance",
		description: "Complete feature documentation, user guides, or architectural specifications authored and linked via doc_entry reference (DOC-*).",
		testSuffix:  "Documentation Verification",
	},
	{
		suffix:      "Integration & Conformance",
		category:    "test",
		description: "System integration, contract conformance, observability, and regression verification.",
		testSuffix:  "Integration & Conformance Tests",
	},
}

// NewGenTracePipelineCmd creates the workflow gen-trace-pipeline command.
func NewGenTracePipelineCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowGenTracePipelineCommandBuilder()
	cmd.Flags().Int("criteria-count", 3, "Target number of base criteria to scaffold (default 3, minimum 2)")
	cli.BindAsyncProgress(cmd, runGenTracePipeline)
	cli.RequireStorage(cmd, true)
	return cmd
}

// generateTracePipelineBundle contains the deterministic core logic for building the missing pipeline components.
// It generates a multi-criteria base structure (default 3 criteria) because requirement:criteria is not 1:1.
// It is exposed for unit testing.
func generateTracePipelineBundle(targetID string, obj map[string]any, criteriaNeighbors []map[string]any, targetCriteriaCount ...int) (*scenario.Bundle, []string, bool) {
	title := "Untitled"
	if t, ok := obj[objects.FieldKeyTitle].(string); ok && strings.TrimSpace(t) != "" {
		title = t
	}

	minCriteria := 3
	if len(targetCriteriaCount) > 0 && targetCriteriaCount[0] >= 2 {
		minCriteria = targetCriteriaCount[0]
	}

	var existingCriteria []string
	if refs, ok := obj[objects.FieldKeyCriteriaRefs].([]any); ok {
		for _, r := range refs {
			s := strings.TrimSpace(fmt.Sprint(r))
			if s != "" && s != "<nil>" {
				existingCriteria = append(existingCriteria, s)
			}
		}
	}

	numCriteriaToGenerate := 0
	if len(existingCriteria) < minCriteria {
		numCriteriaToGenerate = minCriteria - len(existingCriteria)
	}
	needCriteria := numCriteriaToGenerate > 0

	var existingTestCases []string
	hasTestCase := false
	hasBLI := false
	for _, n := range criteriaNeighbors {
		k, _ := n["_kind"].(string)
		if k == objects.KindTestCase {
			hasTestCase = true
			if id, ok := n[objects.FieldKeyID].(string); ok && id != "" {
				existingTestCases = append(existingTestCases, id)
			}
		}
		if k == objects.KindBacklogItem {
			hasBLI = true
		}
	}
	if tcs, ok := obj[objects.FieldKeyTestCaseRefs].([]any); ok && len(tcs) > 0 {
		hasTestCase = true
		for _, tc := range tcs {
			s := strings.TrimSpace(fmt.Sprint(tc))
			if s != "" && s != "<nil>" {
				existingTestCases = append(existingTestCases, s)
			}
		}
	}
	if blis, ok := obj[objects.FieldKeyBacklogItemRefs].([]any); ok && len(blis) > 0 {
		hasBLI = true
	}

	needBLI := !hasBLI
	needTestCase := !hasTestCase

	allCriteria := append([]string(nil), existingCriteria...)

	if !needCriteria && !needTestCase && !needBLI {
		return nil, allCriteria, false // Fully established multi-criteria pipeline
	}

	bundle := &scenario.Bundle{
		APIVersion: "zqk.io/v2",
		Kind:       "Bundle",
		Metadata: scenario.BundleMeta{
			Name:        "trace-pipeline-gen",
			Description: fmt.Sprintf("Auto-generated multi-criteria traceability pipeline for %s", targetID),
		},
	}

	ts := time.Now().UnixNano()

	// When target is in CAS (non-preliminary), newly minted pipeline components must cross
	// the CAS membrane into CAS (originated) rather than parking on the draft plane (conceptual)
	// so that CAS requirements do not reference draft-plane objects ("crossing the streams").
	targetStatus, _ := obj[objects.FieldKeyStatus].(string)
	checker := objects.GetGlobalStatusChecker()
	pipelineStatus := objects.ObjectStatusConceptual
	if checker != nil && targetStatus != "" && !checker.IsPreliminary(objects.KindRequirement, targetStatus) {
		pipelineStatus = objects.ObjectStatusOriginated
	}

	targetReqID := targetID
	if strings.HasPrefix(targetID, "GOAL-") {
		var existingReqs []string
		for _, n := range criteriaNeighbors {
			if k, _ := n["_kind"].(string); k == objects.KindRequirement {
				if id, ok := n[objects.FieldKeyID].(string); ok && id != "" {
					existingReqs = append(existingReqs, id)
				}
			}
		}
		if len(existingReqs) == 0 {
			if rrefs, ok := obj[objects.FieldKeyRequirementRefs].([]any); ok {
				for _, r := range rrefs {
					s := strings.TrimSpace(fmt.Sprint(r))
					if s != "" && s != "<nil>" {
						existingReqs = append(existingReqs, s)
					}
				}
			}
		}
		if len(existingReqs) > 0 {
			targetReqID = existingReqs[0]
		} else {
			reqBytes := make([]byte, 4)
			_, _ = rand.Read(reqBytes)
			reqHint := fmt.Sprintf("REQ-%d-%x", ts+2, reqBytes)
			targetReqID = reqHint
			bundle.Objects.Requirements = append(bundle.Objects.Requirements, scenario.RequirementTemplate{
				ID:           reqHint,
				IDHint:       reqHint,
				Title:        fmt.Sprintf("Requirement: %s", title),
				Status:       pipelineStatus,
				GoalRefs:     []string{targetID},
				Priority:     "p1",
				DocEntryRefs: []string{},
			})
		}
	} else if strings.HasPrefix(targetID, "MIL-") {
		for _, n := range criteriaNeighbors {
			if k, _ := n["_kind"].(string); k == objects.KindRequirement {
				if id, ok := n[objects.FieldKeyID].(string); ok && id != "" {
					targetReqID = id
					break
				}
			}
		}
		if targetReqID == targetID {
			if rrefs, ok := obj[objects.FieldKeyRequirementRefs].([]any); ok && len(rrefs) > 0 {
				targetReqID = strings.TrimSpace(fmt.Sprint(rrefs[0]))
			}
		}
	}

	// Generate supplemental or full base criteria
	for i := 0; i < numCriteriaToGenerate; i++ {
		facetIdx := len(existingCriteria) + i
		var facet criteriaFacet
		if facetIdx < len(defaultCriteriaFacets) {
			facet = defaultCriteriaFacets[facetIdx]
		} else {
			facet = criteriaFacet{
				suffix:      fmt.Sprintf("Verification Condition %d", facetIdx+1),
				category:    "acceptance",
				description: fmt.Sprintf("Supplemental verification criterion %d", facetIdx+1),
				testSuffix:  fmt.Sprintf("Verification Suite %d", facetIdx+1),
			}
		}

		critBytes := make([]byte, 4)
		_, _ = rand.Read(critBytes)
		critHint := fmt.Sprintf("CRIT-%d-%x", ts+int64(i*1000), critBytes)

		allCriteria = append(allCriteria, critHint)

		critTemplate := scenario.CriteriaTemplate{
			ID:          critHint,
			IDHint:      critHint,
			Title:       fmt.Sprintf("Verify: %s - %s", title, facet.suffix),
			Category:    facet.category,
			Description: facet.description,
			Status:      pipelineStatus,
		}
		if strings.HasPrefix(targetReqID, "REQ-") {
			critTemplate.RequirementRef = targetReqID
			critTemplate.RequirementRefs = []string{targetReqID}
		}
		bundle.Objects.Criteria = append(bundle.Objects.Criteria, critTemplate)
	}

	allTestCases := append([]string(nil), existingTestCases...)

	// Generate 1 test_case object per requirement/target, containing all criteria in its CriteriaRefs
	if needTestCase {
		testBytes := make([]byte, 4)
		_, _ = rand.Read(testBytes)
		testHint := fmt.Sprintf("TST-%d-%x", ts+1, testBytes)
		allTestCases = append(allTestCases, testHint)

		var tcReqRefs []string
		if strings.HasPrefix(targetReqID, "REQ-") {
			tcReqRefs = []string{targetReqID}
		}

		bundle.Objects.TestCases = append(bundle.Objects.TestCases, scenario.TestCaseTemplate{
			ID:              testHint,
			IDHint:          testHint,
			Title:           fmt.Sprintf("Test Suite: %s", title),
			Status:          pipelineStatus,
			CriteriaRefs:    allCriteria,
			RequirementRefs: tcReqRefs,
			PathOrID:        "",
		})
	}

	// Generate atomic backlog items implementing criteria (Shift-Left 2-5 BLIs per plan mandate)
	if needBLI {
		atomicBLIs := generateAtomicBacklogItems(title, targetID, targetReqID, pipelineStatus, ts, allCriteria, allTestCases, obj, criteriaNeighbors)
		bundle.Objects.BacklogItems = append(bundle.Objects.BacklogItems, atomicBLIs...)
	}

	return bundle, allCriteria, needCriteria
}

// GenerateTracePipelineBundle is the exported form of generateTracePipelineBundle for testing and community callers.
func GenerateTracePipelineBundle(targetID string, obj map[string]any, criteriaNeighbors []map[string]any, targetCriteriaCount ...int) (*scenario.Bundle, []string, bool) {
	return generateTracePipelineBundle(targetID, obj, criteriaNeighbors, targetCriteriaCount...)
}

// extractGoalRefs extracts goal references from an object map in a type-safe manner.
func extractGoalRefs(obj map[string]any) []string {
	return objects.ExtractGoalRefs(obj)
}

func deduplicateStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" && !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return out
}

// generateAtomicBacklogItems slices criteria across discrete atomic units of effort.
func generateAtomicBacklogItems(
	title string,
	targetID string,
	targetReqID string,
	pipelineStatus string,
	ts int64,
	allCriteria []string,
	allTestCases []string,
	obj map[string]any,
	criteriaNeighbors ...[]map[string]any,
) []scenario.BacklogTemplate {
	var bliReqRefs []string
	if strings.HasPrefix(targetReqID, "REQ-") {
		bliReqRefs = []string{targetReqID}
	}
	var bliMilRefs []string
	if strings.HasPrefix(targetID, "MIL-") {
		bliMilRefs = []string{targetID}
	}
	var bliGoalRefs []string
	if strings.HasPrefix(targetID, "GOAL-") {
		bliGoalRefs = []string{targetID}
	} else {
		bliGoalRefs = extractGoalRefs(obj)
	}
	if len(bliGoalRefs) == 0 {
		for _, list := range criteriaNeighbors {
			for _, n := range list {
				k, _ := n["_kind"].(string)
				id, _ := n[objects.FieldKeyID].(string)
				if strings.HasPrefix(id, "GOAL-") || k == objects.KindGoal {
					gid := strings.TrimPrefix(id, "goal:")
					if gid != "" {
						bliGoalRefs = append(bliGoalRefs, gid)
					}
				} else if id == targetReqID || k == objects.KindRequirement || strings.HasPrefix(id, "REQ-") {
					if g := extractGoalRefs(n); len(g) > 0 {
						bliGoalRefs = append(bliGoalRefs, g...)
					}
				}
			}
		}
	}
	if len(bliGoalRefs) == 0 && strings.HasPrefix(targetReqID, "REQ-") {
		var sp storage.ObjectStorageProvider
		if zqkenv.IsInTest() {
			if testRoot := zqkenv.TestRoot().Get(); testRoot != "" {
				sp, _ = storage.GetFileObjectStorageForTest(testRoot)
			}
		} else {
			root := cli.ResolveProjectRoot(".")
			if root != "" {
				sp, _ = storage.GetFileObjectStorage(root)
			}
		}
		if sp != nil {
			if reqObj, rerr := sp.Read(context.Background(), nil, targetReqID); rerr == nil && reqObj != nil {
				bliGoalRefs = append(bliGoalRefs, extractGoalRefs(reqObj)...)
			}
		}
	}
	bliGoalRefs = deduplicateStrings(bliGoalRefs)

	if len(allCriteria) <= 1 {
		randomBytes := make([]byte, 4)
		_, _ = rand.Read(randomBytes)
		bliHint := fmt.Sprintf("BLI-%d-%x", ts, randomBytes)
		return []scenario.BacklogTemplate{{
			ID:              bliHint,
			IDHint:          bliHint,
			Title:           fmt.Sprintf("Implement: %s", title),
			Description:     fmt.Sprintf("Implementation and verification pipeline for: %s", title),
			Status:          pipelineStatus,
			CriteriaRefs:    allCriteria,
			RequirementRefs: bliReqRefs,
			MilestoneRefs:   bliMilRefs,
			GoalRefs:        bliGoalRefs,
			TestCaseRefs:    allTestCases,
			DocEntryRefs:    []string{},
			Priority:        "high",
			PriorityTier:    "P1",
		}}
	}

	suffixes := []struct {
		titleSuffix string
		descSuffix  string
	}{
		{
			titleSuffix: "Core Capability & Functional Delivery",
			descSuffix:  "Primary implementation and core deliverables",
		},
		{
			titleSuffix: "Boundary Validation & Error Recovery",
			descSuffix:  "Boundary handling, input validation, and negative testing",
		},
		{
			titleSuffix: "Architecture Specification & Conformance",
			descSuffix:  "Architectural documentation, specification, and compliance",
		},
	}

	var blis []scenario.BacklogTemplate
	for idx, critID := range allCriteria {
		randomBytes := make([]byte, 4)
		_, _ = rand.Read(randomBytes)
		bliHint := fmt.Sprintf("BLI-%d-%x", ts+int64((idx+1)*500), randomBytes)

		titleSuffix := fmt.Sprintf("Verification Facet %d", idx+1)
		descSuffix := fmt.Sprintf("Implementation and verification for facet %d", idx+1)
		if idx < len(suffixes) {
			titleSuffix = suffixes[idx].titleSuffix
			descSuffix = suffixes[idx].descSuffix
		}

		blis = append(blis, scenario.BacklogTemplate{
			ID:              bliHint,
			IDHint:          bliHint,
			Title:           fmt.Sprintf("Implement: %s - %s", title, titleSuffix),
			Description:     fmt.Sprintf("%s for: %s", descSuffix, title),
			Status:          pipelineStatus,
			CriteriaRefs:    []string{critID},
			RequirementRefs: bliReqRefs,
			MilestoneRefs:   bliMilRefs,
			GoalRefs:        bliGoalRefs,
			TestCaseRefs:    allTestCases,
			DocEntryRefs:    []string{},
			Priority:        "high",
			PriorityTier:    "P1",
		})
	}
	return blis
}

func validateTraceTargetKind(targetID string) error {
	if strings.HasPrefix(targetID, "REQ-") || strings.HasPrefix(targetID, "GOAL-") || strings.HasPrefix(targetID, "MIL-") {
		return nil
	}
	return fmt.Errorf("trace pipeline generation is only supported for requirements, goals, and milestones")
}

func runGenTracePipeline(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		if len(args) == 0 {
			return fmt.Errorf("missing object ID")
		}
		targetID := args[0]
		if err := validateTraceTargetKind(targetID); err != nil {
			return err
		}

		criteriaCount := 3
		if flagCount, err := cmd.Flags().GetInt("criteria-count"); err == nil && flagCount >= 2 {
			criteriaCount = flagCount
		}

		return applyGeneratedTracePipelineForID(cmd, proc, targetID, criteriaCount)
	})(cmd, args)
}

// ApplyGeneratedTracePipelineFromCmd scaffolds missing criteria, test cases, and
// backlog items for a requirement, goal, or milestone. Used by `new object` so TPM
// mints cannot skip the traceability chain.
// TRACK: POL-AGENT-TPM-TRACE-PIPELINE-001
func ApplyGeneratedTracePipelineFromCmd(cmd *cobra.Command, targetID string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		return applyGeneratedTracePipelineForID(cmd, proc, targetID, 3)
	})(cmd, nil)
}

func applyGeneratedTracePipelineForID(cmd *cobra.Command, proc *cli.Processor, targetID string, criteriaCount int) error {
	ctx, storageProvider, secCtx := processorContexts(proc)

	obj, err := storageProvider.Read(ctx, secCtx, targetID)
	if err != nil {
		return fmt.Errorf("failed to load target object %s: %w", targetID, err)
	}

	seenIDs := make(map[string]bool)
	var criteriaNeighbors []map[string]any
	appendNeighbor := func(n map[string]any) {
		id, _ := n[objects.FieldKeyID].(string)
		if id != "" && !seenIDs[id] {
			seenIDs[id] = true
			criteriaNeighbors = append(criteriaNeighbors, n)
		}
	}

	// 1. Direct incoming & outgoing neighbors for targetID (e.g. BLIs, TestCases, Goals, Requirements)
	if neighbors, nerr := storageProvider.GetNeighbors(ctx, secCtx, targetID, "both"); nerr == nil {
		for _, n := range neighbors {
			appendNeighbor(n)
		}
	} else {
		if inNeighbors, inErr := storageProvider.GetNeighbors(ctx, secCtx, targetID, "incoming"); inErr == nil {
			for _, n := range inNeighbors {
				appendNeighbor(n)
			}
		}
		if outNeighbors, outErr := storageProvider.GetNeighbors(ctx, secCtx, targetID, "outgoing"); outErr == nil {
			for _, n := range outNeighbors {
				appendNeighbor(n)
			}
		}
	}

	// 2. If target is a Milestone, inspect linked requirements and their outgoing neighbors
	if strings.HasPrefix(targetID, "MIL-") {
		var reqIDs []string
		if rrefs, ok := obj[objects.FieldKeyRequirementRefs].([]any); ok {
			for _, r := range rrefs {
				reqIDs = append(reqIDs, fmt.Sprint(r))
			}
		} else if rrefs, ok := obj[objects.FieldKeyRequirementRefs].([]string); ok {
			reqIDs = append(reqIDs, rrefs...)
		}
		for _, rID := range reqIDs {
			if reqObj, rerr := storageProvider.Read(ctx, secCtx, rID); rerr == nil && reqObj != nil {
				appendNeighbor(reqObj)
				if reqNeighbors, rnErr := storageProvider.GetNeighbors(ctx, secCtx, rID, "outgoing"); rnErr == nil {
					for _, rn := range reqNeighbors {
						appendNeighbor(rn)
					}
				}
			}
		}
	}

	// 3. If target is a Goal, inspect linked requirements and their incoming neighbors
	if strings.HasPrefix(targetID, "GOAL-") {
		for _, n := range criteriaNeighbors {
			if k, _ := n["_kind"].(string); k == objects.KindRequirement {
				if reqID, ok := n[objects.FieldKeyID].(string); ok && reqID != "" {
					if reqNeighbors, rerr := storageProvider.GetNeighbors(ctx, secCtx, reqID, "incoming"); rerr == nil {
						for _, rn := range reqNeighbors {
							appendNeighbor(rn)
						}
					}
					if reqObj, roErr := storageProvider.Read(ctx, secCtx, reqID); roErr == nil {
						if crs, ok := reqObj[objects.FieldKeyCriteriaRefs].([]any); ok {
							for _, cr := range crs {
								cid := fmt.Sprint(cr)
								if cNeighbors, cerr := storageProvider.GetNeighbors(ctx, secCtx, cid, "incoming"); cerr == nil {
									for _, cn := range cNeighbors {
										appendNeighbor(cn)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 3. For any criteria referenced directly on targetID, get incoming neighbors
	if refs, ok := obj[objects.FieldKeyCriteriaRefs].([]any); ok && len(refs) > 0 {
		for _, r := range refs {
			critID := fmt.Sprint(r)
			if neighbors, nerr := storageProvider.GetNeighbors(ctx, secCtx, critID, "incoming"); nerr == nil {
				for _, n := range neighbors {
					appendNeighbor(n)
				}
			}
		}
	}

	return applyGeneratedTracePipeline(cmd, proc, targetID, obj, criteriaNeighbors, criteriaCount)
}

func applyGeneratedTracePipeline(cmd *cobra.Command, proc *cli.Processor, targetID string, obj map[string]any, criteriaNeighbors []map[string]any, criteriaCount int) error {
	ctx, storageProvider, secCtx := processorContexts(proc)

	bundle, allCriteria, needCriteria := generateTracePipelineBundle(targetID, obj, criteriaNeighbors, criteriaCount)
	if bundle == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Pipeline is already fully established for %s (criteria: %s)\n", targetID, strings.Join(allCriteria, ", "))
		return nil
	}

	data, err := yaml.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal dynamically generated bundle: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Generating missing traceability pipeline components for %s...\n", targetID)

	if strings.HasPrefix(targetID, "REQ-") {
		coerceRequirementPriorityForCASUpdate(obj)
		if uerr := storageProvider.Update(ctx, secCtx, targetID, obj); uerr != nil {
			return fmt.Errorf("failed to normalize requirement %s before trace pipeline: %w", targetID, uerr)
		}
	}

	var targetKind string
	if strings.HasPrefix(targetID, "REQ-") {
		targetKind = objects.KindRequirement
	} else if strings.HasPrefix(targetID, "GOAL-") {
		targetKind = objects.KindGoal
	} else if strings.HasPrefix(targetID, "MIL-") {
		targetKind = objects.KindMilestone
	} else {
		targetKind = "unknown"
	}
	targetTitle, _ := obj[objects.FieldKeyTitle].(string)

	applyOpts := &scenario.ApplyOptions{
		Storage:           storageProvider,
		TargetObjectID:    targetID,
		TargetKind:        targetKind,
		TargetTitle:       targetTitle,
		TargetPreExisted:  true,
		LinkedExistingIDs: []string{targetID},
		LinkageNote:       fmt.Sprintf("Target %s %s pre-existed in Knowledge Kernel. Generated pipeline components are being linked to it.", targetKind, targetID),
	}

	stopPulse := pulseMeaningfulActivityWhileWaiting(proc)
	summary, err := scenario.ApplyScenarioBundle(ctx, proc.ProjectRoot(), bytes.NewReader(data), scenario.ApplyObjectsOnly, applyOpts)
	stopPulse()
	process.TouchMeaningfulActivity()

	if err != nil {
		return fmt.Errorf("failed to apply generated bundle: %w", err)
	}

	if strings.HasPrefix(targetID, "REQ-") && needCriteria && len(summary.CreatedCriteriaIDs) > 0 {
		// Ensure any newly generated criteria and test cases are in CAS before linking to a CAS requirement.
		if !storage.IsDraftPlaneOnly(proc.ProjectRoot(), objects.KindRequirement, targetID) {
			for _, cid := range summary.CreatedCriteriaIDs {
				if storage.IsDraftPlaneOnly(proc.ProjectRoot(), objects.KindCriteria, cid) {
					critObj, readErr := storageProvider.Read(ctx, secCtx, cid)
					if readErr == nil {
						critObj[objects.FieldKeyStatus] = objects.ObjectStatusOriginated
						_ = storageProvider.Update(ctx, secCtx, cid, critObj)
					}
				}
			}
			for _, tid := range summary.CreatedTestCaseIDs {
				if storage.IsDraftPlaneOnly(proc.ProjectRoot(), objects.KindTestCase, tid) {
					testObj, readErr := storageProvider.Read(ctx, secCtx, tid)
					if readErr == nil {
						testObj[objects.FieldKeyStatus] = objects.ObjectStatusOriginated
						_ = storageProvider.Update(ctx, secCtx, tid, testObj)
					}
				}
			}
			_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), objects.KindCriteria)
			_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), objects.KindTestCase)

			for _, cid := range summary.CreatedCriteriaIDs {
				_ = storage.SyncCASIndexMapping(proc.ProjectRoot(), objects.KindCriteria, cid)
			}
			for _, tid := range summary.CreatedTestCaseIDs {
				_ = storage.SyncCASIndexMapping(proc.ProjectRoot(), objects.KindTestCase, tid)
			}
		}

		reqRefs, ok := obj[objects.FieldKeyCriteriaRefs].([]any)
		if !ok {
			reqRefs = []any{}
		}
		for _, cid := range summary.CreatedCriteriaIDs {
			found := false
			for _, existing := range reqRefs {
				if fmt.Sprint(existing) == cid {
					found = true
					break
				}
			}
			if !found {
				reqRefs = append(reqRefs, cid)
			}
		}
		obj[objects.FieldKeyCriteriaRefs] = reqRefs
		coerceRequirementPriorityForCASUpdate(obj)

		if err := storageProvider.Update(ctx, secCtx, targetID, obj); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Warning: could not link requirement to new criteria due to lifecycle disparity: %v\n", err)
			fmt.Fprintf(cmd.OutOrStdout(), "Pipeline components were successfully generated but left unlinked in the draft plane.\n")
			summary.LinkageNote = fmt.Sprintf("Target requirement %s pre-existed. Pipeline components were generated but unlinked due to lifecycle disparity: %v", targetID, err)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Linked %d new criteria to %s: %s\n", len(summary.CreatedCriteriaIDs), targetID, strings.Join(summary.CreatedCriteriaIDs, ", "))
			summary.LinkageNote = fmt.Sprintf("Target requirement %s pre-existed in Knowledge Kernel. Successfully linked %d criteria (%s), %d test case (%s), and %d backlog item (%s) to it.",
				targetID,
				len(summary.CreatedCriteriaIDs), strings.Join(summary.CreatedCriteriaIDs, ", "),
				len(summary.CreatedTestCaseIDs), strings.Join(summary.CreatedTestCaseIDs, ", "),
				len(summary.CreatedBacklogItemIDs), strings.Join(summary.CreatedBacklogItemIDs, ", "),
			)
		}
		_ = scenario.EmitScenarioSummary(proc.ProjectRoot(), summary)
	}

	if strings.HasPrefix(targetID, "GOAL-") {
		if len(summary.CreatedRequirementIDs) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Minted requirement(s) referencing %s: %s\n", targetID, strings.Join(summary.CreatedRequirementIDs, ", "))
			summary.LinkageNote = fmt.Sprintf("Target goal %s pre-existed in Knowledge Kernel. Minted requirement (%s) with goal_refs and linked criteria, test cases, and backlog items to it.", targetID, strings.Join(summary.CreatedRequirementIDs, ", "))
		} else {
			summary.LinkageNote = fmt.Sprintf("Target goal %s pre-existed in Knowledge Kernel. Pipeline criteria, test cases, and backlog items linked to existing requirement hierarchy.", targetID)
		}
		_ = scenario.EmitScenarioSummary(proc.ProjectRoot(), summary)
	}

	if strings.HasPrefix(targetID, "MIL-") {
		summary.LinkageNote = fmt.Sprintf("Target milestone %s pre-existed in Knowledge Kernel. Generated pipeline backlog items linked to milestone.", targetID)
		_ = scenario.EmitScenarioSummary(proc.ProjectRoot(), summary)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pipeline updated successfully.\n")
	if len(summary.CreatedCriteriaIDs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  - Criteria (%d):\n", len(summary.CreatedCriteriaIDs))
		for _, cid := range summary.CreatedCriteriaIDs {
			fmt.Fprintf(cmd.OutOrStdout(), "    • %s\n", cid)
		}
	}
	if len(summary.CreatedTestCaseIDs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  - Test Cases (%d):\n", len(summary.CreatedTestCaseIDs))
		for _, tid := range summary.CreatedTestCaseIDs {
			fmt.Fprintf(cmd.OutOrStdout(), "    • %s\n", tid)
		}
	}
	if len(summary.CreatedBacklogItemIDs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  - Backlog Items (%d):\n", len(summary.CreatedBacklogItemIDs))
		for _, bid := range summary.CreatedBacklogItemIDs {
			fmt.Fprintf(cmd.OutOrStdout(), "    • %s\n", bid)
		}
	}

	return nil
}

// coerceRequirementPriorityForCASUpdate maps legacy critical/high/medium/low onto
// requirement spec p0–p3 so gen-trace-pipeline can Update criteria_refs.
// TRACK: POL-AGENT-TPM-TRACE-PIPELINE-001
func coerceRequirementPriorityForCASUpdate(obj map[string]any) {
	if obj == nil {
		return
	}
	raw, _ := obj[objects.FieldKeyPriority].(string)
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical", "p0":
		obj[objects.FieldKeyPriority] = "p0"
	case "high", "p1":
		obj[objects.FieldKeyPriority] = "p1"
	case "medium", "p2":
		obj[objects.FieldKeyPriority] = "p2"
	case "low", "p3":
		obj[objects.FieldKeyPriority] = "p3"
	}
}

// pulseMeaningfulActivityWhileWaiting keeps the CLI idle watchdog from canceling while
// graph application takes a long time. Returns a stop func.
func pulseMeaningfulActivityWhileWaiting(proc *cli.Processor) (stop func()) {
	process.TouchMeaningfulActivity()
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("gen_trace_pipeline_pulse", "touch meaningful activity during scenario apply").
		StartSimple(func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					process.TouchMeaningfulActivity()
				}
			}
		})
	return func() {
		close(done)
	}
}
