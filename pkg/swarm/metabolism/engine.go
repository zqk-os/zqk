package metabolism

import (
	"crypto/ed25519"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// PromptTemplateObject represents an ingested prompt template ready for CAS storage.
type PromptTemplateObject struct {
	ID          string            `yaml:"id" json:"id"`
	Kind        string            `yaml:"kind" json:"kind"` // "prompt_template"
	Title       string            `yaml:"title" json:"title"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Category    string            `yaml:"category,omitempty" json:"category,omitempty"`
	Template    string            `yaml:"template" json:"template"`
	Variables   []string          `yaml:"variables,omitempty" json:"variables,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// IngestionOptions configures the metabolism engine ingestion run.
type IngestionOptions struct {
	PackDir     string
	PublicKey   ed25519.PublicKey
	SessionID   string
	OutputDir   string
	Parameters  map[string]interface{}
	VerifySeal  bool
	AccountID   string
}

// MetabolicDigest contains the fully decomposed kernel graph and active receptor lease.
type MetabolicDigest struct {
	URN             HolonURN
	Lease           *ReceptorLease
	Manifest        *pack.SwarmPackage
	PromptTemplates []PromptTemplateObject
	KernelObjects   []map[string]any
	Router          *DualStreamRouter
}

// MetabolismEngine coordinates the ingestion, membrane verification, and decomposition of swarm packages.
type MetabolismEngine struct {
	receptors *ReceptorRegistry
}

// NewMetabolismEngine initializes a metabolism engine.
func NewMetabolismEngine(registry *ReceptorRegistry) *MetabolismEngine {
	if registry == nil {
		registry = NewReceptorRegistry()
	}
	return &MetabolismEngine{
		receptors: registry,
	}
}

// Ingest metabolizes a swarm pack directory into CAS-ready kernel objects and dual-stream routing.
func (e *MetabolismEngine) Ingest(opts IngestionOptions) (*MetabolicDigest, error) {
	if opts.PackDir == "" {
		return nil, fmt.Errorf("pack directory is required")
	}
	if opts.SessionID == "" {
		opts.SessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}

	// 1. Immune Membrane & Verification Gate
	var manifest *pack.SwarmPackage
	var err error
	if opts.VerifySeal {
		manifest, err = pack.VerifyPackIntegrity(opts.PackDir, opts.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("immune membrane rejected pack: %w", err)
		}
	} else {
		manifestPath := filepath.Join(opts.PackDir, "swarm.yaml")
		manifest, err = pack.LoadManifestFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load swarm manifest: %w", err)
		}
	}

	contentDigest := ""
	if manifest.Integrity != nil {
		contentDigest = manifest.Integrity.ContentDigest
	} else {
		contentDigest, _ = pack.ComputePackDigest(opts.PackDir)
	}

	// 2. Receptor Saturation & Idempotency Gate
	urn, err := ComputeHolonURN(manifest.Name, manifest.Version, contentDigest, opts.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to compute holon URN: %w", err)
	}

	lease, err := e.receptors.Acquire(urn.String(), opts.SessionID)
	if err != nil {
		return nil, err
	}

	// 3. Dual-Stream Exhaust Setup
	router, err := NewDualStreamRouter(opts.OutputDir, nil)
	if err != nil {
		_ = e.receptors.Release(urn.String(), lease.LeaseID)
		return nil, fmt.Errorf("failed to initialize dual-stream exhaust: %w", err)
	}

	// 4. Ingest Prompt Templates from packDir/templates/
	var templates []PromptTemplateObject
	templatesDir := filepath.Join(opts.PackDir, "templates")
	if fileutil.Exists(templatesDir) {
		err := filepath.Walk(templatesDir, func(path string, info fileutil.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return walkErr
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".yaml" && ext != ".yml" && ext != ".md" {
				return nil
			}

			content, readErr := fileutil.ReadFile(path)
			if readErr != nil {
				return readErr
			}

			baseName := strings.TrimSuffix(filepath.Base(path), ext)
			cleanBaseName := strings.ToUpper(strings.ReplaceAll(baseName, "-", "_"))
			cleanName := strings.ToUpper(strings.ReplaceAll(manifest.Name, "-", "_"))
			tplID := fmt.Sprintf("TPL-%s-%s", cleanName, cleanBaseName)

			tplObj := PromptTemplateObject{
				ID:          tplID,
				Kind:        "prompt_template",
				Title:       fmt.Sprintf("Prompt Template for %s", baseName),
				Template:    string(content),
				Metadata: map[string]string{
					"pack_urn": urn.String(),
					"file":     filepath.Base(path),
				},
			}
			templates = append(templates, tplObj)
			return nil
		})
		if err != nil {
			_ = e.receptors.Release(urn.String(), lease.LeaseID)
			return nil, fmt.Errorf("failed to ingest prompt templates: %w", err)
		}
	}

	// 5. Synthesize Ontological Graph Lineage
	cleanName := strings.ToUpper(strings.ReplaceAll(manifest.Name, "-", "_"))
	goalID := fmt.Sprintf("GOAL-%s", cleanName)
	milestoneID := fmt.Sprintf("MIL-%s", cleanName)
	planID := fmt.Sprintf("PRI-%s", cleanName)
	cvsID := fmt.Sprintf("CVS-%s", cleanName)

	var kernelObjects []map[string]any

	effectiveParams := make(map[string]string)
	for k, def := range manifest.Parameters {
		if def.Default != nil {
			effectiveParams[k] = fmt.Sprintf("%v", def.Default)
		}
	}
	for k, v := range opts.Parameters {
		if v != nil {
			effectiveParams[k] = fmt.Sprintf("%v", v)
		}
	}

	baselineScore := 4.5
	if b, ok := opts.Parameters["baseline"].(float64); ok && b > 0 {
		baselineScore = b
	}

	// Layer 0: Workstream (Gantt execution lane for the swarm)
	wsID := fmt.Sprintf("WS-%s", cleanName)
	ownerRef := opts.AccountID
	if ownerRef == "" {
		ownerRef = pkgctx.SystemAccountID
	}
	entryPoint := filepath.Join(opts.PackDir, "swarm.yaml")
	wsObj := map[string]any{
		objects.FieldKeyID:          wsID,
		objects.FieldKeyKind:        "workstream",
		objects.FieldKeyTitle:       fmt.Sprintf("Workstream: %s", manifest.Name),
		objects.FieldKeyDescription: manifest.Description,
		"category":                  "feature",
		"owner_ref":                 ownerRef,
		"entry_point":               entryPoint,
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}
	kernelObjects = append(kernelObjects, wsObj)

	// Layer 1: Goal
	goalObj := map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        "goal",
		objects.FieldKeyTitle:       fmt.Sprintf("Swarm Goal: %s", manifest.Name),
		objects.FieldKeyDescription: manifest.Description,
		"metric":                     "quality_score",
		"target":                     fmt.Sprintf("%.1f", baselineScore),
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}
	kernelObjects = append(kernelObjects, goalObj)

	var allCritIDs []string
	var allReqIDs []string

	// Layer 2-4: For each task, synthesize Criteria (3-Fold) -> Req -> Test Case -> Backlog Item
	for _, task := range manifest.Tasks {
		taskClean := strings.ToUpper(strings.ReplaceAll(task.ID, "-", "_"))
		reqID := fmt.Sprintf("REQ-%s-%s", cleanName, taskClean)
		critInvID := fmt.Sprintf("CRIT-%s-%s-INV", cleanName, taskClean)
		critDynID := fmt.Sprintf("CRIT-%s-%s-DYN", cleanName, taskClean)
		critAdvID := fmt.Sprintf("CRIT-%s-%s-ADV", cleanName, taskClean)
		tstID := fmt.Sprintf("TST-%s-%s", cleanName, taskClean)
		bliID := fmt.Sprintf("BLI-%s-%s", cleanName, taskClean)

		allCritIDs = append(allCritIDs, critInvID, critDynID, critAdvID)
		allReqIDs = append(allReqIDs, reqID)

		// Criteria 1: Invariant
		critInv := map[string]any{
			objects.FieldKeyID:          critInvID,
			objects.FieldKeyKind:        "criteria",
			objects.FieldKeyTitle:       fmt.Sprintf("State Invariant for %s", task.ID),
			objects.FieldKeyDescription: fmt.Sprintf("Specification schema and output structure are strictly valid for %s", task.ID),
			"formula_type":              "invariant",
			"statement":                 fmt.Sprintf("Specification schema and output structure are strictly valid for %s", task.ID),
			objects.FieldKeyCategory:    "acceptance",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}
		// Criteria 2: Dynamic
		critDyn := map[string]any{
			objects.FieldKeyID:          critDynID,
			objects.FieldKeyKind:        "criteria",
			objects.FieldKeyTitle:       fmt.Sprintf("Dynamic Execution for %s", task.ID),
			objects.FieldKeyDescription: fmt.Sprintf("Agent task executes successfully and generates verifiable observations for %s", task.ID),
			"formula_type":              "dynamic",
			"statement":                 fmt.Sprintf("Agent task executes successfully and generates verifiable observations for %s", task.ID),
			objects.FieldKeyCategory:    "acceptance",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}
		// Criteria 3: Adversarial
		critAdv := map[string]any{
			objects.FieldKeyID:          critAdvID,
			objects.FieldKeyKind:        "criteria",
			objects.FieldKeyTitle:       fmt.Sprintf("Adversarial Boundary for %s", task.ID),
			objects.FieldKeyDescription: fmt.Sprintf("Non-conforming outputs or membrane transgressions fail closed for %s", task.ID),
			"formula_type":              "adversarial",
			"statement":                 fmt.Sprintf("Non-conforming outputs or membrane transgressions fail closed for %s", task.ID),
			objects.FieldKeyCategory:    "acceptance",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}
		kernelObjects = append(kernelObjects, critInv, critDyn, critAdv)

		// Requirement (persisted after criteria so criteria_refs exist)
		reqObj := map[string]any{
			objects.FieldKeyID:            reqID,
			objects.FieldKeyKind:          "requirement",
			objects.FieldKeyTitle:         task.Title,
			objects.FieldKeyDescription:   fmt.Sprintf("Satisfy specification for task %s (%s)", task.ID, task.Title),
			objects.FieldKeyGoalRefs:      []string{goalID},
			objects.FieldKeyCriteriaRefs:  []string{critInvID, critDynID, critAdvID},
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		}
		kernelObjects = append(kernelObjects, reqObj)

		// Test Case
		tstObj := map[string]any{
			objects.FieldKeyID:              tstID,
			objects.FieldKeyKind:            "test_case",
			objects.FieldKeyTitle:           fmt.Sprintf("Automated Confirmation for %s", task.ID),
			objects.FieldKeyPathOrID:        fmt.Sprintf("packs/%s/tasks/%s", manifest.Name, task.ID),
			objects.FieldKeyCriteriaRefs:    []string{critInvID, critDynID, critAdvID},
			objects.FieldKeyRequirementRefs: []string{reqID},
			objects.FieldKeyBacklogItemRefs:  []string{bliID},
			objects.FieldKeyGoalRefs:         []string{goalID},
			objects.FieldKeyStatus:          objects.ObjectStatusDraft,
		}
		kernelObjects = append(kernelObjects, tstObj)

		// Match template for task
		var matchedTemplate string
		taskKey := strings.ToLower(strings.TrimPrefix(task.ID, "wave-"))
		for _, pfx := range []string{"1-", "2-", "3-", "4-", "5-"} {
			taskKey = strings.TrimPrefix(taskKey, pfx)
		}
		if task.ID == "wave-1-preflight" || strings.Contains(taskKey, "preflight") {
			for _, t := range templates {
				if strings.Contains(strings.ToLower(t.Metadata["file"]), "kickoff") {
					matchedTemplate = t.Template
					break
				}
			}
		} else if strings.HasSuffix(taskKey, "-critique") {
			stem := strings.TrimSuffix(taskKey, "-critique")
			for _, t := range templates {
				if strings.Contains(strings.ToLower(t.Metadata["file"]), stem+"-adv") {
					matchedTemplate = t.Template
					break
				}
			}
		} else {
			for _, t := range templates {
				if strings.Contains(strings.ToLower(t.Metadata["file"]), taskKey+"-spec") {
					matchedTemplate = t.Template
					break
				}
			}
		}
		if matchedTemplate == "" && task.Role == "lead_integrator" {
			for _, t := range templates {
				if strings.Contains(strings.ToLower(t.Metadata["file"]), "integrator-spec") {
					matchedTemplate = t.Template
					break
				}
			}
		}

		bliDesc := fmt.Sprintf("Execute agent task %s in alignment with role %s", task.ID, task.Role)
		if matchedTemplate != "" {
			rendered := matchedTemplate
			for k, v := range effectiveParams {
				rendered = strings.ReplaceAll(rendered, "{{"+k+"}}", v)
			}
			bliDesc = rendered
		}

		// Backlog Item (child owns priority_plan_ref)
		bliObj := map[string]any{
			objects.FieldKeyID:             bliID,
			objects.FieldKeyKind:           "backlog_item",
			objects.FieldKeyTitle:          task.Title,
			objects.FieldKeyDescription:    bliDesc,
			objects.FieldKeyMilestoneRefs:  []string{milestoneID},
			objects.FieldKeyRequirementRefs: []string{reqID},
			objects.FieldKeyCriteriaRefs:   []string{critInvID, critDynID, critAdvID},
			objects.FieldKeyTestCaseRefs:   []string{tstID},
			objects.FieldKeyPriority:       "p1",
			objects.FieldKeyEstimatedEffort: "medium",
			objects.FieldKeyPriorityPlanRef: planID,
			objects.FieldKeyStatus:         objects.ObjectStatusPlanned,
		}
		var taskPersonas []string
		for _, a := range manifest.Agents {
			if a.Role == task.Role && a.Name != "" {
				pRef := a.Name
				if !strings.HasPrefix(pRef, "PER-") {
					pRef = fmt.Sprintf("PER-%s-%s", cleanName, strings.ToUpper(strings.ReplaceAll(a.Role, "-", "_")))
				}
				taskPersonas = append(taskPersonas, pRef)
			}
		}
		if len(taskPersonas) > 0 {
			bliObj[objects.FieldKeyPersonaRefs] = taskPersonas
		}
		kernelObjects = append(kernelObjects, bliObj)
	}

	// Milestone
	mlsObj := map[string]any{
		objects.FieldKeyID:          milestoneID,
		objects.FieldKeyKind:        "milestone",
		objects.FieldKeyTitle:       fmt.Sprintf("Milestone: %s Execution", manifest.Name),
		objects.FieldKeyDescription: fmt.Sprintf("Execute all tasks and evaluations declared by %s v%s", manifest.Name, manifest.Version),
		objects.FieldKeyGoalRefs:    []string{goalID},
		objects.FieldKeyCriteriaRefs: allCritIDs,
		objects.FieldKeyStatus:      objects.ObjectStatusNotStarted,
	}
	kernelObjects = append(kernelObjects, mlsObj)

	// Layer 5: Priority Plan (membership is child-owned via backlog_item.priority_plan_ref)
	priObj := map[string]any{
		objects.FieldKeyID:             planID,
		objects.FieldKeyKind:           "priority_plan",
		objects.FieldKeyTitle:          fmt.Sprintf("Execution Plan: %s", manifest.Name),
		objects.FieldKeyDescription:    manifest.Description,
		objects.FieldKeyWorkstreamRefs: []string{wsID},
		objects.FieldKeyStatus:         objects.ObjectStatusGrooming,
	}

	// Team configuration and persona dispatch wiring (supports ad-hoc personas or reusable team configurations)
	var personaRefs []string

	tcfgID := ""
	if manifest.TeamConfiguration != nil {
		tcfgID = manifest.TeamConfiguration.ID
		if tcfgID == "" {
			tcfgID = fmt.Sprintf("TCFG-%s", cleanName)
		}
		var allocations []map[string]any
		for _, alloc := range manifest.TeamConfiguration.PersonaAllocations {
			allocations = append(allocations, map[string]any{
				"persona_ref": alloc.PersonaRef,
				"role":        alloc.Role,
				"count":       alloc.Count,
			})
			if alloc.PersonaRef != "" {
				personaRefs = append(personaRefs, alloc.PersonaRef)
			}
		}
		tcfgObj := map[string]any{
			objects.FieldKeyID:                 tcfgID,
			objects.FieldKeyKind:               "team_configuration",
			objects.FieldKeyTitle:              fmt.Sprintf("Team Archetype for %s", manifest.Name),
			"cell_type":                         manifest.TeamConfiguration.CellType,
			"focus_area":                        manifest.TeamConfiguration.FocusArea,
			objects.FieldKeyPersonaAllocations: allocations,
			objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		}
		kernelObjects = append(kernelObjects, tcfgObj)
		priObj[objects.FieldKeyTeamConfigurationRef] = tcfgID
	} else if len(manifest.Agents) > 0 || manifest.TeamConfigurationRef != "" {
		tcfgID = manifest.TeamConfigurationRef
		if tcfgID == "" {
			tcfgID = fmt.Sprintf("TCFG-%s", cleanName)
		}
		var allocations []map[string]any
		for _, a := range manifest.Agents {
			pRef := a.Name
			if !strings.HasPrefix(pRef, "PER-") {
				pRef = fmt.Sprintf("PER-%s-%s", cleanName, strings.ToUpper(strings.ReplaceAll(a.Role, "-", "_")))
			}
			allocations = append(allocations, map[string]any{
				"persona_ref": pRef,
				"role":        a.Role,
				"count":       1,
			})
			personaRefs = append(personaRefs, pRef)
			personaObj := map[string]any{
				objects.FieldKeyID:                pRef,
				objects.FieldKeyKind:              objects.KindPersona,
				objects.FieldKeyTitle:             a.Name,
				objects.FieldKeyRole:              a.Role,
				objects.FieldKeyDescription:       a.SystemPrompt,
				"system_prompt":                   a.SystemPrompt,
				objects.FieldKeyRelatedObjectRefs: a.Skills,
				objects.FieldKeyStatus:            objects.ObjectStatusApproved,
			}
			kernelObjects = append(kernelObjects, personaObj)
		}
		tcfgObj := map[string]any{
			objects.FieldKeyID:                 tcfgID,
			objects.FieldKeyKind:               "team_configuration",
			objects.FieldKeyTitle:              fmt.Sprintf("Team Archetype for %s", manifest.Name),
			"cell_type":                         "neuron",
			"focus_area":                        manifest.Name,
			objects.FieldKeyPersonaAllocations: allocations,
			objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		}
		kernelObjects = append(kernelObjects, tcfgObj)
		priObj[objects.FieldKeyTeamConfigurationRef] = tcfgID
	}

	// Ensure persona_refs contains unique configured persona refs, falling back to defaults if empty
	var validPersonaRefs []string
	seenPersonas := make(map[string]bool)
	for _, p := range personaRefs {
		if p != "" && !seenPersonas[p] {
			seenPersonas[p] = true
			validPersonaRefs = append(validPersonaRefs, p)
		}
	}
	if len(validPersonaRefs) == 0 {
		validPersonaRefs = []string{objects.ConstPersonaDefaultAgent, objects.ConstPersonaDefaultOperator}
	}
	priObj[objects.FieldKeyPersonaRefs] = validPersonaRefs

	kernelObjects = append(kernelObjects, priObj)

	// Closed-Loop Convergence Session
	cvsObj := map[string]any{
		objects.FieldKeyID:              cvsID,
		objects.FieldKeyKind:            "convergence_session",
		objects.FieldKeyTitle:           fmt.Sprintf("Closed-Loop Convergence for %s", manifest.Name),
		"current_phase":                 "c1_scope",
		"outcome_character":             "pending",
		objects.FieldKeyRequirementRefs: allReqIDs,
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
	}
	kernelObjects = append(kernelObjects, cvsObj)

	return &MetabolicDigest{
		URN:             urn,
		Lease:           lease,
		Manifest:        manifest,
		PromptTemplates: templates,
		KernelObjects:   kernelObjects,
		Router:          router,
	}, nil
}

// CompleteIngestion marks the lease closed with the terminal outcome.
func (e *MetabolismEngine) CompleteIngestion(digest *MetabolicDigest, outcome string) error {
	if digest == nil || digest.Lease == nil {
		return nil
	}
	return e.receptors.Close(digest.URN.String(), digest.Lease.LeaseID, outcome)
}

// AbortIngestion releases the active lease without recording a completed outcome.
func (e *MetabolismEngine) AbortIngestion(digest *MetabolicDigest) error {
	if digest == nil || digest.Lease == nil {
		return nil
	}
	return e.receptors.Release(digest.URN.String(), digest.Lease.LeaseID)
}

// ExportPromptTemplate exports a prompt template to a YAML representation.
func ExportPromptTemplate(tpl PromptTemplateObject) ([]byte, error) {
	return yaml.Marshal(tpl)
}
