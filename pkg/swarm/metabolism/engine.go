package metabolism

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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
		err := filepath.Walk(templatesDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return walkErr
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".yaml" && ext != ".yml" && ext != ".md" {
				return nil
			}

			content, readErr := os.ReadFile(path)
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
	milestoneID := fmt.Sprintf("MLS-%s", cleanName)
	planID := fmt.Sprintf("PRI-%s", cleanName)
	cvsID := fmt.Sprintf("CVS-%s", cleanName)

	var kernelObjects []map[string]any

	// Layer 1: Goal
	goalObj := map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        "goal",
		objects.FieldKeyTitle:       fmt.Sprintf("Swarm Goal: %s", manifest.Name),
		objects.FieldKeyDescription: manifest.Description,
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}
	kernelObjects = append(kernelObjects, goalObj)

	// Milestone
	mlsObj := map[string]any{
		objects.FieldKeyID:          milestoneID,
		objects.FieldKeyKind:        "milestone",
		objects.FieldKeyTitle:       fmt.Sprintf("Milestone: %s Execution", manifest.Name),
		objects.FieldKeyDescription: fmt.Sprintf("Execute all tasks and evaluations declared by %s v%s", manifest.Name, manifest.Version),
		objects.FieldKeyGoalRefs:    []string{goalID},
		objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
	}
	kernelObjects = append(kernelObjects, mlsObj)

	var bliRefs []string

	// Layer 2-4: For each task, synthesize Req -> Criteria (3-Fold) -> Test Case -> Backlog Item
	for i, task := range manifest.Tasks {
		taskClean := strings.ToUpper(strings.ReplaceAll(task.ID, "-", "_"))
		reqID := fmt.Sprintf("REQ-%s-%s", cleanName, taskClean)
		critInvID := fmt.Sprintf("CRIT-%s-%s-INV", cleanName, taskClean)
		critDynID := fmt.Sprintf("CRIT-%s-%s-DYN", cleanName, taskClean)
		critAdvID := fmt.Sprintf("CRIT-%s-%s-ADV", cleanName, taskClean)
		tstID := fmt.Sprintf("TST-%s-%s", cleanName, taskClean)
		bliID := fmt.Sprintf("BLI-%s-%s", cleanName, taskClean)

		bliRefs = append(bliRefs, bliID)

		// Requirement
		reqObj := map[string]any{
			objects.FieldKeyID:          reqID,
			objects.FieldKeyKind:        "requirement",
			objects.FieldKeyTitle:       task.Title,
			objects.FieldKeyDescription: fmt.Sprintf("Satisfy specification for task %s (%s)", task.ID, task.Title),
			objects.FieldKeyGoalRefs:    []string{goalID},
			objects.FieldKeyCriteriaRefs: []string{critInvID, critDynID, critAdvID},
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
		}
		kernelObjects = append(kernelObjects, reqObj)

		// Criteria 1: Invariant
		critInv := map[string]any{
			objects.FieldKeyID:          critInvID,
			objects.FieldKeyKind:        "criteria",
			objects.FieldKeyTitle:       fmt.Sprintf("State Invariant for %s", task.ID),
			"formula_type":              "invariant",
			"statement":                 fmt.Sprintf("Specification schema and output structure are strictly valid for %s", task.ID),
			objects.FieldKeyRequirementRefs: []string{reqID},
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
		}
		// Criteria 2: Dynamic
		critDyn := map[string]any{
			objects.FieldKeyID:          critDynID,
			objects.FieldKeyKind:        "criteria",
			objects.FieldKeyTitle:       fmt.Sprintf("Dynamic Execution for %s", task.ID),
			"formula_type":              "dynamic",
			"statement":                 fmt.Sprintf("Agent task executes successfully and generates verifiable observations for %s", task.ID),
			objects.FieldKeyRequirementRefs: []string{reqID},
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
		}
		// Criteria 3: Adversarial
		critAdv := map[string]any{
			objects.FieldKeyID:          critAdvID,
			objects.FieldKeyKind:        "criteria",
			objects.FieldKeyTitle:       fmt.Sprintf("Adversarial Boundary for %s", task.ID),
			"formula_type":              "adversarial",
			"statement":                 fmt.Sprintf("Non-conforming outputs or membrane transgressions fail closed for %s", task.ID),
			objects.FieldKeyRequirementRefs: []string{reqID},
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
		}
		kernelObjects = append(kernelObjects, critInv, critDyn, critAdv)

		// Test Case
		tstObj := map[string]any{
			objects.FieldKeyID:          tstID,
			objects.FieldKeyKind:        "test_case",
			objects.FieldKeyTitle:       fmt.Sprintf("Automated Confirmation for %s", task.ID),
			objects.FieldKeyCriteriaRefs: []string{critInvID, critDynID, critAdvID},
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
		}
		kernelObjects = append(kernelObjects, tstObj)

		// Backlog Item
		bliObj := map[string]any{
			objects.FieldKeyID:             bliID,
			objects.FieldKeyKind:           "backlog_item",
			objects.FieldKeyTitle:          task.Title,
			objects.FieldKeyDescription:    fmt.Sprintf("Execute agent task %s in alignment with role %s", task.ID, task.Role),
			objects.FieldKeyMilestoneRefs:  []string{milestoneID},
			objects.FieldKeyRequirementRefs: []string{reqID},
			objects.FieldKeyCriteriaRefs:   []string{critInvID, critDynID, critAdvID},
			objects.FieldKeyTestCaseRefs:   []string{tstID},
			objects.FieldKeyPriority:       "p1",
			objects.FieldKeyEstimatedEffort: "medium",
			"sequence_index":               i + 1,
			objects.FieldKeyStatus:         objects.ObjectStatusPlanned,
		}
		kernelObjects = append(kernelObjects, bliObj)
	}

	// Layer 5: Priority Plan
	priObj := map[string]any{
		objects.FieldKeyID:             planID,
		objects.FieldKeyKind:           "priority_plan",
		objects.FieldKeyTitle:          fmt.Sprintf("Execution Plan: %s", manifest.Name),
		objects.FieldKeyDescription:    manifest.Description,
		objects.FieldKeyBacklogItemRefs: bliRefs,
		objects.FieldKeyMilestoneRefs:  []string{milestoneID},
		objects.FieldKeyGoalRefs:       []string{goalID},
		objects.FieldKeyStatus:         objects.ObjectStatusPlanned,
	}
	kernelObjects = append(kernelObjects, priObj)

	// Closed-Loop Convergence Session
	cvsObj := map[string]any{
		objects.FieldKeyID:             cvsID,
		objects.FieldKeyKind:           "convergence_session",
		objects.FieldKeyTitle:          fmt.Sprintf("Closed-Loop Convergence for %s", manifest.Name),
		"pack_urn":                     urn.String(),
		"evaluation_surface":           "cef_diamond_scorecard",
		objects.FieldKeyGoalRefs:       []string{goalID},
		objects.FieldKeyMilestoneRefs:  []string{milestoneID},
		objects.FieldKeyStatus:         "c1_intake",
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
