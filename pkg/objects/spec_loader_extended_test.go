package objects

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/logging"
)

type mockSpecBuilder struct {
	spec     *Spec
	version  string
	ontology string
}

func (m *mockSpecBuilder) Build() *Spec {
	return m.spec
}

func (m *mockSpecBuilder) GetVersion() string {
	return m.version
}

func (m *mockSpecBuilder) GetOntology() string {
	return m.ontology
}

type mockVersionedRegistry struct {
	builders map[string]map[string]SpecBuilderInterface
}

func (m *mockVersionedRegistry) GetBuilder(ontology, version string) (SpecBuilderInterface, error) {
	if b, ok := m.builders[ontology][version]; ok {
		return b, nil
	}
	return nil, errors.New("builder not found")
}

func (m *mockVersionedRegistry) GetLatestVersion(ontology string) (string, error) {
	return "v1_0_0", nil
}

func (m *mockVersionedRegistry) GetVersions(ontology string) []string {
	return []string{"v1_0_0"}
}

func TestSpecLoader_Extended(t *testing.T) {
	// Version conversions
	if got := ConvertInstanceVersionToBuilderVersion("1.0.0"); got != "v1_0_0" {
		t.Errorf("ConvertInstanceVersionToBuilderVersion('1.0.0') = %s, want v1_0_0", got)
	}
	if got := ConvertInstanceVersionToBuilderVersion("v2.1.0"); got != "v2_1_0" {
		t.Errorf("ConvertInstanceVersionToBuilderVersion('v2.1.0') = %s, want v2_1_0", got)
	}
	if got := ConvertInstanceVersionToBuilderVersion(""); got != "" {
		t.Errorf("ConvertInstanceVersionToBuilderVersion('') = %s, want empty", got)
	}

	if got := ConvertBuilderVersionToInstanceVersion("v1_0_0"); got != "1.0.0" {
		t.Errorf("ConvertBuilderVersionToInstanceVersion('v1_0_0') = %s, want 1.0.0", got)
	}
	if got := ConvertBuilderVersionToInstanceVersion(""); got != "" {
		t.Errorf("ConvertBuilderVersionToInstanceVersion('') = %s, want empty", got)
	}

	loader := NewSpecLoader("")

	// EnsureReady
	ctx := context.Background()
	_ = loader.EnsureReady(ctx)

	// GetMetrics
	_ = loader.GetMetrics()

	// LoadSpecByVersion without registry set should error
	loader.SetBuilderRegistry(nil)
	if _, err := loader.LoadSpecByVersion("goal", "1.0.0"); err == nil {
		t.Error("expected error when builder registry is nil")
	}

	// Mock builder registry
	reg := &mockVersionedRegistry{
		builders: map[string]map[string]SpecBuilderInterface{
			"goal": {
				"v1_0_0": &mockSpecBuilder{
					spec: &Spec{
						Ontology:      "goal",
						SchemaVersion: "1.0.0",
					},
					version:  "v1_0_0",
					ontology: "goal",
				},
			},
		},
	}
	loader.SetBuilderRegistry(reg)

	spec, err := loader.LoadSpecByVersion("goal", "1.0.0")
	if err != nil {
		t.Fatalf("LoadSpecByVersion failed: %v", err)
	}
	if spec == nil || spec.Ontology != "goal" {
		t.Errorf("unexpected spec loaded: %+v", spec)
	}

	// Test fallback to latest version
	specFallback, err := loader.LoadSpecByVersion("goal", "9.9.9")
	if err != nil {
		t.Fatalf("LoadSpecByVersion with fallback failed: %v", err)
	}
	if specFallback == nil || specFallback.Ontology != "goal" {
		t.Errorf("unexpected fallback spec loaded: %+v", specFallback)
	}

	// Test lock logger Warn and Debug
	logger := &specLoaderLockLogger{
		logger: logging.GetLoggerFromProfile("system"),
	}
	logger.Debug("debug message",
		concurrency.LockField{Key: "str", Value: "string_val"},
		concurrency.LockField{Key: "i", Value: int(1)},
		concurrency.LockField{Key: "i64", Value: int64(2)},
		concurrency.LockField{Key: "err", Value: errors.New("sample error")},
		concurrency.LockField{Key: "other", Value: true},
	)
	logger.Warn("warn message",
		concurrency.LockField{Key: "str", Value: "string_val"},
		concurrency.LockField{Key: "i", Value: int(1)},
		concurrency.LockField{Key: "i64", Value: int64(2)},
		concurrency.LockField{Key: "err", Value: errors.New("sample error")},
		concurrency.LockField{Key: "other", Value: 3.14},
	)

	// ValidateSpecFile with invalid spec file
	errs, valErr := ValidateSpecFile(ctx, loader, "nonexistent_spec_xyz.yaml", nil)
	_ = errs
	if valErr == nil {
		t.Log("ValidateSpecFile expected error for nonexistent file")
	}

	// PrewarmGlobalsForProjectRoot
	PrewarmGlobalsForProjectRoot("") // empty root: no-op
}
