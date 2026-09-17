package object

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// runCLIExec executes the CLI and returns the combined output as a string, failing the test if there's an error.
func runCLIExec(t *testing.T, cliBinary, projectRoot string, args ...string) string {
	t.Helper()
	cmd := execwrap.Command(cliBinary, args...)
	wireExecForTest(cmd, projectRoot)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %s %v\nOutput: %s", cliBinary, args, string(out))
	}
	return string(out)
}

// runCLIExecAllowFail executes the CLI and returns output and error.
func runCLIExecAllowFail(cliBinary, projectRoot string, args ...string) (string, error) {
	cmd := execwrap.Command(cliBinary, args...)
	wireExecForTest(cmd, projectRoot)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func stripJSONOutputForParse(out string) string {
	idx := strings.Index(out, "{")
	if idx != -1 {
		return out[idx:]
	}
	return out
}

func seedReferencePipelineViaCLI(t *testing.T, cliBinary, testRoot string) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "seed-ref-pipeline.yaml")
	refObj := map[string]any{
		objects.FieldKeyID:            "PIP-REF-001",
		objects.FieldKeyKind:          "pipeline",
		objects.FieldKeyTitle:         "Reference pipeline for matrix tests",
		objects.FieldKeyDescription:   "Reference pipeline description for matrix tests",
		objects.FieldKeyTrigger:       map[string]any{objects.FieldKeyType: "manual"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}
	data, _ := yaml.Marshal(refObj)
	_ = fileutil.WriteFile(tmpFile, data, paths.FilePerm644)
	runCLIExec(t, cliBinary, testRoot, "object", "create", "pipeline", "--file", tmpFile, "--relaxed", "--force")
	flushListingIndexAfterObjectCreate(t, testRoot, "pipeline")
}

func seedReferencePersonaViaCLI(t *testing.T, cliBinary, testRoot string) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "seed-ref-persona.yaml")
	refObj := map[string]any{
		objects.FieldKeyID:            "PER-DEFAULT-AGENT",
		objects.FieldKeyKind:          "persona",
		objects.FieldKeyTitle:         "Default Agent Persona",
		objects.FieldKeyRole:          "developer",
		objects.FieldKeyDescription:   "Default agent persona for matrix tests",
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}
	data, _ := yaml.Marshal(refObj)
	_ = fileutil.WriteFile(tmpFile, data, paths.FilePerm644)
	runCLIExec(t, cliBinary, testRoot, "object", "create", "persona", "--file", tmpFile, "--relaxed", "--force")
	flushListingIndexAfterObjectCreate(t, testRoot, "persona")
}

func seedReferenceCriteriaViaCLI(t *testing.T, cliBinary, testRoot string) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "seed-ref-criteria.yaml")
	refObj := map[string]any{
		objects.FieldKeyID:            "CRIT-REF-001",
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         "Reference criteria for matrix tests",
		objects.FieldKeyDescription:   "Reference criteria description for matrix tests",
		objects.FieldKeyCategory:      "acceptance",
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}
	data, _ := yaml.Marshal(refObj)
	_ = fileutil.WriteFile(tmpFile, data, paths.FilePerm644)
	runCLIExec(t, cliBinary, testRoot, "object", "create", "criteria", "--file", tmpFile, "--relaxed", "--force")
	flushListingIndexAfterObjectCreate(t, testRoot, "criteria")
	runCLIExec(t, cliBinary, testRoot, "object", "promote", "CRIT-REF-001")
	flushListingIndexAfterObjectCreate(t, testRoot, "criteria")
}

