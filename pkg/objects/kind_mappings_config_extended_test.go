package objects

import (
	"testing"
)

func TestKindMappingsConfig_BackendAndInference(t *testing.T) {
	cfg := defaultKindMappingsConfig()
	if cfg == nil {
		t.Fatal("expected default config")
	}

	// Backend Type getter/setter
	if bt := cfg.GetBackendType(); bt != defaultBackendType {
		t.Errorf("expected %s, got %s", defaultBackendType, bt)
	}
	cfg.SetBackendType("graph")
	if bt := cfg.GetBackendType(); bt != "graph" {
		t.Errorf("expected graph, got %s", bt)
	}

	// ShouldSkipDirectory and ShouldSkipSpec
	if !cfg.ShouldSkipDirectory("_internal") {
		t.Error("expected _internal to be skipped")
	}
	if !cfg.ShouldSkipDirectory("_internal/sub") {
		t.Error("expected _internal/sub to be skipped")
	}
	if cfg.ShouldSkipDirectory("backlog") {
		t.Error("expected backlog not to be skipped")
	}

	if !cfg.ShouldSkipSpec("base_object.yaml") {
		t.Error("expected base_object.yaml to be skipped")
	}
	if !cfg.ShouldSkipSpec("auditable") {
		t.Error("expected auditable to be skipped")
	}
	if cfg.ShouldSkipSpec("backlog_item.yaml") {
		t.Error("expected backlog_item.yaml not to be skipped")
	}

	// IsOnDemandKind
	cfg.OnDemandKinds = []any{
		"my_ondemand_kind",
		map[string]any{
			"pattern": `^.+_metric$`,
		},
		map[string]any{
			"pattern": `[invalid regex`,
		},
	}
	if !cfg.IsOnDemandKind("my_ondemand_kind") {
		t.Error("expected exact string on demand kind to match")
	}
	if !cfg.IsOnDemandKind("custom_metric") {
		t.Error("expected pattern on demand kind to match")
	}
	if cfg.IsOnDemandKind("other_kind") {
		t.Error("expected other_kind not to match on demand")
	}

	// Merge backend config
	defaultB := BackendConfig{
		KindToDirectory: map[string]string{"foo": "foos", "bar": "bars"},
		DirectoryToKind: map[string]string{"foos": "foo", "bars": "bar"},
		MultiKindDirs: map[string]MultiKindConfig{
			"multi": {DefaultKind: "foo", Kinds: []string{"foo", "bar"}},
		},
	}
	backendB := BackendConfig{
		KindToDirectory: map[string]string{"foo": "foo_custom"},
		DirectoryToKind: map[string]string{"foo_custom": "foo"},
	}
	merged := cfg.mergeBackendConfig(backendB, defaultB)
	if merged.KindToDirectory["foo"] != "foo_custom" {
		t.Errorf("merged foo: got %s, want foo_custom", merged.KindToDirectory["foo"])
	}
	if merged.KindToDirectory["bar"] != "bars" {
		t.Errorf("merged bar: got %s, want bars", merged.KindToDirectory["bar"])
	}
	if _, ok := merged.MultiKindDirs["multi"]; !ok {
		t.Errorf("expected multi in merged MultiKindDirs")
	}

	// Inference rules with priorities and fallback
	cfg.InferenceRules = InferenceRules{
		DirectoryPatterns: []PatternRule{
			{Pattern: `^(.+)_item$`, Replacement: `${1}s`, Priority: 2},
			{Pattern: `^urgent_(.+)$`, Replacement: `urgent_${1}s`, Priority: 1},
			{Pattern: `[invalid(`, Replacement: `bad`},
		},
		KindPatterns: []PatternRule{
			{Pattern: `^(.+)s$`, Replacement: `${1}_item`, Priority: 2},
			{Pattern: `^urgent_(.+)s$`, Replacement: `urgent_${1}`, Priority: 1},
			{Pattern: `[invalid(`, Replacement: `bad`},
		},
	}

	dir := cfg.inferDirectoryFromKind("urgent_task")
	if dir != "urgent_tasks" {
		t.Errorf("inferDirectoryFromKind(urgent_task) = %s, want urgent_tasks", dir)
	}

	kind := cfg.inferKindFromDirectory("urgent_tasks")
	if kind != "urgent_task" {
		t.Errorf("inferKindFromDirectory(urgent_tasks) = %s, want urgent_task", kind)
	}

	// Reset global config and load
	ResetGlobalKindMappingsConfig()
	globalCfg := GetGlobalKindMappingsConfig("file")
	if globalCfg == nil {
		t.Fatal("expected non-nil globalCfg")
	}
}
