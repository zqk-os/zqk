// Package reqharness is the test-requirements verification harness.
//
// It closes the loop between a requirement (kernel object shape: id, claim,
// test_criteria) and re-runnable evidence: every criterion names a *predicate*
// (scope:Name) that the harness registers and executes. A requirement is only
// PASS when its structure is sound AND every criterion predicate succeeds.
// Narrative text on the requirement never counts as evidence — predicates are
// the evidence, mirroring the VDS independent-verify posture (POL-WORKFLOW-VDS).
package reqharness

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Schema and verdict vocabulary.
const (
	SchemaRequirementsV1 = "zqk_reqharness_report_v1"

	VerdictPass = "PASS"
	VerdictFail = "FAIL"
)

// Message fragments (named to keep the audit quiet and copyable).
const (
	msgNameRequired  = "reqharness: predicate name required"
	msgNameGrammar   = "reqharness: predicate %q must be scope:Name where each segment is [A-Za-z0-9_]+"
	msgNilFunc       = "reqharness: predicate %s: nil function"
	msgDuplicate     = "reqharness: predicate %s already registered"
	nextAssignID     = "assign a stable requirement id"
	nextWriteClaim   = "write one independently verifiable claim sentence"
	nextAttachCrit   = "attach at least one test_criterion predicate"
	detailBadRequest = "invalid predicate shape: "
	nextFixGrammar   = "fix predicate grammar on "
	detailNotReg     = "predicate not registered (registered: "
	detailRegEmpty   = "predicate not registered (registry is empty)"
	nextRegister     = "register predicate "
	nextRegSuffix    = " before verify"
	nextPassPred     = "make predicate " // #nosec G101
	briefEvidence    = "Done means re-runnable predicate evidence; narrative is never a gate."
	msgRead          = "reqharness: read %s: %w"
	msgParse         = "reqharness: parse %s: %w"
	msgNoReq         = "reqharness: no requirements in %s"
	msgRefuseSave    = "reqharness: refuse to save empty requirements to %s"
	msgMkdir         = "reqharness: mkdir for %s: %w"
	msgMarshal       = "reqharness: marshal requirements: %w"
	msgWrite         = "reqharness: write %s: %w"
)

// errPredFail is the canonical predicate failure error.
var errPredFail = fmt.Errorf("predicate returned failure")

// PredicateFunc is one registered, re-runnable check.
type PredicateFunc func(ctx context.Context) error

// Criterion is one machine-verifiable check attached to a requirement.
type Criterion struct {
	ID        string `json:"id"`
	Predicate string `json:"predicate"`
}

// Requirement is the harness input: a claim plus its test criteria.
type Requirement struct {
	RequirementID string      `json:"requirement_id"`
	Claim         string      `json:"claim"`
	TestCriteria  []Criterion `json:"test_criteria,omitempty"`
}

// CriterionResult is the judged outcome of one criterion.
type CriterionResult struct {
	CriterionID string `json:"criterion_id"`
	Predicate   string `json:"predicate"`
	OK          bool   `json:"ok"`
	Skipped     bool   `json:"skipped"`
	Detail      string `json:"detail,omitempty"`
}

// RequirementResult is the judged outcome of one requirement.
type RequirementResult struct {
	RequirementID string            `json:"requirement_id"`
	Claim         string            `json:"claim"`
	Verdict       string            `json:"verdict"`
	Criteria      []CriterionResult `json:"criteria"`
	NextActions   []string          `json:"next_actions,omitempty"`
}

// Report is the stable, JSON-encodable harness output.
type Report struct {
	Schema               string              `json:"schema"`
	Verdict              string              `json:"verdict"`
	Results              []RequirementResult `json:"results,omitempty"`
	FailedRequirementIDs []string            `json:"failed_requirement_ids,omitempty"`
	NextActions          []string            `json:"next_actions,omitempty"`
	AgentBrief           string              `json:"agent_brief"`
}

// --- Predicate registry (process-lifetime, concurrency-safe) -----------------

var (
	regMu  sync.RWMutex
	regMap = make(map[string]PredicateFunc)
)

var predicateNameRe = regexp.MustCompile(`^[A-Za-z0-9_]+(:[A-Za-z0-9_]+)+$`)

// PredicateName validates the scope:Name predicate grammar.
func PredicateName(name string) error {
	if name == "" {
		return fmt.Errorf("%s", msgNameRequired)
	}
	if !predicateNameRe.MatchString(name) {
		return fmt.Errorf(msgNameGrammar, name)
	}
	return nil
}

// RegisterPredicate binds a name to a runnable check. Duplicates are rejected
// so two consumers can never silently disagree about what a name means.
func RegisterPredicate(name string, fn PredicateFunc) error {
	if err := PredicateName(name); err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf(msgNilFunc, name)
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, exists := regMap[name]; exists {
		return fmt.Errorf(msgDuplicate, name)
	}
	regMap[name] = fn
	return nil
}

// lookupPredicate resolves a name; ok=false when never registered.
func lookupPredicate(name string) (PredicateFunc, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	fn, ok := regMap[name]
	return fn, ok
}

