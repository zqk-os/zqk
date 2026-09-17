package workflow

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/lanceman/zqk/pkg/scenario"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
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
// to verify functional acceptance, boundary conditions/negative cases, and integration/conformance.
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

	hasTestCase := false
	hasBLI := false
	for _, n := range criteriaNeighbors {
		k, _ := n["_kind"].(string)
		if k == objects.KindTestCase {
			hasTestCase = true
		}
		if k == objects.KindBacklogItem {
			hasBLI = true
		}
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
		if strings.HasPrefix(targetID, "REQ-") {
			critTemplate.RequirementRef = targetID
			critTemplate.RequirementRefs = []string{targetID}
		}
		bundle.Objects.Criteria = append(bundle.Objects.Criteria, critTemplate)
	}

	// Generate 1 test_case object per requirement/target, containing all criteria in its CriteriaRefs
	if needTestCase {
		testBytes := make([]byte, 4)
		_, _ = rand.Read(testBytes)
		testHint := fmt.Sprintf("TST-%d-%x", ts+1, testBytes)
		bundle.Objects.TestCases = append(bundle.Objects.TestCases, scenario.TestCaseTemplate{
			ID:           testHint,
			IDHint:       testHint,
			Title:        fmt.Sprintf("Test Suite: %s", title),
			Status:       pipelineStatus,
			CriteriaRefs: allCriteria,
			PathOrID:     "pkg/dummy/path_test.go",
		})
	}

	// Generate backlog item implementing all criteria
	if needBLI {
		randomBytes := make([]byte, 4)
		_, _ = rand.Read(randomBytes)
		bliHint := fmt.Sprintf("BLI-%d-%x", ts, randomBytes)
		bundle.Objects.BacklogItems = append(bundle.Objects.BacklogItems, scenario.BacklogTemplate{
			ID:              bliHint,
			IDHint:          bliHint,
			Title:           fmt.Sprintf("Implement: %s", title),
			Description:     fmt.Sprintf("Implementation and verification pipeline for: %s", title),
			Status:          pipelineStatus,
			CriteriaRefs:    allCriteria,
			RequirementRefs: []string{targetID},
			Priority:        "high",
			PriorityTier:    "P1",
		})
	}

	return bundle, allCriteria, needCriteria
}

func runGenTracePipeline(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		storageProvider := proc.Storage()
		secCtx := proc.SecurityContext()

		if len(args) == 0 {
			return fmt.Errorf("missing object ID")
		}
		targetID := args[0]

		obj, err := storageProvider.Read(ctx, secCtx, targetID)
		if err != nil {
			return fmt.Errorf("failed to load target object %s: %w", targetID, err)
		}

		var kind string
		if strings.HasPrefix(targetID, "REQ-") {
			kind = objects.KindRequirement
		} else if strings.HasPrefix(targetID, "GOAL-") {
			kind = objects.KindGoal
		} else if strings.HasPrefix(targetID, "MIL-") {
			kind = objects.KindMilestone
		} else {
			return fmt.Errorf("trace pipeline generation is only supported for requirements, goals, and milestones")
		}

		var criteriaNeighbors []map[string]any
		if refs, ok := obj[objects.FieldKeyCriteriaRefs].([]any); ok && len(refs) > 0 {
			for _, r := range refs {
				critID := fmt.Sprint(r)
				neighbors, err := storageProvider.GetNeighbors(ctx, secCtx, critID, "incoming")
				if err == nil {
					criteriaNeighbors = append(criteriaNeighbors, neighbors...)
				}
			}
		}

		criteriaCount := 3
		if flagCount, err := cmd.Flags().GetInt("criteria-count"); err == nil && flagCount >= 2 {
			criteriaCount = flagCount
		}

		_ = kind
		return applyGeneratedTracePipeline(cmd, proc, targetID, obj, criteriaNeighbors, criteriaCount)
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
	ctx := proc.OperationContext()
	storageProvider := proc.Storage()
	secCtx := proc.SecurityContext()

	obj, err := storageProvider.Read(ctx, secCtx, targetID)
	if err != nil {
		return fmt.Errorf("failed to load target object %s: %w", targetID, err)
	}

	var criteriaNeighbors []map[string]any
	if refs, ok := obj[objects.FieldKeyCriteriaRefs].([]any); ok && len(refs) > 0 {
		for _, r := range refs {
			critID := fmt.Sprint(r)
			neighbors, nerr := storageProvider.GetNeighbors(ctx, secCtx, critID, "incoming")
			if nerr == nil {
				criteriaNeighbors = append(criteriaNeighbors, neighbors...)
			}
		}
	}

	return applyGeneratedTracePipeline(cmd, proc, targetID, obj, criteriaNeighbors, criteriaCount)
}

func applyGeneratedTracePipeline(cmd *cobra.Command, proc *cli.Processor, targetID string, obj map[string]any, criteriaNeighbors []map[string]any, criteriaCount int) error {
	ctx := proc.OperationContext()
	storageProvider := proc.Storage()
	secCtx := proc.SecurityContext()

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

	stopPulse := pulseMeaningfulActivityWhileWaiting(proc)
	summary, err := scenario.ApplyScenarioBundle(ctx, proc.ProjectRoot(), bytes.NewReader(data), scenario.ApplyObjectsOnly, &scenario.ApplyOptions{Storage: storageProvider})
	stopPulse()
	process.TouchMeaningfulActivity()

	if err != nil {
		return fmt.Errorf("failed to apply generated bundle: %w", err)
	}

	if strings.HasPrefix(targetID, "REQ-") && needCriteria && len(summary.CreatedCriteriaIDs) > 0 {
		// Ensure any newly generated criteria are in CAS before linking to a CAS requirement.
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
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Linked %d new criteria to %s: %s\n", len(summary.CreatedCriteriaIDs), targetID, strings.Join(summary.CreatedCriteriaIDs, ", "))
		}
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