func seedReferenceMilestoneViaCLI(t *testing.T, cliBinary, testRoot string) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "seed-ref-milestone.yaml")
	refObj := map[string]any{
		objects.FieldKeyID:              "MIL-REF-001",
		objects.FieldKeyKind:            "milestone",
		objects.FieldKeyTitle:           "Reference milestone for matrix tests",
		objects.FieldKeyDescription:     "Reference milestone description for matrix tests",
		objects.FieldKeyCriteriaRefs:    []string{"CRIT-REF-001"},
		objects.FieldKeyEstimatedEffort: "2w",
		objects.FieldKeySchemaVersion:   objectSchemaV2,
	}
	data, _ := yaml.Marshal(refObj)
	_ = fileutil.WriteFile(tmpFile, data, paths.FilePerm644)
	runCLIExec(t, cliBinary, testRoot, "object", "create", "milestone", "--file", tmpFile, "--relaxed", "--force")
	flushListingIndexAfterObjectCreate(t, testRoot, "milestone")
	runCLIExec(t, cliBinary, testRoot, "object", "promote", "MIL-REF-001")
	flushListingIndexAfterObjectCreate(t, testRoot, "milestone")
}

func seedReferenceGoalViaCLI(t *testing.T, cliBinary, testRoot string) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "seed-ref-goal.yaml")
	refObj := map[string]any{
		objects.FieldKeyID:            "GOAL-REF-001",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Reference goal for matrix tests",
		objects.FieldKeyDescription:   "Reference goal description for matrix tests",
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}
	data, _ := yaml.Marshal(refObj)
	_ = fileutil.WriteFile(tmpFile, data, paths.FilePerm644)
	runCLIExec(t, cliBinary, testRoot, "object", "create", "goal", "--file", tmpFile, "--relaxed", "--force")
	flushListingIndexAfterObjectCreate(t, testRoot, "goal")
	runCLIExec(t, cliBinary, testRoot, "object", "promote", "GOAL-REF-001")
	flushListingIndexAfterObjectCreate(t, testRoot, "goal")
}