// registeredNames returns a sorted list of predicate names (diagnostics).
func registeredNames() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(regMap))
	for n := range regMap {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// --- Verify ------------------------------------------------------------------

// critOutcome is one criterion's judged state, computed declaratively.
type critOutcome struct {
	result       CriterionResult
	structBroken bool
	next         []string
}

// judgeCriterion decides a single criterion: malformed shape, unregistered
// predicate, failing predicate, or pass.
func judgeCriterion(ctx context.Context, c Criterion) critOutcome {
	var o critOutcome
	o.result = CriterionResult{CriterionID: c.ID, Predicate: c.Predicate}

	if err := PredicateName(c.Predicate); err != nil {
		o.result.Skipped = true
		o.result.Detail = detailBadRequest + err.Error()
		o.structBroken = true
		if c.Predicate != "" {
			o.next = append(o.next, nextFixGrammar+nameOrID(c))
		}
		return o
	}
	fn, ok := lookupPredicate(c.Predicate)
	if !ok {
		o.result.Skipped = true
		if hint := registeredNames(); len(hint) > 0 {
			o.result.Detail = detailNotReg + strings.Join(hint, ", ") + ")"
		} else {
			o.result.Detail = detailRegEmpty
		}
		o.next = append(o.next, nextRegister+c.Predicate+nextRegSuffix)
		return o
	}
	if err := fn(ctx); err != nil {
		o.result.Detail = err.Error()
		o.next = append(o.next, nextPassPred+c.Predicate)
		return o
	}
	o.result.OK = true
	return o
}

// Verify runs every requirement's structure checks and criterion predicates.
// A requirement passes iff its structure is sound and all predicates succeed.
// Unknown predicates are reported (skipped + detail) and fail the requirement:
// the harness never credits evidence it cannot run.
func Verify(ctx context.Context, reqs []Requirement) *Report {
	rep := &Report{Schema: SchemaRequirementsV1, NextActions: []string{}}
	allPass := true

	for _, req := range reqs {
		rr := RequirementResult{
			RequirementID: req.RequirementID,
			Claim:         req.Claim,
			Criteria:      []CriterionResult{},
			NextActions:   []string{},
		}

		structOK := true
		if req.RequirementID == "" {
			structOK = false
			rr.NextActions = append(rr.NextActions, nextAssignID)
		}
		if req.Claim == "" {
			structOK = false
			rr.NextActions = append(rr.NextActions, nextWriteClaim)
		}
		if len(req.TestCriteria) == 0 {
			structOK = false
			rr.NextActions = append(rr.NextActions, nextAttachCrit)
		}

		allCriteriaOK := true
		for _, c := range req.TestCriteria {
			o := judgeCriterion(ctx, c)
			if o.structBroken {
				structOK = false
			}
			if !o.result.OK {
				allCriteriaOK = false
			}
			rr.Criteria = append(rr.Criteria, o.result)
			rr.NextActions = append(rr.NextActions, o.next...)
		}

		if structOK && allCriteriaOK {
			rr.Verdict = VerdictPass
		} else {
			rr.Verdict = VerdictFail
		}

		if rr.Verdict != VerdictPass {
			allPass = false
			rep.FailedRequirementIDs = append(rep.FailedRequirementIDs, req.RequirementID)
		}
		rep.Results = append(rep.Results, rr)
	}

	if allPass {
		rep.Verdict = VerdictPass
	} else {
		rep.Verdict = VerdictFail
		rep.NextActions = uniqStrings(collectNextActions(rep.Results))
	}
	rep.AgentBrief = briefEvidence
	return rep
}

func collectNextActions(results []RequirementResult) []string {
	var out []string
	for _, r := range results {
		out = append(out, r.NextActions...)
	}
	return out
}

func uniqStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func nameOrID(c Criterion) string {
	if c.ID != "" {
		return c.ID
	}
	return c.Predicate
}

// --- Requirement file IO ------------------------------------------------------

// LoadRequirements reads a JSON requirement list; empty lists are rejected so
// a harness run can never vacuously pass.
func LoadRequirements(path string) ([]Requirement, error) {
	b, err := fileutil.ReadFile(filepath.Clean(path)) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf(msgRead, path, err)
	}
	var reqs []Requirement
	if err := json.Unmarshal(b, &reqs); err != nil {
		return nil, fmt.Errorf(msgParse, path, err)
	}
	if len(reqs) == 0 {
		return nil, fmt.Errorf(msgNoReq, path)
	}
	return reqs, nil
}

// SaveRequirements writes the requirement list as stable JSON.
func SaveRequirements(path string, reqs []Requirement) error {
	if len(reqs) == 0 {
		return fmt.Errorf(msgRefuseSave, path)
	}
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm750); err != nil {
		return fmt.Errorf(msgMkdir, path, err)
	}
	b, err := json.MarshalIndent(reqs, "", "  ")
	if err != nil {
		return fmt.Errorf(msgMarshal, err)
	}
	b = append(b, '\n')
	if err := fileutil.WriteSecureFile(path, b); err != nil {
		return fmt.Errorf(msgWrite, path, err)
	}
	return nil
}
