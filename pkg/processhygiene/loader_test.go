package processhygiene

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Static Floor Invariants: LoadEmbeddedDefaultRules and CompileRuleConfig
// return well-formed rule contracts with non-empty metadata.
func TestLoader_StaticFloorInvariants(t *testing.T) {
	rules, err := LoadEmbeddedDefaultRules()
	if err != nil {
		t.Fatalf("LoadEmbeddedDefaultRules failed: %v", err)
	}
	if len(rules) == 0 {
		t.Fatalf("expected embedded default rules to have at least 1 rule")
	}

	for i, r := range rules {
		if r.ID() == "" {
			t.Fatalf("rule %d has empty ID", i)
		}
		if r.Description() == "" {
			t.Fatalf("rule %s has empty Description", r.ID())
		}
		if r.MatchedField() == "" {
			t.Fatalf("rule %s has empty MatchedField", r.ID())
		}
	}
}

// Operational Dynamic Proofs: Verify CompileRuleConfig for all match modes
// (prefix, suffix, equals, regex) and ResolveRules waterfall resolution order.
func TestLoader_OperationalDynamicProofs(t *testing.T) {
	t.Run("CompileRuleConfig matches prefix, suffix, equals, and regex", func(t *testing.T) {
		// 1. Prefix rule
		rPrefix, err := CompileRuleConfig(RuleConfig{
			ID:          "pfx_rule",
			Description: "prefix test",
			Match: Match{
				Field:  objects.FieldKeyTitle,
				Prefix: "WIP:",
			},
		})
		if err != nil {
			t.Fatalf("CompileRuleConfig prefix: %v", err)
		}
		if detail, ok := rPrefix.Evaluate(map[string]any{objects.FieldKeyTitle: "WIP: active task"}); !ok || !strings.Contains(detail, "prefix") {
			t.Fatalf("expected prefix match, got ok=%v detail=%q", ok, detail)
		}
		if _, ok := rPrefix.Evaluate(map[string]any{objects.FieldKeyTitle: "Done: finished task"}); ok {
			t.Fatalf("expected prefix mismatch to return false")
		}

		// 2. Suffix rule
		rSuffix, err := CompileRuleConfig(RuleConfig{
			ID:          "sfx_rule",
			Description: "suffix test",
			Match: Match{
				Field:  objects.FieldKeyTitle,
				Suffix: "[DRAFT]",
			},
		})
		if err != nil {
			t.Fatalf("CompileRuleConfig suffix: %v", err)
		}
		if detail, ok := rSuffix.Evaluate(map[string]any{objects.FieldKeyTitle: "Feature plan [DRAFT]"}); !ok || !strings.Contains(detail, "suffix") {
			t.Fatalf("expected suffix match, got ok=%v detail=%q", ok, detail)
		}
		if _, ok := rSuffix.Evaluate(map[string]any{objects.FieldKeyTitle: "Feature plan [PROMOTED]"}); ok {
			t.Fatalf("expected suffix mismatch to return false")
		}

		// 3. Equals rule
		rEquals, err := CompileRuleConfig(RuleConfig{
			ID:          "eq_rule",
			Description: "equals test",
			Match: Match{
				Field:  objects.FieldKeyKind,
				Equals: "deprecated_kind",
			},
		})
		if err != nil {
			t.Fatalf("CompileRuleConfig equals: %v", err)
		}
		if detail, ok := rEquals.Evaluate(map[string]any{objects.FieldKeyKind: "deprecated_kind"}); !ok || !strings.Contains(detail, "equals") {
			t.Fatalf("expected equals match, got ok=%v detail=%q", ok, detail)
		}
		if _, ok := rEquals.Evaluate(map[string]any{objects.FieldKeyKind: "active_kind"}); ok {
			t.Fatalf("expected equals mismatch to return false")
		}

		// 4. Regex rule
		rRegex, err := CompileRuleConfig(RuleConfig{
			ID:          "re_rule",
			Description: "regex test",
			Match: Match{
				Field: objects.FieldKeyID,
				Regex: `^BLI-\d{3,4}$`,
			},
		})
		if err != nil {
			t.Fatalf("CompileRuleConfig regex: %v", err)
		}
		if detail, ok := rRegex.Evaluate(map[string]any{objects.FieldKeyID: "BLI-123"}); !ok || !strings.Contains(detail, "matches pattern") {
			t.Fatalf("expected regex match, got ok=%v detail=%q", ok, detail)
		}
		if _, ok := rRegex.Evaluate(map[string]any{objects.FieldKeyID: "BLI-INVALID-SUFFIX"}); ok {
			t.Fatalf("expected regex mismatch to return false")
		}
	})

	t.Run("ResolveRules priority 1: explicit rulesFile overrides everything", func(t *testing.T) {
		tmpDir := t.TempDir()
		rulesPath := filepath.Join(tmpDir, "custom_rules.yaml")
		content := `rules:
  - id: custom_explicit
    description: custom explicit rule
    match:
      field: title
      equals: forbidden_title
`
		if err := fileutil.WriteFile(rulesPath, []byte(content), 0o600); err != nil {
			t.Fatalf("write custom rules: %v", err)
		}

		rules, source, err := ResolveRules(tmpDir, rulesPath)
		if err != nil {
			t.Fatalf("ResolveRules failed: %v", err)
		}
		if source != rulesPath {
			t.Fatalf("expected source %q, got %q", rulesPath, source)
		}
		if len(rules) != 1 || rules[0].ID() != "custom_explicit" {
			t.Fatalf("unexpected rules loaded: %+v", rules)
		}
	})

	t.Run("ResolveRules priority 2: candidate file in projectRoot/.zqk/specs", func(t *testing.T) {
		tmpDir := t.TempDir()
		specsDir := filepath.Join(tmpDir, paths.ProcessInternalDir)
		if err := fileutil.MkdirAll(specsDir, 0o755); err != nil {
			t.Fatalf("mkdir specs: %v", err)
		}
		candidatePath := filepath.Join(specsDir, "process_hygiene_rules.yaml")
		content := `rules:
  - id: project_candidate_rule
    description: candidate rule in project root specs
    match:
      field: id
      prefix: PROJ-
`
		if err := fileutil.WriteFile(candidatePath, []byte(content), 0o600); err != nil {
			t.Fatalf("write candidate file: %v", err)
		}

		rules, source, err := ResolveRules(tmpDir, "")
		if err != nil {
			t.Fatalf("ResolveRules failed: %v", err)
		}
		if source != candidatePath {
			t.Fatalf("expected source %q, got %q", candidatePath, source)
		}
		if len(rules) != 1 || rules[0].ID() != "project_candidate_rule" {
			t.Fatalf("unexpected rules loaded: %+v", rules)
		}
	})

	t.Run("ResolveRules priority 3: fallback to embedded defaults when no files exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		rules, source, err := ResolveRules(tmpDir, "")
		if err != nil {
			t.Fatalf("ResolveRules fallback failed: %v", err)
		}
		if source != "embedded:default_rules.yaml" {
			t.Fatalf("expected embedded default source, got %q", source)
		}
		if len(rules) == 0 {
			t.Fatalf("expected non-empty embedded default rules")
		}
	})

	t.Run("ResolveRulesWithOptions storage provider path", func(t *testing.T) {
		ctx := context.Background()
		sec := pkgctx.NewSystemSecurityContext()
		st := &stubObjectStorage{
			listObjs: []map[string]any{
				{
					objects.FieldKeyRuleID:      "rule_from_storage",
					objects.FieldKeyMatchField:  objects.FieldKeyTitle,
					objects.FieldKeyMatchPrefix: "STORAGE:",
					objects.FieldKeyEnabled:     true,
				},
			},
		}

		opts := ResolveRulesOptions{
			StorageProvider: st,
			SecCtx:          sec,
			ListCtx:         ctx,
		}

		rules, source, err := ResolveRulesWithOptions(opts)
		if err != nil {
			t.Fatalf("ResolveRulesWithOptions failed: %v", err)
		}
		if source != "storage:process_hygiene_rule" {
			t.Fatalf("expected storage source, got %q", source)
		}
		if len(rules) != 1 || rules[0].ID() != "rule_from_storage" {
			t.Fatalf("unexpected rule loaded from storage: %+v", rules)
		}
	})
}

