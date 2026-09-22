package lifecycle_builders

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type dummyLifecycleBuilder struct {
	objectType string
	version    string
	lifecycle  *objects.Lifecycle
}

func (d *dummyLifecycleBuilder) Build() *objects.Lifecycle {
	return d.lifecycle
}

func (d *dummyLifecycleBuilder) GetVersion() string {
	return d.version
}

func (d *dummyLifecycleBuilder) GetObjectType() string {
	return d.objectType
}

func TestVersionedLifecycleBuilderRegistry(t *testing.T) {
	reg := NewVersionedLifecycleBuilderRegistry()

	lc1 := &objects.Lifecycle{ObjectType: "account"}
	lc2 := &objects.Lifecycle{ObjectType: "account"}
	lc3 := &objects.Lifecycle{ObjectType: "account"}
	lcCustom := &objects.Lifecycle{ObjectType: "custom"}

	b1 := &dummyLifecycleBuilder{objectType: "account", version: "v1_0_0", lifecycle: lc1}
	b2 := &dummyLifecycleBuilder{objectType: "account", version: "v1_2_0", lifecycle: lc2}
	b3 := &dummyLifecycleBuilder{objectType: "account", version: "v1_10_0", lifecycle: lc3}
	bCustom := &dummyLifecycleBuilder{objectType: "custom", version: "custom_ver", lifecycle: lcCustom}

	reg.Register(b1)
	reg.Register(b2)
	reg.Register(b3)
	reg.Register(bCustom)

	t.Run("GetBuilder existing", func(t *testing.T) {
		got, err := reg.GetBuilder("account", "v1_2_0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.GetVersion() != "v1_2_0" {
			t.Fatalf("expected v1_2_0, got %s", got.GetVersion())
		}
	})

	t.Run("GetBuilder missing objectType", func(t *testing.T) {
		_, err := reg.GetBuilder("nonexistent", "v1_0_0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetBuilder missing version", func(t *testing.T) {
		_, err := reg.GetBuilder("account", "v9_9_9")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetLatestVersion semver comparison", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("account")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if latest != "v1_10_0" {
			t.Fatalf("expected v1_10_0, got %s", latest)
		}
	})

	t.Run("GetLatestVersion non-semver fallback", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("custom")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if latest != "custom_ver" {
			t.Fatalf("expected custom_ver, got %s", latest)
		}
	})

	t.Run("GetLatestVersion nonexistent", func(t *testing.T) {
		_, err := reg.GetLatestVersion("nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetAllObjectTypes and GetVersions", func(t *testing.T) {
		types := reg.GetAllObjectTypes()
		if len(types) < 2 {
			t.Fatalf("expected at least 2 types, got %d", len(types))
		}

		versions := reg.GetVersions("account")
		if len(versions) != 3 {
			t.Fatalf("expected 3 versions for account, got %d", len(versions))
		}

		if nilVersions := reg.GetVersions("nonexistent"); nilVersions != nil {
			t.Fatalf("expected nil for nonexistent type versions, got %v", nilVersions)
		}
	})

	t.Run("Global Registry", func(t *testing.T) {
		global := GetGlobalRegistry()
		if global == nil {
			t.Fatal("expected non-nil global registry")
		}
		RegisterBuilder(b1)
		got, err := global.GetBuilder("account", "v1_0_0")
		if err != nil || got == nil {
			t.Fatalf("expected registered builder in global registry, err: %v", err)
		}
	})
}

func TestBaseLifecycleBuilder(t *testing.T) {
	builder := NewBaseLifecycleBuilder("test_obj", "v1_0_0")
	if builder.GetObjectType() != "test_obj" {
		t.Errorf("expected test_obj, got %s", builder.GetObjectType())
	}
	if builder.GetVersion() != "v1_0_0" {
		t.Errorf("expected v1_0_0, got %s", builder.GetVersion())
	}

	builder.SetExtends("base_parent").
		SetStatusMapping(map[string]string{"orig": "target"}).
		AddStatus(objects.Status{
			Value:       "active",
			Display:     "Active",
			Origin:      true,
			Terminal:    false,
			Archive:     false,
			WorkDone:    true,
			Satisfied:   true,
			System:      false,
			Description: "Active state",
			Preconditions: []string{"step1"},
			Shockwave: objects.ShockwavePolicy{
				Mode: "sync",
				Fail: "halt",
				LineageKinds: []string{"task"},
			},
		}).
		AddTransition(objects.Transition{
			From:        "orig",
			To:          "active",
			Description: "Start work",
			Manual:      true,
			Auto:        false,
			Preconditions: []string{"check1"},
			OnDependentStatus: &objects.DependentStatusTrigger{
				Kind: "dep",
				To:   objects.StringOrSlice{"done"},
			},
			SideEffects: []objects.TransitionSideEffect{
				{Clear: "draft_flag"},
			},
			Shockwave: objects.ShockwavePolicy{
				Mode: "async",
			},
		}).
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_based",
			DefaultByStatus: map[string]any{"active": 50},
			MilestoneBased:  map[string]any{"milestone1": 100},
		})

	lc := builder.Build()
	if lc.ObjectType != "test_obj" {
		t.Errorf("expected test_obj, got %s", lc.ObjectType)
	}
	if lc.Extends != "base_parent" {
		t.Errorf("expected base_parent, got %s", lc.Extends)
	}
	if len(lc.Statuses) != 1 || lc.Statuses[0].Value != "active" {
		t.Fatalf("unexpected statuses: %#v", lc.Statuses)
	}
	if len(lc.Transitions) != 1 || lc.Transitions[0].To != "active" {
		t.Fatalf("unexpected transitions: %#v", lc.Transitions)
	}
}

