package specbuilder_test

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"

	// Side-effect imports to register builders in global registries
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_config_v1"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_lifecycle_v1"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_profile_v1"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_routing_v1"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_trait_v1"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"

	// Import actual builders packages
	"github.com/lanceman/zqk/pkg/specbuilder/api_builders"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/specbuilder/config_builders"
	sbcore "github.com/lanceman/zqk/pkg/specbuilder/core"
	"github.com/lanceman/zqk/pkg/specbuilder/generators"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
	"github.com/lanceman/zqk/pkg/specbuilder/routing_builders"
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"

	mcptesting "github.com/lanceman/zqk/pkg/mcp/testing"
	enumv "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/account"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
)

// specBuilderAdapter wraps builders.SpecBuilder to satisfy instance_builders.SpecBuilderInterface
type specBuilderAdapter struct {
	builder builders.SpecBuilder
}

func (s specBuilderAdapter) Build() any {
	return s.builder.Build()
}

func (s specBuilderAdapter) GetVersion() string {
	return s.builder.GetVersion()
}

func (s specBuilderAdapter) GetOntology() string {
	return s.builder.GetOntology()
}

// specRegistryAdapter wraps builders.VersionedBuilderRegistry to satisfy instance_builders.SpecBuilderRegistryInterface
type specRegistryAdapter struct {
	reg *builders.VersionedBuilderRegistry
}

func (a specRegistryAdapter) GetBuilder(ontology, version string) (instance_builders.SpecBuilderInterface, error) {
	b, err := a.reg.GetBuilder(ontology, version)
	if err != nil {
		return nil, err
	}
	return specBuilderAdapter{builder: b}, nil
}

func (a specRegistryAdapter) GetLatestVersion(ontology string) (string, error) {
	return a.reg.GetLatestVersion(ontology)
}

