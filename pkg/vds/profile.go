// Package vds implements the Verifiable Decomposition Spine evaluator (POL-WORKFLOW-VDS).
package vds

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Default relative paths (portable across projects using zqk).
const (
	DefaultSpineProfileRel         = "docs/quality/verifiable_decomposition_spine_profile.yaml"
	DefaultCustomizationProfileRel = "docs/quality/verifiable_decomposition_customization.yaml"
	DefaultChunksRel               = "docs/quality/vds_chunks.yaml"
	SchemaEvaluate                 = "zqk_vds_evaluate_v1"
	SchemaChecklist                = "zqk_vds_checklist_v1"
	// PolicyID is the durable policy id (stable across projects that adopt VDS).
	// Glossary GLS-* CAS ids are project-local — pin them in customization.glossary or resolve by title.
	PolicyID = "POL-WORKFLOW-VDS"
)

// StageIDs is the rigid ordered spine.
var StageIDs = []string{
	"intent_capture",
	"design",
	"implement",
	"integrate_verify",
	"operate_release",
}

// SpineProfile is the rigid matrix profile.
type SpineProfile struct {
	SchemaVersion            int           `yaml:"schema_version" json:"schema_version"`
	ProfileID                string        `yaml:"profile_id" json:"profile_id"`
	CustomizationProfilePath string        `yaml:"customization_profile_path" json:"customization_profile_path"`
	Stages                   []StageDef    `yaml:"stages" json:"stages"`
	Completion               CompletionDef `yaml:"completion" json:"completion"`
	ChunkContract            ChunkContract `yaml:"chunk_contract" json:"chunk_contract"`
}

// StageDef describes one rigid stage.
type StageDef struct {
	ID      string `yaml:"id" json:"id"`
	Label   string `yaml:"label" json:"label"`
	Purpose string `yaml:"purpose" json:"purpose"`
}

// CompletionDef holds gate done values.
type CompletionDef struct {
	DoneValues []string `yaml:"done_values" json:"done_values"`
	HardGates  []string `yaml:"hard_gates" json:"hard_gates"`
}

// ChunkContract is the mandatory chunk field set.
type ChunkContract struct {
	RequiredFields []string `yaml:"required_fields" json:"required_fields"`
}

// Customization is project-local preferences (never deletes spine gates).
type Customization struct {
	SchemaVersion     int                 `yaml:"schema_version" json:"schema_version"`
	ProfileID         string              `yaml:"profile_id" json:"profile_id"`
	ProjectID         string              `yaml:"project_id" json:"project_id"`
	CodeStyle         CodeStylePrefs      `yaml:"code_style" json:"code_style"`
	TestExecution     TestExecutionPrefs  `yaml:"test_execution" json:"test_execution"`
	ProcessData       ProcessDataPrefs    `yaml:"process_data" json:"process_data"`
	Security          StageGatePrefs      `yaml:"security" json:"security"`
	Performance       StageGatePrefs      `yaml:"performance" json:"performance"`
	CICD              CICDPrefs           `yaml:"ci_cd" json:"ci_cd"`
	StageDSLTemplates map[string][]string `yaml:"stage_dsl_templates" json:"stage_dsl_templates"`
	// Glossary optionally pins this kernel's GLS-* ids (not portable; prefer title resolve).
	Glossary GlossaryBinding `yaml:"glossary" json:"glossary"`
	// VendorProviders configures instruction export targets (ide, etc.) — CLI stays generic.
	VendorProviders VendorProvidersConfig `yaml:"vendor_providers" json:"vendor_providers"`
}

// CodeStylePrefs holds lint/format preferences.
type CodeStylePrefs struct {
	LintCommands []string `yaml:"lint_commands" json:"lint_commands"`
	Formatter    string   `yaml:"formatter" json:"formatter"`
}

// TestExecutionPrefs holds how tests are verified.
type TestExecutionPrefs struct {
	Mode                             string `yaml:"mode" json:"mode"`
	SchedulerSubmitExample           string `yaml:"scheduler_submit_example" json:"scheduler_submit_example"`
	ForegroundProbeMaxTimeoutSeconds int    `yaml:"foreground_probe_max_timeout_seconds" json:"foreground_probe_max_timeout_seconds"`
	ForegroundProbeAllowed           bool   `yaml:"foreground_probe_allowed" json:"foreground_probe_allowed"`
}

// ProcessDataPrefs holds process mutation preferences.
type ProcessDataPrefs struct {
	Mutation          string   `yaml:"mutation" json:"mutation"`
	InstanceDataRoots []string `yaml:"instance_data_roots" json:"instance_data_roots"`
}

// StageGatePrefs lists stages and optional scan commands.
type StageGatePrefs struct {
	RequiredForStages []string `yaml:"required_for_stages" json:"required_for_stages"`
	ScanCommands      []string `yaml:"scan_commands" json:"scan_commands"`
	BudgetDoc         string   `yaml:"budget_doc" json:"budget_doc"`
}

// CICDPrefs holds CI/publish preferences.
type CICDPrefs struct {
	RequiredChecks     []string `yaml:"required_checks" json:"required_checks"`
	PublishRequiresAck bool     `yaml:"publish_requires_ack" json:"publish_requires_ack"`
}

// LoadSpine loads the spine profile from project root.
func LoadSpine(projectRoot, relPath string) (*SpineProfile, error) {
	rel := strings.TrimSpace(relPath)
	if rel == "" {
		rel = DefaultSpineProfileRel
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, rel)
	}
	b, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, errfmt.Errorf("vds: read spine profile %s: %w", path, err)
	}
	var p SpineProfile
	if err := yaml.Unmarshal(b, &p); err != nil {
		return nil, errfmt.Errorf("vds: parse spine profile: %w", err)
	}
	if len(p.Stages) == 0 {
		return nil, errfmt.Errorf("vds: spine profile has no stages: %s", path)
	}
	return &p, nil
}

// LoadCustomization loads project customization; missing file yields empty prefs (not an error).
func LoadCustomization(projectRoot, relPath string) (*Customization, string, error) {
	rel := strings.TrimSpace(relPath)
	if rel == "" {
		rel = DefaultCustomizationProfileRel
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, rel)
	}
	b, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return &Customization{}, path, nil
		}
		return nil, path, errfmt.Errorf("vds: read customization %s: %w", path, err)
	}
	var c Customization
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, path, errfmt.Errorf("vds: parse customization: %w", err)
	}
	return &c, path, nil
}

// ResolveProfiles loads spine then customization path from spine (or defaults).
func ResolveProfiles(projectRoot, spineRel, customRel string) (*SpineProfile, *Customization, error) {
	spine, err := LoadSpine(projectRoot, spineRel)
	if err != nil {
		return nil, nil, err
	}
	custRel := strings.TrimSpace(customRel)
	if custRel == "" {
		custRel = strings.TrimSpace(spine.CustomizationProfilePath)
	}
	if custRel == "" {
		custRel = DefaultCustomizationProfileRel
	}
	cust, _, err := LoadCustomization(projectRoot, custRel)
	if err != nil {
		return nil, nil, err
	}
	return spine, cust, nil
}

// QualityDir returns docs/quality under project root.
func QualityDir(projectRoot string) string {
	return filepath.Join(projectRoot, paths.DocsQualityDir)
}