func TestLifecycleGenerator(t *testing.T) {
	tempDir := t.TempDir()
	gen := NewLifecycleGenerator(tempDir)

	if err := gen.EnsureOutputDir(); err != nil {
		t.Fatalf("unexpected EnsureOutputDir error: %v", err)
	}

	dummyLC := &objects.Lifecycle{
		ObjectType: "gen_test",
		Statuses: []objects.Status{
			{Value: "init", Description: "Initial"},
		},
	}
	b := &dummyLifecycleBuilder{objectType: "gen_test", version: "v1_0_0", lifecycle: dummyLC}
	gen.registry.Register(b)

	t.Run("GenerateLifecycle", func(t *testing.T) {
		err := gen.GenerateLifecycle("gen_test", "v1_0_0")
		if err != nil {
			t.Fatalf("unexpected GenerateLifecycle error: %v", err)
		}
		path := filepath.Join(tempDir, "gen_test_lifecycle.yaml")
		content, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("expected file to exist: %v", err)
		}
		if len(content) == 0 {
			t.Fatal("expected non-empty lifecycle file")
		}
	})

	t.Run("GenerateLatestLifecycle", func(t *testing.T) {
		err := gen.GenerateLatestLifecycle("gen_test")
		if err != nil {
			t.Fatalf("unexpected GenerateLatestLifecycle error: %v", err)
		}
	})

	t.Run("GenerateAllLifecycles", func(t *testing.T) {
		err := gen.GenerateAllLifecycles()
		if err != nil {
			t.Fatalf("unexpected GenerateAllLifecycles error: %v", err)
		}
	})
}