// Negative Adversarial Boundaries: Verify error handling on invalid configurations,
// YAML parse failures, missing files, and hostile rule inputs.
func TestLoader_NegativeAdversarialBoundaries(t *testing.T) {
	t.Run("CompileRuleConfig input validation errors", func(t *testing.T) {
		// Missing ID
		_, err := CompileRuleConfig(RuleConfig{
			Match: Match{Field: "title", Prefix: "P"},
		})
		if err == nil || !strings.Contains(err.Error(), "missing id") {
			t.Fatalf("expected 'missing id' error, got %v", err)
		}

		// Missing Field
		_, err = CompileRuleConfig(RuleConfig{
			ID:    "r1",
			Match: Match{Prefix: "P"},
		})
		if err == nil || !strings.Contains(err.Error(), "match.field is required") {
			t.Fatalf("expected 'match.field is required' error, got %v", err)
		}

		// Zero matchers
		_, err = CompileRuleConfig(RuleConfig{
			ID:    "r2",
			Match: Match{Field: "title"},
		})
		if err == nil || !strings.Contains(err.Error(), "exactly one of prefix, suffix, equals, regex must be set") {
			t.Fatalf("expected 'exactly one of...' error, got %v", err)
		}

		// Multiple matchers (prefix and suffix)
		_, err = CompileRuleConfig(RuleConfig{
			ID:    "r3",
			Match: Match{Field: "title", Prefix: "P", Suffix: "S"},
		})
		if err == nil || !strings.Contains(err.Error(), "exactly one of prefix, suffix, equals, regex must be set") {
			t.Fatalf("expected 'exactly one of...' error on multi-matcher, got %v", err)
		}

		// Multiple matchers (equals and regex)
		_, err = CompileRuleConfig(RuleConfig{
			ID:    "r4",
			Match: Match{Field: "title", Equals: "E", Regex: ".*"},
		})
		if err == nil || !strings.Contains(err.Error(), "exactly one of prefix, suffix, equals, regex must be set") {
			t.Fatalf("expected 'exactly one of...' error on multi-matcher, got %v", err)
		}

		// Invalid regex pattern
		_, err = CompileRuleConfig(RuleConfig{
			ID:    "r5",
			Match: Match{Field: "title", Regex: `[unclosed-regex`},
		})
		if err == nil || !strings.Contains(err.Error(), "compile regex") {
			t.Fatalf("expected regex compilation error, got %v", err)
		}
	})

	t.Run("LoadRulesYAML parse errors", func(t *testing.T) {
		// Malformed YAML
		_, err := LoadRulesYAML([]byte("not: valid: yaml: [}{] malformed"))
		if err == nil || !strings.Contains(err.Error(), "parse rules yaml") {
			t.Fatalf("expected 'parse rules yaml' error, got %v", err)
		}

		// Empty rules list
		_, err = LoadRulesYAML([]byte(`rules: []`))
		if err == nil || !strings.Contains(err.Error(), "no rules defined") {
			t.Fatalf("expected 'no rules defined' error, got %v", err)
		}

		// Rules containing an invalid rule
		_, err = LoadRulesYAML([]byte(`rules:
  - id: bad_rule
    match:
      field: ""
`))
		if err == nil || !strings.Contains(err.Error(), "match.field is required") {
			t.Fatalf("expected wrapped rule compilation error, got %v", err)
		}
	})

	t.Run("LoadRulesFile errors", func(t *testing.T) {
		_, err := LoadRulesFile("/non/existent/path/rules.yaml")
		if err == nil {
			t.Fatal("expected error loading non-existent file")
		}
	})

	t.Run("ResolveRulesWithOptions error propagation", func(t *testing.T) {
		// Non-existent rulesFile
		_, _, err := ResolveRulesWithOptions(ResolveRulesOptions{RulesFile: "/non/existent/file.yaml"})
		if err == nil {
			t.Fatal("expected error with invalid RulesFile path")
		}

		// Storage provider error
		ctx := context.Background()
		sec := pkgctx.NewSystemSecurityContext()
		errStorage := &failingStorageProvider{}
		_, _, err = ResolveRulesWithOptions(ResolveRulesOptions{
			StorageProvider: errStorage,
			SecCtx:          sec,
			ListCtx:         ctx,
		})
		if err == nil {
			t.Fatal("expected error propagating from StorageProvider")
		}
	})

	t.Run("Rule Evaluate boundary edge cases and ScanObjects preview trimming", func(t *testing.T) {
		r, err := CompileRuleConfig(RuleConfig{
			ID:          "edge_rule",
			Description: "rule for edge cases",
			Match: Match{
				Field:  objects.FieldKeyTitle,
				Prefix: "LONG:",
			},
		})
		if err != nil {
			t.Fatalf("compile edge rule: %v", err)
		}

		// 1. Evaluate with nil obj
		if _, ok := r.Evaluate(nil); ok {
			t.Fatal("expected Evaluate(nil) to return false")
		}

		// 2. Evaluate with missing field
		if _, ok := r.Evaluate(map[string]any{"other_field": "val"}); ok {
			t.Fatal("expected Evaluate with missing field to return false")
		}

		// 3. Evaluate with nil value in field
		if _, ok := r.Evaluate(map[string]any{objects.FieldKeyTitle: nil}); ok {
			t.Fatal("expected Evaluate with nil field value to return false")
		}

		// 4. Evaluate with whitespace-only field value
		if _, ok := r.Evaluate(map[string]any{objects.FieldKeyTitle: "   \t\n  "}); ok {
			t.Fatal("expected Evaluate with whitespace-only field to return false")
		}

		// 5. ScanObjects with value > defaultMaxValue (500 chars) triggering trimPreview
		longVal := "LONG:" + strings.Repeat("A", 600)
		longObj := map[string]any{
			objects.FieldKeyID:    "BLI-LONG",
			objects.FieldKeyKind:  "backlog_item",
			objects.FieldKeyTitle: longVal,
		}
		findings := ScanObjects([]map[string]any{longObj}, []Rule{r})
		if len(findings) != 1 {
			t.Fatalf("expected 1 finding for long object, got %d", len(findings))
		}
		if !strings.HasSuffix(findings[0].Value, "...") {
			t.Fatalf("expected value to be trimmed with '...', got len=%d", len(findings[0].Value))
		}
		if len(findings[0].Value) != 503 { // 500 + "..."
			t.Fatalf("expected trimmed length 503, got %d", len(findings[0].Value))
		}
	})

	t.Run("Storage field decoding helpers and ruleConfigFromObject boundaries", func(t *testing.T) {
		// asIntField branches
		intObj := map[string]any{
			"i":   10,
			"i64": int64(20),
			"f64": float64(30.0),
			"str": "not-an-int",
		}
		if got := asIntField(intObj, "i", 0); got != 10 {
			t.Fatalf("expected 10, got %d", got)
		}
		if got := asIntField(intObj, "i64", 0); got != 20 {
			t.Fatalf("expected 20, got %d", got)
		}
		if got := asIntField(intObj, "f64", 0); got != 30 {
			t.Fatalf("expected 30, got %d", got)
		}
		if got := asIntField(intObj, "str", 99); got != 99 {
			t.Fatalf("expected default 99 for non-numeric string, got %d", got)
		}
		if got := asIntField(intObj, "missing", 42); got != 42 {
			t.Fatalf("expected default 42 for missing key, got %d", got)
		}

		// asBoolField branches
		boolObj := map[string]any{
			"b_true":  true,
			"b_false": false,
			"str":     "not-a-bool",
		}
		if got := asBoolField(boolObj, "b_true", false); !got {
			t.Fatalf("expected true, got %v", got)
		}
		if got := asBoolField(boolObj, "b_false", true); got {
			t.Fatalf("expected false, got %v", got)
		}
		if got := asBoolField(boolObj, "str", true); !got {
			t.Fatalf("expected default true for non-bool, got %v", got)
		}
		if got := asBoolField(boolObj, "missing", true); !got {
			t.Fatalf("expected default true for missing key, got %v", got)
		}

		// ruleConfigFromObject validation boundaries
		// Missing rule_id
		if _, err := ruleConfigFromObject(map[string]any{}); err == nil {
			t.Fatal("expected error for missing rule_id")
		}
		// Missing match_field
		if _, err := ruleConfigFromObject(map[string]any{objects.FieldKeyRuleID: "r1"}); err == nil {
			t.Fatal("expected error for missing match_field")
		}
		// Invalid matchers count
		if _, err := ruleConfigFromObject(map[string]any{
			objects.FieldKeyRuleID:     "r1",
			objects.FieldKeyMatchField: "title",
		}); err == nil {
			t.Fatal("expected error for 0 matchers")
		}
		// Suffix matcher
		rcSuffix, err := ruleConfigFromObject(map[string]any{
			objects.FieldKeyRuleID:      "r_sfx",
			objects.FieldKeyMatchField:  "title",
			objects.FieldKeyMatchSuffix: "-end",
		})
		if err != nil || rcSuffix.Match.Suffix != "-end" {
			t.Fatalf("failed to build suffix rule config: %v", err)
		}
		// Equals matcher
		rcEquals, err := ruleConfigFromObject(map[string]any{
			objects.FieldKeyRuleID:      "r_eq",
			objects.FieldKeyMatchField:  "title",
			objects.FieldKeyMatchEquals: "exact",
		})
		if err != nil || rcEquals.Match.Equals != "exact" {
			t.Fatalf("failed to build equals rule config: %v", err)
		}
		// Regex matcher
		rcRegex, err := ruleConfigFromObject(map[string]any{
			objects.FieldKeyRuleID:     "r_re",
			objects.FieldKeyMatchField: "title",
			objects.FieldKeyMatchRegex: "^pattern$",
		})
		if err != nil || rcRegex.Match.Regex != "^pattern$" {
			t.Fatalf("failed to build regex rule config: %v", err)
		}
	})
}

// failingStorageProvider simulates a storage failure.
type failingStorageProvider struct {
	stubObjectStorage
}

func (*failingStorageProvider) List(context.Context, *storage.SecurityContext, *pkgctx.StorageContext, storage.ListFilter) (*storage.QueryResult, error) {
	return nil, errors.New("simulated storage list error")
}
