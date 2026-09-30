package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Correspondence test: overlay rule configs name statuses, lifecycles define which statuses
// exist, and nothing previously checked that the two agree. A rule naming a status its target
// kind does not have is dead configuration — it reads like a barrier, it compiles, it is
// registered, and it can never match. That is the same silent fail-open as a precondition the
// validator cannot parse, and it is why the requirement overlay spent an unknown period
// refusing criteria in "not_started", a status the criteria lifecycle has never contained.

// statusOwner says whose lifecycle defines the statuses held under a config key: the kind the
// overlay itself belongs to, or the kind referenced by another config key naming a ref field.
type statusOwner struct {
	subject      bool
	refFieldFrom string
}

var ownerSubject = statusOwner{subject: true}

func ownerRefField(cfgKey string) statusOwner { return statusOwner{refFieldFrom: cfgKey} }

// statusKeyOwners declares, for every config key the evaluator reads as a status list, which
// kind's lifecycle those values must come from.
//
// TestStatusKeyOwners_coversEveryKeyTheEvaluatorReads keeps this honest. Without that guard a
// new status key in the evaluator would simply not be checked here, and this test would report
// success over an unexamined key.
var statusKeyOwners = map[string]statusOwner{
	objects.FieldKeyStatuses: ownerSubject,
	"when_object_status":     ownerSubject,
	"when_parent_status":     ownerSubject,
	"refuse_when":            ownerRefField("plan_field"),
	"require_plan_status":    ownerRefField("plan_field"),
	"refuse_child_status":    ownerRefField("child_field"),
}

// refFieldKind resolves a ref field to the kind its ids are. Declared rather than derived by
// trimming _ref/_refs, because a wrong guess here would validate statuses against the wrong
// lifecycle and report a clean result.
var refFieldKind = map[string]string{
	objects.FieldKeyPriorityPlanRef: objects.KindPriorityPlan,
	objects.FieldKeyCriteriaRefs:    objects.KindCriteria,
}

func repoRootFromCompose() string { return filepath.Join("..", "..", "..") }

// lifecycleKinds returns every kind that declares a lifecycle, read from object_type rather
// than the filename so a renamed file cannot quietly drop a kind from the sweep.
func lifecycleKinds(t *testing.T) []string {
	t.Helper()
	root := repoRootFromCompose()
	dirs := []string{filepath.Join(root, paths.ProcessInternalLifecyclesDir)}
	for _, extra := range objects.ExtraLifecycleRoots() {
		if filepath.IsAbs(extra) {
			dirs = append(dirs, extra)
		} else {
			dirs = append(dirs, filepath.Join(root, extra))
		}
	}
	var kinds []string
	seen := map[string]bool{}
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".yaml") {
				return nil
			}
			raw, err := fileutil.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", info.Name(), err)
			}
			var lc objects.Lifecycle
			if err := yaml.Unmarshal(raw, &lc); err != nil {
				return nil
			}
			if lc.ObjectType != "" && !seen[lc.ObjectType] {
				seen[lc.ObjectType] = true
				kinds = append(kinds, lc.ObjectType)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("read lifecycles dir %s: %v", dir, err)
		}
	}
	sort.Strings(kinds)
	return kinds
}

