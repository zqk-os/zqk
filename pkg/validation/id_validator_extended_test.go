package validation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestIDValidator_LoadingAndPathsExtended(t *testing.T) {
	// NewIDValidatorWithGraph
	vGraph := NewIDValidatorWithGraph("", nil)
	if vGraph == nil {
		t.Fatal("expected non-nil IDValidator from NewIDValidatorWithGraph")
	}
	if vGraph.hasGraphBackend() {
		t.Errorf("expected hasGraphBackend false when nil conn")
	}

	// SetGraphConnection
	vGraph.SetGraphConnection(nil)
	if vGraph.hasGraphBackend() {
		t.Errorf("expected hasGraphBackend false")
	}

	// hasFileBackend
	vGraph.SetSpecsDirForTest("/some/test/specs")
	if !vGraph.hasFileBackend() {
		t.Errorf("expected hasFileBackend true")
	}

	// InjectPatternForTest
	vGraph.InjectPatternForTest("test_kind", "^TEST-[0-9]+$", "TEST-")
	if vGraph.patterns["test_kind"] == nil || vGraph.patterns["test_kind"].Pattern != "^TEST-[0-9]+$" {
		t.Errorf("expected injected pattern to exist")
	}

	// NewIDValidatorWithGraphAndConfigs
	prefixCfg := getDefaultIDPrefixesConfig()
	pathsCfg := getDefaultPathsConfig()
	vConfigs := NewIDValidatorWithGraphAndConfigs("", nil, prefixCfg, pathsCfg)
	if vConfigs.getIDPrefixesConfig() != prefixCfg {
		t.Errorf("expected injected prefixCfg")
	}
	if vConfigs.getPathsConfig() != pathsCfg {
		t.Errorf("expected injected pathsCfg")
	}

	// loadDefaultPatterns
	vGraph.loadDefaultPatterns()
	if len(vGraph.patterns) == 0 {
		t.Errorf("expected patterns populated from loadDefaultPatterns")
	}

	// loadPatternsFromSpecs
	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "sample_kind.yaml")
	specContent := `
kind: sample_kind
id_pattern: "^SMP-[0-9]+$"
prefixes: ["SMP-"]
`
	if err := os.WriteFile(specFile, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write temp spec file: %v", err)
	}

	vGraph.SetSpecsDirForTest(tmpDir)
	if err := vGraph.loadPatternsFromSpecs(); err != nil {
		t.Fatalf("loadPatternsFromSpecs failed: %v", err)
	}
	if vGraph.patterns["sample_kind"] == nil {
		t.Errorf("expected sample_kind pattern loaded from specs")
	}

	// findPathByWalkingUp and hasMarkerInDir
	markerDir := t.TempDir()
	subDir := filepath.Join(markerDir, "nested", "level")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create nested dirs: %v", err)
	}
	gitMarker := filepath.Join(markerDir, ".git")
	if err := os.Mkdir(gitMarker, 0755); err != nil {
		t.Fatalf("failed to create marker dir: %v", err)
	}
	targetFile := filepath.Join(markerDir, "file.txt")
	if err := os.WriteFile(targetFile, []byte("ok"), 0644); err != nil {
		t.Fatalf("failed to write target file: %v", err)
	}

	pathsCfgWithMarker := &PathsConfig{
		SearchStrategy: SearchStrategyConfig{
			Markers: []string{".git"},
		},
	}
	found := findPathByWalkingUp(subDir, "file.txt", false, pathsCfgWithMarker)
	if found != targetFile {
		t.Errorf("findPathByWalkingUp = %s, want %s", found, targetFile)
	}
}

func TestIDValidator_LoadPatternsFromGraph(t *testing.T) {
	ctx := context.Background()
	mockProvider := provider.NewMockGraphProvider()
	conn, err := mockProvider.Connect(ctx, provider.ConnectionConfig{})
	if err != nil {
		t.Fatalf("failed to connect to mock graph: %v", err)
	}

	// Create an ObjectSpec node in graph
	specNode := provider.Node{
		ID:     "SPEC-GOAL",
		Labels: []string{"ObjectSpec", "Spec"},
		Properties: map[string]any{
			objects.FieldKeyOntology: "goal",
			"id_template":            "GOAL-{sequence}",
		},
	}
	if err := conn.CreateNode(ctx, specNode); err != nil {
		t.Fatalf("failed to create node in mock graph: %v", err)
	}

	v := NewIDValidatorWithGraph("", conn)
	patterns, err := v.loadPatternsFromGraphUnlocked(ctx, conn)
	if err != nil {
		t.Fatalf("loadPatternsFromGraphUnlocked failed: %v", err)
	}
	if patterns["goal"] == nil || patterns["goal"].Template != "GOAL-{sequence}" {
		t.Errorf("expected goal pattern loaded from graph, got %v", patterns["goal"])
	}

	// loadPatternsFromGraph with connected graph
	if err := v.loadPatternsFromGraph(ctx); err != nil {
		t.Errorf("loadPatternsFromGraph failed: %v", err)
	}

	// loadPatternsFromGraph with nil graph connection
	vNoConn := NewIDValidatorWithGraph("", nil)
	if err := vNoConn.loadPatternsFromGraph(ctx); err == nil {
		t.Errorf("expected error from loadPatternsFromGraph with nil conn")
	}

	// NewIDValidatorWithConfigs
	prefixCfg := getDefaultIDPrefixesConfig()
	pathsCfg := getDefaultPathsConfig()
	vWithConfigs := NewIDValidatorWithConfigs("", prefixCfg, pathsCfg)
	if vWithConfigs == nil {
		t.Errorf("expected non-nil from NewIDValidatorWithConfigs")
	}
}