func TestLifecycleCodegenHelpers(t *testing.T) {
	t.Run("toCamelCase", func(t *testing.T) {
		cases := map[string]string{
			"agent_skill": "AgentSkill",
			"task":        "Task",
			"":            "",
		}
		for in, want := range cases {
			if got := toCamelCase(in); got != want {
				t.Errorf("toCamelCase(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("mapKeyExpr", func(t *testing.T) {
		wireToConst := map[string]string{
			"status": "FieldKeyStatus",
		}
		if got := mapKeyExpr("status", wireToConst); got != "objects.FieldKeyStatus" {
			t.Errorf("expected objects.FieldKeyStatus, got %s", got)
		}
		if got := mapKeyExpr("other", wireToConst); got != "\"other\"" {
			t.Errorf("expected \"other\", got %s", got)
		}
	})

	t.Run("generateBuilderCode", func(t *testing.T) {
		lc := &objects.Lifecycle{
			ObjectType: "example_item",
			Extends:    "base_lifecycle",
			StatusMapping: map[string]string{
				"active": "in_progress",
			},
			PercentComplete: objects.PercentCompleteConfig{
				Method: "status_based",
				DefaultByStatus: map[string]any{"in_progress": 50},
			},
			Statuses: []objects.Status{
				{
					Value:       "open",
					Display:     "Open",
					Origin:      true,
					Terminal:    false,
					Archive:     false,
					WorkDone:    false,
					Satisfied:   false,
					System:      true,
					Description: "Initial open state",
					Preconditions: []string{"check1"},
					Shockwave: objects.ShockwavePolicy{
						Mode: "sync",
						Fail: "halt",
						LineageKinds: []string{"task"},
						ClusterKinds: []string{"group"},
						ClusterRefFields: []string{"group_ref"},
						LineageRefFields: []string{"parent_ref"},
						ExclusiveKinds: []string{"exclusive"},
					},
				},
			},
			Transitions: []objects.Transition{
				{
					From:        "open",
					To:          "closed",
					Description: "Close issue",
					Manual:      true,
					Auto:        false,
					Preconditions: []string{"all_tasks_done"},
					OnDependentStatus: &objects.DependentStatusTrigger{
						Kind: "task",
						To:   objects.StringOrSlice{"completed"},
					},
					SideEffects: []objects.TransitionSideEffect{
						{Clear: "active_run"},
						{Clear: ""},
					},
					Shockwave: objects.ShockwavePolicy{
						Mode: "async",
					},
				},
			},
		}
		code, err := generateBuilderCode(lc, "example_item", "v1_0_0", "test_pkg", map[string]string{"status": "FieldKeyStatus"}, "github.com/zqk-os/zqk", "lifecycle_builders")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code == "" {
			t.Fatal("expected non-empty code")
		}
	})

	t.Run("formatValueWithFieldKeys", func(t *testing.T) {
		wireToConst := map[string]string{
			"status": "FieldKeyStatus",
		}
		input := map[string]any{
			"status": "active",
			"count":  10,
			"uint":   uint(20),
			"flt":    3.14,
			"ok":     true,
			"nilval": nil,
			"list":   []any{"a", 1},
			"custom": "test",
		}
		res := formatValueWithFieldKeys(input, 1, wireToConst)
		if res == "" {
			t.Fatal("expected non-empty formatValueWithFieldKeys")
		}
	})

	t.Run("GenerateBuilderFromYAML", func(t *testing.T) {
		tempDir := t.TempDir()
		outDir := filepath.Join(tempDir, "pkg", "specbuilder", "lifecycle_builders")
		if err := fileutil.MkdirAll(outDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create outDir: %v", err)
		}
		_ = fileutil.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module github.com/zqk-os/zqk\n"), paths.FilePerm644)
		objectsDir := filepath.Join(tempDir, "pkg", "objects")
		if err := fileutil.MkdirAll(objectsDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create objectsDir: %v", err)
		}
		_ = fileutil.WriteFile(filepath.Join(objectsDir, "field_keys.go"), []byte("package objects\n\nconst (\n\tFieldKeyStatus = \"status\"\n)\n"), paths.FilePerm644)

		yamlPath := filepath.Join(tempDir, "sample_lifecycle.yaml")
		_ = fileutil.WriteFile(yamlPath, []byte("object_type: sample\nstatuses:\n  - value: active\n"), paths.FilePerm644)

		err := GenerateBuilderFromYAML(yamlPath, outDir)
		if err != nil {
			t.Fatalf("unexpected GenerateBuilderFromYAML error: %v", err)
		}
	})
}