func TestOverlayRules_statusesExistInTheLifecycleTheyGate(t *testing.T) {
	kinds := lifecycleKinds(t)
	if len(kinds) == 0 {
		t.Fatal("no lifecycles found; this guard would be vacuous")
	}
	// Same loader production validates against, so aliases and status_mapping resolve identically.
	loader := objects.NewLifecycleLoader(filepath.Join(repoRootFromCompose(), paths.ProcessInternalLifecyclesDir))

	var checked, rulesWithStatuses int
	kindsWithRules := map[string]bool{}

	for _, kind := range kinds {
		// All intents that reach kindOverlayRules: backlog_item adds transition-only rules, so
		// sweeping a single intent would miss them.
		for _, intent := range []string{IntentCreate, IntentUpdateFields, IntentTransition} {
			for _, rule := range kindOverlayRules(kind, intent) {
				if rule.Config == nil {
					continue
				}
				kindsWithRules[kind] = true
				sawStatus := false
				t.Logf("Checking rule %q", rule.ID)
				for cfgKey, owner := range statusKeyOwners {
					values, ok := statusValues(rule.Config[cfgKey])
					t.Logf("  key %q: ok=%v, vals=%v", cfgKey, ok, values)
					if !ok {
						continue
					}
					sawStatus = true
					target, err := resolveStatusOwner(kind, owner, rule.Config)
					if err != nil {
						t.Errorf("%s rule %q key %q: %v", kind, rule.ID, cfgKey, err)
						continue
					}
					for _, status := range values {
						checked++
						valid, err := loader.IsValidStatus(target, status)
						if err != nil {
							t.Errorf("%s rule %q: cannot validate %q against %s lifecycle: %v",
								kind, rule.ID, status, target, err)
							continue
						}
						if !valid {
							t.Errorf("%s rule %q (%s) names status %q, which the %s lifecycle does "+
								"not define — this rule can never match, so the barrier it describes "+
								"does not exist",
								kind, rule.ID, cfgKey, status, target)
						}
					}
				}
				if sawStatus {
					rulesWithStatuses++
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no statuses checked; the sweep found no status-bearing overlay configs, so this guard proves nothing")
	}
	t.Logf("checked %d status values across %d status-bearing rules in %d kinds with overlays",
		checked, rulesWithStatuses, len(kindsWithRules))
}

// resolveStatusOwner returns the kind whose lifecycle defines the statuses under a key.
func resolveStatusOwner(subjectKind string, owner statusOwner, cfg map[string]any) (string, error) {
	if owner.subject {
		return subjectKind, nil
	}
	field, _ := cfg[owner.refFieldFrom].(string)
	if field == "" {
		return "", errNoRefField(owner.refFieldFrom)
	}
	kind, ok := refFieldKind[field]
	if !ok {
		return "", errUnmappedRefField(field)
	}
	return kind, nil
}

type errNoRefField string

func (e errNoRefField) Error() string {
	return "config declares statuses for a referenced object but has no " + string(e) + " naming which field"
}

type errUnmappedRefField string

func (e errUnmappedRefField) Error() string {
	return "ref field " + string(e) + " is absent from refFieldKind, so its statuses would be " +
		"validated against the wrong lifecycle; add it rather than letting this key go unchecked"
}

// statusValues normalizes a config value that holds one or more statuses.
func statusValues(v any) ([]string, bool) {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil, false
		}
		return []string{t}, true
	case []string:
		return t, len(t) > 0
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

// TestRequirementUnverifiedCriteria_barrierNowFires is the behavior half. The correspondence
// test proves the rule names real statuses; this proves the rule actually refuses, and — just as
// important for a barrier being switched on after a period of being inert — that it does not
// refuse things it should allow. 938 of the criteria in this tree are archived, so an
// over-broad list here would block completing most requirements that ever linked one.
func TestRequirementUnverifiedCriteria_barrierNowFires(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	const critID = "CRIT-TEST-1"
	lookupWithStatus := func(status string) ObjectLookup {
		return func(id string) (map[string]any, error) {
			if id != critID {
				return nil, nil
			}
			return map[string]any{
				objects.FieldKeyKind:   objects.KindCriteria,
				objects.FieldKeyStatus: status,
			}, nil
		}
	}

	for _, tc := range []struct {
		name           string
		reqStatus      string
		criteriaStatus string
		wantRefused    bool
	}{
		{"awaiting_verification refuses", objects.ObjectStatusComplete, objects.ObjectStatusAwaitingVerification, true},
		{"in_progress refuses", objects.ObjectStatusComplete, objects.ObjectStatusInProgress, true},
		{"blocked refuses", objects.ObjectStatusComplete, objects.ObjectStatusBlocked, true},
		{"validated is evidence", objects.ObjectStatusComplete, objects.ObjectStatusValidated, false},
		{"complete is evidence", objects.ObjectStatusComplete, objects.ObjectStatusComplete, false},
		{"archived is history not an obstacle", objects.ObjectStatusComplete, objects.ObjectStatusArchived, false},
		{"rejected is not pending evidence", objects.ObjectStatusComplete, objects.ObjectStatusRejected, false},
		{"only fires at complete", objects.ObjectStatusActive, objects.ObjectStatusAwaitingVerification, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := map[string]any{
				objects.FieldKeyKind:         objects.KindRequirement,
				objects.FieldKeyID:           "REQ-TEST-1",
				objects.FieldKeyTitle:        "Test Requirement",
				objects.FieldKeyStatus:       tc.reqStatus,
				objects.FieldKeyCriteriaRefs: []any{critID},
			}
			errs := ValidateObject(t.Context(), Default(), objects.KindRequirement, obj, lookupWithStatus(tc.criteriaStatus))
			refused := false
			for _, e := range errs {
				if e.Field == objects.FieldKeyCriteriaRefs && strings.Contains(e.Message, "cannot complete requirement") {
					refused = true
				}
			}
			if refused != tc.wantRefused {
				t.Errorf("requirement=%s criteria=%s: refused=%v want %v (errors: %#v)",
					tc.reqStatus, tc.criteriaStatus, refused, tc.wantRefused, errs)
			}
		})
	}
}

// TestStatusKeyOwners_coversEveryKeyTheEvaluatorReads is what stops the sweep above from
// quietly narrowing. It reads the evaluator's own source for cfg lookups that carry statuses,
// so adding one without declaring its owner fails here instead of going unchecked.
func TestStatusKeyOwners_coversEveryKeyTheEvaluatorReads(t *testing.T) {
	t.Parallel()
	src, err := fileutil.ReadFile("evaluate.go")
	if err != nil {
		t.Fatalf("read evaluate.go: %v", err)
	}
	// cfg["…status…"] / koi.GetString(Slice)(cfg, "…") plus the one status key that is not spelled with "status".
	re := regexp.MustCompile(`(?:cfg\["|koi\.GetString(?:Slice)?\(cfg,\s*")([a-z_]*status[a-z_]*|refuse_when)"`)
	found := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		found[m[1]] = true
	}
	if len(found) == 0 {
		t.Fatal("found no status config lookups in evaluate.go; the pattern no longer matches, so this guard is vacuous")
	}
	for key := range found {
		if _, ok := statusKeyOwners[key]; !ok {
			t.Errorf("evaluate.go reads cfg[%q] as a status but statusKeyOwners does not declare "+
				"whose lifecycle defines it, so overlay values under that key are never checked", key)
		}
	}
	// FieldKeyStatuses is read via the constant, not a literal, so assert it separately.
	if _, ok := statusKeyOwners[objects.FieldKeyStatuses]; !ok {
		t.Errorf("statusKeyOwners is missing %q", objects.FieldKeyStatuses)
	}
	t.Logf("evaluator status keys covered: %d literal + FieldKeyStatuses", len(found))
}
