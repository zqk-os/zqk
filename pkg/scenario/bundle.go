package scenario

import (
	"errors"
	"io"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"gopkg.in/yaml.v3"
)

const (
	errDecodeBundleFmt   = "decode bundle: %w"
	errMissingAPIVersion = "bundle missing api_version"
	errMissingKind       = "bundle missing kind"
	emptyBundleValue     = ""
)

// ApplyMode controls whether a scenario bundle applies only objects or
// objects and executable steps.
type ApplyMode int

const (
	ApplyObjectsOnly ApplyMode = iota
	ApplyObjectsAndSteps
)

// Bundle represents the top-level scenario or traceability bundle document.
// It is intentionally generic so it can be used for:
//   - Traceability bundles (requirements, criteria, backlog, test cases, fixtures).
//   - Execution scenarios (what currently lives under test-scenarios/).
//   - Future seed/maintenance bundles.
type Bundle struct {
	APIVersion string        `yaml:"api_version" json:"api_version"`
	Kind       string        `yaml:"kind" json:"kind"`
	Metadata   BundleMeta    `yaml:"metadata" json:"metadata"`
	Objects    BundleObjects `yaml:"objects" json:"objects"`
	Steps      []BundleStep  `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// BundleMeta holds descriptive fields for a bundle.
type BundleMeta struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// BundleObjects groups all object templates that a bundle may create.
type BundleObjects struct {
	Goals               []GoalTemplate               `yaml:"goals,omitempty" json:"goals,omitempty"`
	Requirements        []RequirementTemplate        `yaml:"requirements,omitempty" json:"requirements,omitempty"`
	Criteria            []CriteriaTemplate           `yaml:"criteria,omitempty" json:"criteria,omitempty"`
	BacklogItems        []BacklogTemplate            `yaml:"backlog_items,omitempty" json:"backlog_items,omitempty"`
	TestCases           []TestCaseTemplate           `yaml:"test_cases,omitempty" json:"test_cases,omitempty"`
	DocEntries          []DocEntryTemplate           `yaml:"doc_entries,omitempty" json:"doc_entries,omitempty"`
	ConvergenceSessions []ConvergenceSessionTemplate `yaml:"convergence_sessions,omitempty" json:"convergence_sessions,omitempty"`
	Fixtures            FixtureGroup                 `yaml:"fixtures,omitempty" json:"fixtures,omitempty"`
	RawExtensions       map[string]any               `yaml:",inline" json:"-"` // for future extension without breaking schema
}

// GoalTemplate describes a goal object to be created from a bundle (required by requirement.goal_refs).
type GoalTemplate struct {
	IDHint      string `yaml:"id_hint,omitempty" json:"id_hint,omitempty"`
	ID          string `yaml:"id,omitempty" json:"id,omitempty"`
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Status      string `yaml:"status,omitempty" json:"status,omitempty"`
	Authority   string `yaml:"authority,omitempty" json:"authority,omitempty"` // required by spec at creation
	Target      string `yaml:"target,omitempty" json:"target,omitempty"`       // required by spec at creation
}

// RequirementTemplate describes a requirement object to be created from a bundle.
type RequirementTemplate struct {
	IDHint          string   `yaml:"id_hint,omitempty" json:"id_hint,omitempty"`
	ID              string   `yaml:"id,omitempty" json:"id,omitempty"`
	Title           string   `yaml:"title" json:"title"`
	Status          string   `yaml:"status,omitempty" json:"status,omitempty"`
	GoalRefs        []string `yaml:"goal_refs,omitempty" json:"goal_refs,omitempty"` // required by spec (minCount: 1); use id or id_hint of a goal
	Criteria        []string `yaml:"criteria_refs,omitempty" json:"criteria_refs,omitempty"`
	Body            string   `yaml:"body,omitempty" json:"body,omitempty"`
	Priority        string   `yaml:"priority,omitempty" json:"priority,omitempty"` // p0..p3
	PriorityPlanRef string   `yaml:"priority_plan_ref,omitempty" json:"priority_plan_ref,omitempty"`
	DocEntryRefs    []string `yaml:"doc_entry_refs,omitempty" json:"doc_entry_refs,omitempty"`
}

// CriteriaTemplate describes a criteria object. Parent linkage is a bundle hint only
// (requirement_ref / requirement_refs) — persisted on requirement.criteria_refs, never on criteria.
type CriteriaTemplate struct {
	IDHint           string   `yaml:"id_hint,omitempty" json:"id_hint,omitempty"`
	ID               string   `yaml:"id,omitempty" json:"id,omitempty"`
	Title            string   `yaml:"title" json:"title"`
	Status           string   `yaml:"status,omitempty" json:"status,omitempty"`
	RequirementRef   string   `yaml:"requirement_ref,omitempty" json:"requirement_ref,omitempty"`
	RequirementRefs  []string `yaml:"requirement_refs,omitempty" json:"requirement_refs,omitempty"` // hint; not stored on criteria
	Description      string   `yaml:"description,omitempty" json:"description,omitempty"`
	Category         string   `yaml:"category,omitempty" json:"category,omitempty"`                   // default: acceptance
	ValidationMethod string   `yaml:"validation_method,omitempty" json:"validation_method,omitempty"` // e.g. code_review, manual_check
}

// DocEntryTemplate describes a doc_entry object (e.g. linking a design file to requirements).
type DocEntryTemplate struct {
	IDHint            string   `yaml:"id_hint,omitempty" json:"id_hint,omitempty"`
	ID                string   `yaml:"id,omitempty" json:"id,omitempty"`
	Title             string   `yaml:"title" json:"title"`
	Summary           string   `yaml:"summary" json:"summary"`
	Path              string   `yaml:"path" json:"path"`
	Group             string   `yaml:"group,omitempty" json:"group,omitempty"`       // e.g. architecture, design
	Category          string   `yaml:"category,omitempty" json:"category,omitempty"` // lowercase slug per spec
	Status            string   `yaml:"status,omitempty" json:"status,omitempty"`     // draft, published, ...
	ContentSearchable *bool    `yaml:"content_searchable,omitempty" json:"content_searchable,omitempty"`
	RequirementRefs   []string `yaml:"requirement_refs,omitempty" json:"requirement_refs,omitempty"`
}

// ConvergenceSessionTemplate describes a convergence_session scaffold (CVS-*).
type ConvergenceSessionTemplate struct {
	IDHint           string   `yaml:"id_hint,omitempty" json:"id_hint,omitempty"`
	ID               string   `yaml:"id,omitempty" json:"id,omitempty"`
	Title            string   `yaml:"title" json:"title"`
	Status           string   `yaml:"status,omitempty" json:"status,omitempty"`                       // draft default
	CurrentPhase     string   `yaml:"current_phase,omitempty" json:"current_phase,omitempty"`         // c1_scope default
	OutcomeCharacter string   `yaml:"outcome_character,omitempty" json:"outcome_character,omitempty"` // pending default
	DeltaAssessment  string   `yaml:"delta_assessment,omitempty" json:"delta_assessment,omitempty"`
	DesiredEndState  string   `yaml:"desired_end_state,omitempty" json:"desired_end_state,omitempty"`
	Hypothesis       string   `yaml:"hypothesis,omitempty" json:"hypothesis,omitempty"`
	IterationProcess string   `yaml:"iteration_process,omitempty" json:"iteration_process,omitempty"`
	NextAction       string   `yaml:"next_action,omitempty" json:"next_action,omitempty"`
	FlowVariant      string   `yaml:"flow_variant,omitempty" json:"flow_variant,omitempty"`
	GlossaryTermRef  string   `yaml:"glossary_term_ref,omitempty" json:"glossary_term_ref,omitempty"`
	AutomationHooks  string   `yaml:"automation_hooks,omitempty" json:"automation_hooks,omitempty"`
	RequirementRefs  []string `yaml:"requirement_refs,omitempty" json:"requirement_refs,omitempty"`
}

// BacklogTemplate describes a backlog_item that implements one or more criteria.
type BacklogTemplate struct {
	IDHint          string   `yaml:"id_hint,omitempty" json:"id_hint,omitempty"`
	ID              string   `yaml:"id,omitempty" json:"id,omitempty"`
	Title           string   `yaml:"title" json:"title"`
	Description     string   `yaml:"description,omitempty" json:"description,omitempty"`
	Status          string   `yaml:"status,omitempty" json:"status,omitempty"`
	RequirementRefs []string `yaml:"requirement_refs,omitempty" json:"requirement_refs,omitempty"`
	CriteriaRefs    []string `yaml:"criteria_refs,omitempty" json:"criteria_refs,omitempty"`
	TestCaseRefs    []string `yaml:"test_case_refs,omitempty" json:"test_case_refs,omitempty"`
	MilestoneRefs   []string `yaml:"milestone_refs,omitempty" json:"milestone_refs,omitempty"`
	GoalRefs        []string `yaml:"goal_refs,omitempty" json:"goal_refs,omitempty"`
	DocEntryRefs    []string `yaml:"doc_entry_refs,omitempty" json:"doc_entry_refs,omitempty"`
	KindUnderTest   string   `yaml:"kind_under_test,omitempty" json:"kind_under_test,omitempty"`
	Priority        string   `yaml:"priority,omitempty" json:"priority,omitempty"`
	PriorityTier    string   `yaml:"priority_tier,omitempty" json:"priority_tier,omitempty"`
}

// TestCaseTemplate describes a test_case object that links criteria to concrete tests.
type TestCaseTemplate struct {
	IDHint          string   `yaml:"id_hint,omitempty" json:"id_hint,omitempty"`
	ID              string   `yaml:"id,omitempty" json:"id,omitempty"`
	Title           string   `yaml:"title" json:"title"`
	Status          string   `yaml:"status,omitempty" json:"status,omitempty"`
	KindUnderTest   string   `yaml:"kind_under_test,omitempty" json:"kind_under_test,omitempty"`
	RequirementRefs []string `yaml:"requirement_refs,omitempty" json:"requirement_refs,omitempty"`
	CriteriaRefs    []string `yaml:"criteria_refs,omitempty" json:"criteria_refs,omitempty"`
	TestFunctions   []string `yaml:"test_functions,omitempty" json:"test_functions,omitempty"`
	PathOrID        string   `yaml:"path_or_id,omitempty" json:"path_or_id,omitempty"`
}

// FixtureGroup groups fixture templates by kind. This is intentionally narrow
// to start; additional fixture types can be added as needed.
type FixtureGroup struct {
	SchedulerJobs []SchedulerJobFixture `yaml:"scheduler_jobs,omitempty" json:"scheduler_jobs,omitempty"`
	// Additional fixture groups (backlog_items, goals, etc.) can be added here.
}

// SchedulerJobFixture represents a scheduler_job template included in a bundle.
type SchedulerJobFixture struct {
	IDHint   string         `yaml:"id_hint,omitempty" json:"id_hint,omitempty"`
	ID       string         `yaml:"id,omitempty" json:"id,omitempty"`
	Template map[string]any `yaml:"template" json:"template"`
}

// BundleStep represents an executable step in a scenario (commands, triggers, etc.).
// Pure traceability bundles do not need steps; test-scenarios will use them heavily.
type BundleStep struct {
	ID          string   `yaml:"id,omitempty" json:"id,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Commands    []string `yaml:"commands,omitempty" json:"commands,omitempty"`
}

// LoadBundle decodes a scenario bundle from an io.Reader (YAML).
// It performs basic validation of api_version and kind but does not yet
// apply any objects or steps. Callers are expected to pass the resulting
// Bundle to ApplyScenarioBundle.
func LoadBundle(r io.Reader) (*Bundle, error) {
	var b Bundle
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&b); err != nil {
		return nil, errfmt.Errorf(errDecodeBundleFmt, err)
	}
	if b.APIVersion == emptyBundleValue {
		return nil, errors.New(errMissingAPIVersion)
	}
	if b.Kind == emptyBundleValue {
		return nil, errors.New(errMissingKind)
	}
	return &b, nil
}