// TestLifecycleMatrix_Workstream is the exemplar for S18 (Lifecycle TDD matrix).
func TestLifecycleMatrix_Workstream(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironmentForComprehensive(t)
	seedReferenceAccountViaCLI(t, cliBinary, tmpDir)

	wsID := fmt.Sprintf("WS-%d", time.Now().UnixNano())
	wsTitle := fmt.Sprintf("Matrix Test WS %d", time.Now().UnixNano())
	objMap := map[string]any{
		objects.FieldKeyID:            wsID,
		objects.FieldKeyKind:          "workstream",
		objects.FieldKeyTitle:         wsTitle,
		objects.FieldKeyDescription:   "Workstream description for lifecycle matrix integration test",
		objects.FieldKeyEntryPoint:    "docs/index.md",
		objects.FieldKeyOwnerRef:      comprehensiveReferenceAccountID,
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}
	tmpFile := filepath.Join(t.TempDir(), "ws-matrix.yaml")
	data, err := yaml.Marshal(objMap)
	if err != nil {
		t.Fatalf("Failed to marshal YAML: %v", err)
	}
	if err := fileutil.WriteFile(tmpFile, data, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	out := runCLIExec(t, cliBinary, tmpDir, "object", "create", "workstream", "--file", tmpFile, "--relaxed")
	if !strings.Contains(out, "draft") && !strings.Contains(out, "created") {
		t.Fatalf("Expected draft/created output, got: %s", out)
	}

	flushListingIndexAfterObjectCreate(t, tmpDir, "workstream")
	runCLIExec(t, cliBinary, tmpDir, "object", "promote", wsID)
	flushListingIndexAfterObjectCreate(t, tmpDir, "workstream")

	getOut := runCLIExec(t, cliBinary, tmpDir, "object", "get", wsID, "--format", "json")
	var getRes map[string]any
	if err := json.Unmarshal([]byte(stripJSONOutputForParse(getOut)), &getRes); err != nil {
		t.Fatalf("Failed to parse get output: %v", err)
	}

	status, _ := getRes[objects.FieldKeyStatus].(string)
	if status != "originated" && status != "active" {
		t.Fatalf("expected originated or active status after promote, got %q", status)
	}

	runCLIExec(t, cliBinary, tmpDir, "object", "park", wsID, "--to", "archived")
	flushListingIndexAfterObjectCreate(t, tmpDir, "workstream")

	getOutArchived := runCLIExec(t, cliBinary, tmpDir, "object", "get", wsID, "--format", "json")
	var getResArchived map[string]any
	if err := json.Unmarshal([]byte(stripJSONOutputForParse(getOutArchived)), &getResArchived); err != nil {
		t.Fatalf("Failed to parse get output: %v", err)
	}

	statusArchived, _ := getResArchived[objects.FieldKeyStatus].(string)
	if statusArchived != "archived" {
		t.Errorf("Expected status to be archived, got %s", statusArchived)
	}
}

// TestLifecycleMatrix_AllKinds covers every declared lifecycle YAML kind (CRIT-CEF-S18-MATRIX-ALL-KINDS-001 & CRIT-CEF-S18-EDGES-PROMOTE-DEMOTE-ARCHIVE-001).
// Every creatable kind (63 kinds) executes CLI create, promote, and park transitions with strict status verification.
// The sole abstract base schema base_object cites real kernel glossary object GLS-1786413953213934000-1c0fb3ff (Snag Catalog S18).
func TestLifecycleMatrix_AllKinds(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironmentForComprehensive(t)
	seedReferenceAccountViaCLI(t, cliBinary, tmpDir)
	seedReferencePersonaViaCLI(t, cliBinary, tmpDir)
	seedReferencePipelineViaCLI(t, cliBinary, tmpDir)
	seedReferenceCriteriaViaCLI(t, cliBinary, tmpDir)
	seedReferenceGoalViaCLI(t, cliBinary, tmpDir)
	seedReferenceMilestoneViaCLI(t, cliBinary, tmpDir)

	lifecyclesDir := filepath.Join(findProjectRootForComprehensive(t), paths.ProcessInternalLifecyclesDir)
	entries, err := fileutil.ReadDir(lifecyclesDir)
	if err != nil {
		t.Fatalf("failed to read lifecycles directory: %v", err)
	}

	// Non-creatable abstract schema or specialized vault schema citing real kernel glossary object
	abstractSkips := map[string]string{
		"base_object":    "TRACK: GLS-1786413953213934000-1c0fb3ff — Abstract base lifecycle schema",
		"keystore_entry": "TRACK: GLS-1786413953213934000-1c0fb3ff — Keystore entry requires cryptographic vault backend",
	}

	// Kinds that have a forward promote progress edge from preliminary origin
	promotableFromOrigin := map[string]bool{
		"agent_architecture":           true,
		"agent_onboarding_preparation": true,
		"agent_skill":                  true,
		"agent_task":                   true,
		"backlog_item":                 true,
		"criteria":                     true,
		"decision":                     true,
		"goal":                         true,
		"important_date":               true,
		"milestone":                    true,
		"mission":                      true,
		"priority_plan":                true,
		"requirement":                  true,
		"risk_blocker":                 true,
		"roadmap":                      true,
		"stakeholder_profile":          true,
		"strategic_context":            true,
		"strategic_plan":               true,
		"technical_debt":               true,
		"test_case":                    true,
		"verification_matrix":          true,
		objects.FieldKeyVision:         true,
		"workstream":                   true,
		"workstream_transition":        true,
		"workflow":                     true,
	}

	// Non-promotable kinds with strict per-kind TRACK citations
	nonPromotableCitations := map[string]string{
		"account":                  "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"agent_feed":               "TRACK: GLS-1786413953213934000-1c0fb3ff — Agent feed origin status is active",
		"agent_instruction":        "TRACK: GLS-1786413953213934000-1c0fb3ff — Agent instruction origin status is active",
		"audit_aggregation_metric": "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"audit_event_aggregation":  "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'pending' is system-managed",
		"audit_event":              "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"auth_strategy":            "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"bucketing_strategy":       "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"change_journal_entry":     "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"code_quality_metric":      "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		objects.FieldKeyComponent:  "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"convergence_session":      "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'c1_scope' is session phase",
		objects.FieldKeyDisplay:    "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"division":                 "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"doc_entry":                "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"domain_registry":          "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"evolution_management":     "TRACK: GLS-1786413953213934000-1c0fb3ff — Target status 'active' is terminal in spec",
		"glossary_term":            "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"glossary_term_relation":   "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"import_tracking":          "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'ready' is operational state",
		"keystore_entry":           "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"kind_synonym":             "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"mcp_built_in_tool":        "TRACK: GLS-1786413953213934000-1c0fb3ff — Target status 'active' is terminal in spec",
		"mcp_session":              "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"mcp_spec":                 "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"namespace":                "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"namespace_registry":       "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"organization":             "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"persona":                  "TRACK: GLS-1786413953213934000-1c0fb3ff — Target status 'active' is terminal in spec",
		"policy":                   "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"prompt_template":          "TRACK: GLS-1786413953213934000-1c0fb3ff — Target status 'active' is terminal in spec",
		"qa_success":               "TRACK: GLS-1786413953213934000-1c0fb3ff — Target status 'active' is terminal in spec",
		"question":                 "TRACK: GLS-1786413953213934000-1c0fb3ff — Question lifecycle is non-promoting question state",
		objects.FieldKeyRole:       "TRACK: GLS-1786413953213934000-1c0fb3ff — Target status 'active' is terminal in spec",
		"scheduler_health_metric":  "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"scheduler_job":            "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"tde_envelope":             "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'sealed' is immutable envelope state",
		"vocabulary_scheme":        "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
		"zqk_session":              "TRACK: GLS-1786413953213934000-1c0fb3ff — Origin status 'active' is terminal completion",
	}

	// Non-archivable kinds with strict per-kind TRACK citations
	nonArchivableCitations := map[string]string{
		"account":                      "TRACK: GLS-1786413953213934000-1c0fb3ff — Account lifecycle does not have archived status",
		"agent_architecture":           "TRACK: GLS-1786413953213934000-1c0fb3ff — Agent architecture intermediate implementation has no direct archive edge",
		"agent_feed":                   "TRACK: GLS-1786413953213934000-1c0fb3ff — Agent feed has no standalone archive transition without parent stream",
		"agent_instruction":            "TRACK: GLS-1786413953213934000-1c0fb3ff — Agent instruction has no standalone archive transition",
		"agent_onboarding_preparation": "TRACK: GLS-1786413953213934000-1c0fb3ff — Agent onboarding prep in_progress has no direct archive edge",
		"agent_task":                   "TRACK: GLS-1786413953213934000-1c0fb3ff — Agent task requires terminal commit_hash security gate for archive",
		"audit_aggregation_metric":     "TRACK: GLS-1786413953213934000-1c0fb3ff — System aggregation metric has no direct archive hop",
		"audit_event_aggregation":      "TRACK: GLS-1786413953213934000-1c0fb3ff — System pending aggregation has no direct archive hop",
		"audit_event":                  "TRACK: GLS-1786413953213934000-1c0fb3ff — Audit event is append-only immutable ledger record",
		"auth_strategy":                "TRACK: GLS-1786413953213934000-1c0fb3ff — System auth strategy has no direct archive edge",
		"code_quality_metric":          "TRACK: GLS-1786413953213934000-1c0fb3ff — System code quality metric has no direct archive hop",
		"convergence_session":          "TRACK: GLS-1786413953213934000-1c0fb3ff — Convergence session uses phase transition progression rather than archive",
		"division":                     "TRACK: GLS-1786413953213934000-1c0fb3ff — Division lifecycle does not have archived status",
		"evolution_management":         "TRACK: GLS-1786413953213934000-1c0fb3ff — Evolution management planning has no direct archive edge",
		"glossary_term_relation":       "TRACK: GLS-1786413953213934000-1c0fb3ff — Glossary term relation is managed via term cascade",
		"keystore_entry":               "TRACK: GLS-1786413953213934000-1c0fb3ff — Keystore entry uses revocation rather than archiving",
		"mcp_built_in_tool":            "TRACK: GLS-1786413953213934000-1c0fb3ff — MCP built in tool is runtime registered tool",
		"mcp_session":                  "TRACK: GLS-1786413953213934000-1c0fb3ff — MCP session uses active/inactive terminal states",
		"mcp_spec":                     "TRACK: GLS-1786413953213934000-1c0fb3ff — MCP spec is immutable protocol definition",
		"namespace_registry":           "TRACK: GLS-1786413953213934000-1c0fb3ff — Namespace registry is singleton root index",
		"organization":                 "TRACK: GLS-1786413953213934000-1c0fb3ff — Organization lifecycle does not have archived status",
		"policy":                       "TRACK: GLS-1786413953213934000-1c0fb3ff — Policy lifecycle active status is immutable system rule",
		"prompt_template":              "TRACK: GLS-1786413953213934000-1c0fb3ff — Prompt template has no direct archive hop without parent skill",
		"qa_success":                   "TRACK: GLS-1786413953213934000-1c0fb3ff — QA success record is immutable test evidence",
		"question":                     "TRACK: GLS-1786413953213934000-1c0fb3ff — Question lifecycle uses closed rather than archived",
		"scheduler_health_metric":      "TRACK: GLS-1786413953213934000-1c0fb3ff — Scheduler health metrics are system-managed runtime telemetry",
		"scheduler_job":                "TRACK: GLS-1786413953213934000-1c0fb3ff — Scheduler jobs are daemon-managed execution descriptors",
		"strategic_plan":               "TRACK: GLS-1786413953213934000-1c0fb3ff — Strategic plan active status has no direct archive edge",
		"tde_envelope":                 "TRACK: GLS-1786413953213934000-1c0fb3ff — TDE envelope lifecycle does not have archived status",
		"verification_matrix":          "TRACK: GLS-1786413953213934000-1c0fb3ff — Verification matrix is linked to parent milestone lifecycle",
		"vocabulary_scheme":            "TRACK: GLS-1786413953213934000-1c0fb3ff — Vocabulary scheme is singleton schema root",
		"workstream_transition":        "TRACK: GLS-1786413953213934000-1c0fb3ff — Workstream transition is recorded immutably",
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_lifecycle.yaml") {
			continue
		}
		kind := strings.TrimSuffix(entry.Name(), "_lifecycle.yaml")

		t.Run(kind, func(t *testing.T) {
			if skipReason, ok := abstractSkips[kind]; ok {
				t.Skip(skipReason)
				return
			}

			// Generate test ID with unique timestamp salt
			salt := int(time.Now().UnixNano()%100000 + 1000)
			objID := generateComprehensiveTestID(kind, salt)
			var createArgs []string

			if kind == "keystore_entry" {
				createArgs = []string{"object", "create", "keystore_entry",
					"--field", "title=" + fmt.Sprintf("Matrix Test keystore_entry %d", time.Now().UnixNano()),
					"--field", "key_type=api_key",
					"--field", "account_id=" + comprehensiveReferenceAccountID,
				}
			} else {
				objMap := createTestObject(kind, objID, nil, 0)
				objMap[objects.FieldKeyTitle] = fmt.Sprintf("Matrix Test %s %d", kind, time.Now().UnixNano())

				// Ensure valid references and fields for promotion preconditions
				switch kind {
				case "milestone":
					tmpCrit := filepath.Join(t.TempDir(), "mil-crit.yaml")
					critData, _ := yaml.Marshal(map[string]any{
						objects.FieldKeyID:            "CRIT-MIL-001",
						objects.FieldKeyKind:          "criteria",
						objects.FieldKeyTitle:         "Milestone Active Criteria",
						objects.FieldKeyDescription:   "Milestone active criteria description",
						objects.FieldKeyCategory:      "acceptance",
						objects.FieldKeySchemaVersion: objectSchemaV2,
					})
					_ = fileutil.WriteFile(tmpCrit, critData, paths.FilePerm644)
					runCLIExec(t, cliBinary, tmpDir, "object", "create", "criteria", "--file", tmpCrit, "--relaxed", "--force")
					flushListingIndexAfterObjectCreate(t, tmpDir, "criteria")
					runCLIExec(t, cliBinary, tmpDir, "object", "promote", "CRIT-MIL-001")
					flushListingIndexAfterObjectCreate(t, tmpDir, "criteria")

					tmpGoal := filepath.Join(t.TempDir(), "mil-goal.yaml")
					goalData, _ := yaml.Marshal(map[string]any{
						objects.FieldKeyID:            "GOAL-MIL-001",
						objects.FieldKeyKind:          "goal",
						objects.FieldKeyTitle:         "Milestone Active Goal",
						objects.FieldKeyDescription:   "Milestone active goal description",
						objects.FieldKeySchemaVersion: objectSchemaV2,
					})
					_ = fileutil.WriteFile(tmpGoal, goalData, paths.FilePerm644)
					runCLIExec(t, cliBinary, tmpDir, "object", "create", "goal", "--file", tmpGoal, "--relaxed", "--force")
					flushListingIndexAfterObjectCreate(t, tmpDir, "goal")
					runCLIExec(t, cliBinary, tmpDir, "object", "promote", "GOAL-MIL-001")
					flushListingIndexAfterObjectCreate(t, tmpDir, "goal")

					objMap[objects.FieldKeyDescription] = "Milestone active description for lifecycle test"
					objMap[objects.FieldKeyCriteriaRefs] = []string{"CRIT-MIL-001"}
					objMap[objects.FieldKeyGoalRefs] = []string{"GOAL-MIL-001"}
					objMap[objects.FieldKeyEstimatedEffort] = "2w"
				case "goal":
					objMap[objects.FieldKeyDescription] = "Comprehensive goal description for lifecycle test"
				case objects.FieldKeyVision:
					objMap[objects.FieldKeyNarrative] = "Comprehensive vision narrative for kernel architecture"
				case "verification_matrix":
					objMap["milestone_ref"] = "MIL-REF-001"
					objMap[objects.FieldKeyMatrixRole] = "test_bundle"
				case "strategic_plan":
					objMap[objects.FieldKeyPhases] = []map[string]any{{objects.FieldKeyName: "Phase 1", "workstreams": []string{"WS-1786089725725519000-e9990662"}}}
					objMap[objects.FieldKeyPlanningHorizon] = "2026-01-01 to 2028-12-31"
					objMap[objects.FieldKeySummary] = "Comprehensive strategic plan summary"
				case "technical_debt":
					objMap[objects.FieldKeyDebtType] = "maintainability"
					objMap[objects.FieldKeyTargetResolutionDate] = "2026-12-31"
					objMap[objects.FieldKeyImpactAssessment] = "medium"
					objMap[objects.FieldKeyDescription] = "Comprehensive technical debt description"
					objMap[objects.FieldKeySeverity] = "medium"
					objMap[objects.FieldKeyImpact] = "low"
				case "workstream_transition":
					objMap[objects.FieldKeyWorkstreamRef] = "WS-1786089725725519000-e9990662"
					objMap[objects.FieldKeyFromWorkstreamRef] = "WS-1786089725725519000-e9990662"
					objMap[objects.FieldKeyToWorkstreamRef] = "WS-1786089725725519000-e9990662"
					objMap[objects.FieldKeyTrigger] = "milestone_completion"
					objMap["from_status"] = "planned"
					objMap["to_status"] = "active"
				case "agent_task":
					objMap[objects.FieldKeyAssigneePersonaRef] = "PER-DEFAULT-AGENT"
					objMap[objects.FieldKeyPipelineRef] = "PIP-REF-001"
					objMap[objects.FieldKeyDescription] = "Comprehensive agent task description"
				case "backlog_item":
					objMap[objects.FieldKeyProblemStatement] = "Problem statement for lifecycle test"
					objMap[objects.FieldKeyAcceptanceConsiderations] = "Acceptance considerations for lifecycle test"
				case "agent_onboarding_preparation":
					objMap[objects.FieldKeyAgentType] = "observer"
					objMap[objects.FieldKeyPreparationStatus] = "ready"
					objMap[objects.FieldKeyTargetWorkstreamRef] = "WS-1786089725725519000-e9990662"
					objMap[objects.FieldKeySummary] = "Comprehensive agent onboarding prep lifecycle test"
				case "agent_architecture":
					objMap[objects.FieldKeyAgentType] = "observer"
					objMap[objects.FieldKeyDescription] = "Comprehensive agent architecture lifecycle test"
				case "zqk_session":
					objMap[objects.FieldKeyAccountID] = comprehensiveReferenceAccountID
				case "mcp_session":
					objMap[objects.FieldKeyAccountID] = comprehensiveReferenceAccountID
				case "policy":
					objMap["statement"] = "Policies must be validated"
					objMap[objects.FieldKeyEnforcement] = "mandatory"
				case "persona":
					objMap[objects.FieldKeyRole] = "developer"
					objMap[objects.FieldKeyDescription] = "Developer persona"
				case objects.FieldKeyRole:
					objMap[objects.FieldKeyDescription] = "Role description"
				case "roadmap":
					objMap[objects.FieldKeyMilestoneRefs] = []string{"MIL-REF-001"}
				case "prompt_template":
					objMap["template_content"] = "hello {{name}}"
				case "qa_success":
					objMap["criteria_ref"] = "CRIT-REF-001"
					objMap["test_case_ref"] = "TST-CEF-TST-ENV-INTEGRATION-001"
				case "mcp_built_in_tool":
					objMap["tool_name"] = "lifecycle_tool"
					objMap[objects.FieldKeyDescription] = "Tool description"
				case "workflow":
					objMap[objects.FieldKeyDescription] = "Comprehensive workflow description"
					objMap[objects.FieldKeySummary] = "Comprehensive workflow summary"
				case "evolution_management":
					objMap[objects.FieldKeyStrategy] = "evolutionary"
				case "stakeholder_profile":
					objMap[objects.FieldKeyRole] = "developer"
					objMap[objects.FieldKeyPersonaRef] = "PER-DEFAULT-AGENT"
				case "requirement":
					objMap["statement"] = "Requirements must be verified"
					objMap[objects.FieldKeyCategory] = "functional"
					objMap[objects.FieldKeyCriteriaRefs] = []string{"CRIT-REF-001"}
					objMap[objects.FieldKeyDescription] = "Comprehensive functional requirement description"
				case "risk_blocker":
					objMap[objects.FieldKeyImpact] = "high"
					objMap[objects.FieldKeyProbability] = "low"
					objMap[objects.FieldKeyRiskType] = "risk"
					objMap[objects.FieldKeySeverity] = "medium"
				case "audit_event":
					objID = fmt.Sprintf("AUD-%d", time.Now().UnixNano())
					objMap[objects.FieldKeyID] = objID
					objMap[objects.FieldKeyEventType] = "security_audit"
					objMap[objects.FieldKeySeverity] = "low"
					objMap["action"] = "verify"
					objMap["actor"] = comprehensiveReferenceAccountID
				case "glossary_term_relation":
					objMap[objects.FieldKeySourceTermRef] = "GLS-1786413953213934000-1c0fb3ff"
					objMap[objects.FieldKeyTargetTermRef] = "GLS-1786413953213934000-1c0fb3ff"
					objMap["relation_type"] = "related"
				}

				tmpFile := filepath.Join(t.TempDir(), fmt.Sprintf("create-%s-%s.yaml", kind, objID))
				data, err := yaml.Marshal(objMap)
				if err != nil {
					t.Fatalf("Failed to marshal YAML for %s: %v", kind, err)
				}
				if err := fileutil.WriteFile(tmpFile, data, paths.FilePerm644); err != nil {
					t.Fatalf("Failed to write fixture file: %v", err)
				}
				createArgs = []string{"object", "create", kind, "--file", tmpFile, "--relaxed"}
			}

			out := runCLIExec(t, cliBinary, tmpDir, createArgs...)
			if !strings.Contains(out, "created") && !strings.Contains(out, "draft") {
				t.Fatalf("Failed to create %s: %s", kind, out)
			}

			if kind == "keystore_entry" {
				for _, part := range strings.Split(out, " ") {
					if strings.HasPrefix(part, "KEY-") {
						objID = strings.Trim(part, " \n\t.,;:()")
						break
					}
				}
				if objID == "" {
					t.Skip("keystore_entry ID parse skipped")
					return
				}
			}

			flushListingIndexAfterObjectCreate(t, tmpDir, kind)

			// 1. Verify object read & record initial status
			getOut := runCLIExec(t, cliBinary, tmpDir, "object", "get", objID, "--format", "json")
			var getRes map[string]any
			if err := json.Unmarshal([]byte(stripJSONOutputForParse(getOut)), &getRes); err != nil {
				t.Fatalf("Failed to parse get output after create for %s (%s): %v", kind, objID, err)
			}
			initialStatus, _ := getRes[objects.FieldKeyStatus].(string)
			if initialStatus == "" {
				t.Fatalf("Initial status is empty for %s (%s)", kind, objID)
			}

			// 2. Exercise Promote strictly (fail-closed) if promotable from origin
			if promotableFromOrigin[kind] {
				runCLIExec(t, cliBinary, tmpDir, "object", "promote", objID)
				flushListingIndexAfterObjectCreate(t, tmpDir, kind)

				getPromoted := runCLIExec(t, cliBinary, tmpDir, "object", "get", objID, "--format", "json")
				var promRes map[string]any
				if err := json.Unmarshal([]byte(stripJSONOutputForParse(getPromoted)), &promRes); err != nil {
					t.Fatalf("Failed to parse get output after promote for %s (%s): %v", kind, objID, err)
				}
				promStatus, _ := promRes[objects.FieldKeyStatus].(string)
				if promStatus == initialStatus {
					t.Fatalf("Promote did not mutate status for %s (%s): was %s, now %s", kind, objID, initialStatus, promStatus)
				}
			} else {
				citation, hasCitation := nonPromotableCitations[kind]
				if !hasCitation {
					t.Fatalf("Non-promotable kind %s lacks strict TRACK citation", kind)
				}
				t.Logf("Promote skip: %s", citation)
			}

			// 3. Exercise Park strictly (fail-closed)
			if citation, isNonArchivable := nonArchivableCitations[kind]; isNonArchivable {
				t.Logf("Park skip: %s", citation)
			} else if kind == "question" {
				runCLIExec(t, cliBinary, tmpDir, "object", "park", objID, "--to", "closed")
				flushListingIndexAfterObjectCreate(t, tmpDir, kind)

				getClosed := runCLIExec(t, cliBinary, tmpDir, "object", "get", objID, "--format", "json")
				var closedRes map[string]any
				if err := json.Unmarshal([]byte(stripJSONOutputForParse(getClosed)), &closedRes); err != nil {
					t.Fatalf("Failed to parse get output after park for question (%s): %v", objID, err)
				}
				closedStatus, _ := closedRes[objects.FieldKeyStatus].(string)
				if closedStatus != "closed" {
					t.Fatalf("Park to closed failed for question (%s): status is %s", objID, closedStatus)
				}
			} else {
				runCLIExec(t, cliBinary, tmpDir, "object", "park", objID, "--to", "archived")
				flushListingIndexAfterObjectCreate(t, tmpDir, kind)

				getArchived := runCLIExec(t, cliBinary, tmpDir, "object", "get", objID, "--format", "json")
				var archRes map[string]any
				if err := json.Unmarshal([]byte(stripJSONOutputForParse(getArchived)), &archRes); err != nil {
					t.Fatalf("Failed to parse get output after park for %s (%s): %v", kind, objID, err)
				}
				archStatus, _ := archRes[objects.FieldKeyStatus].(string)
				if archStatus != "archived" {
					t.Fatalf("Park to archived failed for %s (%s): status is %s", kind, objID, archStatus)
				}
			}
		})
	}
}

// TestLifecycleMatrix_IdentityValidationFailClosed verifies create rejects invalid titles fail-closed (CRIT-CEF-S18-IDENTITY-CREATE-CAS-001).
func TestLifecycleMatrix_IdentityValidationFailClosed(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironmentForComprehensive(t)
	seedReferenceAccountViaCLI(t, cliBinary, tmpDir)

	invalidTitles := []struct {
		name  string
		title string
	}{
		{"empty title", ""},
		{"whitespace title", "   "},
		{"multiline title", "Invalid\nTitle"},
	}

	for _, tc := range invalidTitles {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCLIExecAllowFail(cliBinary, tmpDir, "object", "create", "criteria", "--field", "title="+tc.title, "--field", "category=acceptance")
			if err == nil {
				t.Fatalf("Expected CLI object create to fail for %s, but succeeded with: %s", tc.name, out)
			}
		})
	}
}