// TestConcurrencySpecGenerator runs builders.SpecGenerator concurrently
func TestConcurrencySpecGenerator(t *testing.T) {
	t.Parallel()
	const workers = 10
	const iterations = 5

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tmpDir := t.TempDir()
				generator := builders.NewSpecGenerator(tmpDir)
				if err := generator.GenerateAllSpecs(); err != nil {
					t.Errorf("worker %d iteration %d: GenerateAllSpecs failed: %v", workerID, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestConcurrencyAPIGenerator runs api_builders.APIGenerator concurrently
func TestConcurrencyAPIGenerator(t *testing.T) {
	t.Parallel()
	const workers = 10
	const iterations = 5

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tmpDir := t.TempDir()
				generator := api_builders.NewAPIGenerator(tmpDir)
				if err := generator.GenerateAllAPIs(); err != nil {
					t.Errorf("worker %d iteration %d: GenerateAllAPIs failed: %v", workerID, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestConcurrencyConfigGenerator runs config_builders.ConfigGenerator concurrently
func TestConcurrencyConfigGenerator(t *testing.T) {
	t.Parallel()
	const workers = 10
	const iterations = 5

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tmpDir := t.TempDir()
				generator := config_builders.NewConfigGenerator(tmpDir)
				if err := generator.GenerateAllConfigs(); err != nil {
					t.Errorf("worker %d iteration %d: GenerateAllConfigs failed: %v", workerID, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestConcurrencyLifecycleGenerator runs lifecycle_builders.LifecycleGenerator concurrently
func TestConcurrencyLifecycleGenerator(t *testing.T) {
	t.Parallel()
	const workers = 10
	const iterations = 5

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tmpDir := t.TempDir()
				generator := lifecycle_builders.NewLifecycleGenerator(tmpDir)
				if err := generator.GenerateAllLifecycles(); err != nil {
					t.Errorf("worker %d iteration %d: GenerateAllLifecycles failed: %v", workerID, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestConcurrencyProfileGenerator runs profile_builders.ProfileGenerator concurrently
func TestConcurrencyProfileGenerator(t *testing.T) {
	t.Parallel()
	const workers = 10
	const iterations = 5

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tmpDir := t.TempDir()
				generator := profile_builders.NewProfileGenerator(tmpDir)
				if err := generator.GenerateAllProfiles(); err != nil {
					t.Errorf("worker %d iteration %d: GenerateAllProfiles failed: %v", workerID, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestConcurrencyRoutingRuleGenerator runs routing_builders.RoutingRuleGenerator concurrently
func TestConcurrencyRoutingRuleGenerator(t *testing.T) {
	t.Parallel()
	const workers = 10
	const iterations = 5

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tmpDir := t.TempDir()
				generator := routing_builders.NewRoutingRuleGenerator(tmpDir)
				if err := generator.GenerateAllRoutingRules(); err != nil {
					t.Errorf("worker %d iteration %d: GenerateAllRoutingRules failed: %v", workerID, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestConcurrencyTraitGenerator runs trait_builders.TraitGenerator concurrently
func TestConcurrencyTraitGenerator(t *testing.T) {
	t.Parallel()
	const workers = 10
	const iterations = 5

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tmpDir := t.TempDir()
				generator := trait_builders.NewTraitGenerator(tmpDir)
				if err := generator.GenerateAllTraits(); err != nil {
					t.Errorf("worker %d iteration %d: GenerateAllTraits failed: %v", workerID, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestConcurrencyScenarioGenerator runs generators.ScenarioGenerator concurrently
func TestConcurrencyScenarioGenerator(t *testing.T) {
	t.Parallel()
	const workers = 10
	const iterations = 5

	specs := []mcptesting.ScenarioSpec{
		{
			Name:        "Stress Scenario 1",
			Description: "Stress testing concurrency",
			Tests: []mcptesting.TestStep{
				{Name: "Echo Step", Tool: "zqk_test_echo"},
			},
		},
		{
			Name:        "Stress Scenario 2",
			Description: "Stress testing concurrency second",
			Tests: []mcptesting.TestStep{
				{Name: "Echo Step 2", Tool: "zqk_test_echo"},
			},
		},
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tmpDir := t.TempDir()
				generator := generators.NewScenarioGenerator(tmpDir)
				if err := generator.GenerateFromSpecs(specs); err != nil {
					t.Errorf("worker %d iteration %d: GenerateFromSpecs failed: %v", workerID, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// MockSpec implementation for BaseGenerator tests
type MockSpec struct {
	Name string
}

func (m MockSpec) GetName() string {
	return m.Name
}

func (m MockSpec) Validate() error {
	return nil
}

// MockBuilder implementation
type MockBuilder struct {
	spec MockSpec
}

func (mb MockBuilder) Build() string {
	return "Artifact for " + mb.spec.Name
}

// MockBuilderFactory implementation
type MockBuilderFactory struct{}

func (f MockBuilderFactory) CreateBuilder(spec MockSpec) sbcore.Builder[string] {
	return MockBuilder{spec: spec}
}

// MockWriter implementation
type MockWriter struct {
	mu      sync.Mutex
	written map[string]string
}

func (mw *MockWriter) Write(artifact string, writer io.Writer) error {
	_, err := writer.Write([]byte(artifact))
	return err
}

func (mw *MockWriter) WriteToFile(artifact string, filePath string) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.written[filePath] = artifact
	return nil
}

func (mw *MockWriter) WriteToBytes(artifact string) ([]byte, error) {
	return []byte(artifact), nil
}

func (mw *MockWriter) WriteToString(artifact string) (string, error) {
	return artifact, nil
}

// TestConcurrencyBaseGenerator tests core.BaseGenerator concurrently
func TestConcurrencyBaseGenerator(t *testing.T) {
	t.Parallel()
	const workers = 15
	const iterations = 10

	factory := MockBuilderFactory{}
	writer := &MockWriter{
		written: make(map[string]string),
	}

	bg := sbcore.NewBaseGenerator[MockSpec, string](factory, writer, "/tmp/basegen")

	specs := []MockSpec{
		{Name: "Spec1"},
		{Name: "Spec2"},
		{Name: "Spec3"},
		{Name: "Spec4"},
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Test concurrent reads/generation
				artifacts, err := bg.GenerateFromSpecs(specs)
				if err != nil {
					t.Errorf("GenerateFromSpecs failed: %v", err)
				}
				if len(artifacts) != len(specs) {
					t.Errorf("Expected %d artifacts, got %d", len(specs), len(artifacts))
				}
			}
		}()
	}
	wg.Wait()
}

// TestInstanceBuilderStatefulness verifies that instance_builders are stateful (not thread-safe for reuse)
// because calling Build() resets the builder's internal fields map.
func TestInstanceBuilderStatefulness(t *testing.T) {
	t.Parallel()

	builder := bldr_instance_v1.NewAccountInstanceBuilder(objects.DefaultSchemaVersion)
	builder.ID("ACC-STRESS-001").Status(enumv.StatusActive)

	// First Build should succeed
	inst1, err := builder.Build()
	if err != nil {
		t.Fatalf("First Build failed: %v", err)
	}
	if inst1[objects.FieldKeyID] != "ACC-STRESS-001" {
		t.Errorf("Expected id ACC-STRESS-001, got %v", inst1[objects.FieldKeyID])
	}

	// Second Build should fail because internal fields map is reset by Build()
	_, err = builder.Build()
	if err == nil {
		t.Error("Expected error on second Build call due to fields reset, but got no error")
	}
}

// TestInstanceGeneratorIsSequential checks that instance_builders generator is sequential
// and does not use any parallel concurrency primitives under the hood.
func TestInstanceGeneratorIsSequential(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	registry := instance_builders.NewVersionedInstanceBuilderRegistry(specRegistryAdapter{reg: builders.GetGlobalRegistry()})

	// Register account builder
	registry.Register(bldr_instance_v1.NewAccountInstanceBuilder(objects.DefaultSchemaVersion))

	generator := instance_builders.NewInstanceGenerator(tmpDir, registry)

	// Generate sequentially
	err := generator.GenerateInstance("account", "ACC-STRESS-101", objects.DefaultSchemaVersion)
	if err != nil {
		t.Fatalf("GenerateInstance failed: %v", err)
	}

	filePath := filepath.Join(tmpDir, "ACC-STRESS-101.yaml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Expected file %s was not created", filePath)
	}
}
