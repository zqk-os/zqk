package quality

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
	"github.com/zqk-os/zqk/pkg/workflow"
)

// RemediationBundle contains the synthesized backlog item, requirement, criteria, and test case
// ready to insert into CAS for an accepted defect.
type RemediationBundle struct {
	BacklogItem map[string]any   `json:"backlog_item"`
	Requirement map[string]any   `json:"requirement"`
	Criteria    []map[string]any `json:"criteria"`
	TestCase    map[string]any   `json:"test_case"`
}

// FindingToRemediationBLI converts an evaluation finding into a complete shovel-ready remediation bundle.
func FindingToRemediationBLI(finding metabolism.Finding, milestoneID string) (*RemediationBundle, error) {
	if finding.ID == "" || finding.Title == "" {
		return nil, fmt.Errorf("finding must have non-empty ID and Title")
	}

	cleanID := strings.ToUpper(strings.ReplaceAll(finding.ID, "-", "_"))
	bliID := fmt.Sprintf("BLI-REMEDIATE-%s", cleanID)
	reqID := fmt.Sprintf("REQ-REMEDIATE-%s", cleanID)
	tstID := fmt.Sprintf("TST-REMEDIATE-%s", cleanID)
	critInvID := fmt.Sprintf("CRIT-REMEDIATE-%s-INV", cleanID)
	critDynID := fmt.Sprintf("CRIT-REMEDIATE-%s-DYN", cleanID)
	critAdvID := fmt.Sprintf("CRIT-REMEDIATE-%s-ADV", cleanID)

	priority := "p2"
	switch strings.ToUpper(finding.Severity) {
	case "E0":
		priority = "p0"
	case "E1":
		priority = "p1"
	case "E2":
		priority = "p2"
	case "E3":
		priority = "p3"
	}

	// 1. Synthesize Requirement
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         "requirement",
		objects.FieldKeyTitle:        fmt.Sprintf("Resolve %s Defect: %s", finding.Lens, finding.Title),
		objects.FieldKeyDescription:  fmt.Sprintf("Remediate defect %s observed in lens %s. Root cause: %s. Action: %s", finding.ID, finding.Lens, finding.Description, finding.Remediation),
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyCriteriaRefs: []string{critInvID, critDynID, critAdvID},
	}

	// 2. Synthesize Three-Fold Criteria
	critInv := map[string]any{
		objects.FieldKeyID:          critInvID,
		objects.FieldKeyKind:        "criteria",
		objects.FieldKeyTitle:       fmt.Sprintf("State Invariant for %s", finding.ID),
		objects.FieldKeyDescription: fmt.Sprintf("Static schema invariants and configuration adhere to %s requirements without regression, with documentation recorded in doc_entry knowledge base.", finding.Lens),
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}

	critDyn := map[string]any{
		objects.FieldKeyID:          critDynID,
		objects.FieldKeyKind:        "criteria",
		objects.FieldKeyTitle:       fmt.Sprintf("Dynamic Fix Verification for %s", finding.ID),
		objects.FieldKeyDescription: fmt.Sprintf("Remediated code compiles and executes dynamic tests proving resolution of %s.", finding.Title),
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}

	critAdv := map[string]any{
		objects.FieldKeyID:          critAdvID,
		objects.FieldKeyKind:        "criteria",
		objects.FieldKeyTitle:       fmt.Sprintf("Adversarial Regression Guard for %s", finding.ID),
		objects.FieldKeyDescription: fmt.Sprintf("Adversarial inputs reproducing defect %s fail closed and prevent regression.", finding.ID),
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}

	criteriaList := []map[string]any{critInv, critDyn, critAdv}

	// Verify quality through anti-superficiality linter
	if err := workflow.CheckShovelReadyQuality(reqObj, criteriaList); err != nil {
		return nil, fmt.Errorf("synthesized remediation requirement failed quality linter: %w", err)
	}

	// 3. Synthesize Test Case
	tstObj := map[string]any{
		objects.FieldKeyID:           tstID,
		objects.FieldKeyKind:         "test_case",
		objects.FieldKeyTitle:        fmt.Sprintf("Verification Suite for %s", finding.ID),
		objects.FieldKeyCriteriaRefs: []string{critInvID, critDynID, critAdvID},
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
	}

	// 4. Synthesize Backlog Item
	bliObj := map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeyTitle:           fmt.Sprintf("Remediate [%s] %s", finding.Lens, finding.Title),
		objects.FieldKeyDescription:     fmt.Sprintf("Fix defect %s (%s). Evidence: %s. Remediation: %s", finding.ID, finding.Title, finding.Evidence, finding.Remediation),
		objects.FieldKeyMilestoneRefs:   []string{milestoneID},
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyCriteriaRefs:    []string{critInvID, critDynID, critAdvID},
		objects.FieldKeyTestCaseRefs:    []string{tstID},
		objects.FieldKeyPriority:        priority,
		objects.FieldKeyEstimatedEffort: "medium",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
	}

	return &RemediationBundle{
		BacklogItem: bliObj,
		Requirement: reqObj,
		Criteria:    criteriaList,
		TestCase:    tstObj,
	}, nil
}
