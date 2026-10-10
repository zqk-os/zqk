package builders_test

import (
	"fmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"io"
	"path/filepath"
	"sync"
	"testing"

	mcptesting "github.com/zqk-os/zqk/pkg/mcp/testing"
	"github.com/zqk-os/zqk/pkg/specbuilder/api_builders"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/specbuilder/config_builders"
	"github.com/zqk-os/zqk/pkg/specbuilder/core"
	"github.com/zqk-os/zqk/pkg/specbuilder/generators"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/specbuilder/lifecycle_builders"
	"github.com/zqk-os/zqk/pkg/specbuilder/profile_builders"
	"github.com/zqk-os/zqk/pkg/specbuilder/routing_builders"
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	// Register versioned builders so they are populated in registries
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_config_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_profile_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_routing_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_trait_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"
)

// Stress test all newly concurrent generators
func TestConcurrencyStressAllGenerators(t *testing.T) {
	t.Parallel()

	const numGoroutines = 15
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		iID := i
		goroutinelabels.NewGoroutine("builders_test", "stress all generators worker").StartSimple(func() {
			id := iID
			defer wg.Done()

			tmpDir, err := fileutil.MkdirTemp("", fmt.Sprintf("stress-test-%d-", id))
			if err != nil {
				t.Errorf("[Routine %d] Failed to create temp dir: %v", id, err)
				return
			}
			defer fileutil.RemoveAll(tmpDir)

			// 1. SpecGenerator
			specGen := builders.NewSpecGenerator(filepath.Join(tmpDir, "specs"))
			if err := specGen.GenerateAllSpecs(); err != nil {
				t.Errorf("[Routine %d] SpecGenerator failed: %v", id, err)
			}

			// 2. APIGenerator
			apiGen := api_builders.NewAPIGenerator(filepath.Join(tmpDir, "apis"))
			if err := apiGen.GenerateAllAPIs(); err != nil {
				t.Errorf("[Routine %d] APIGenerator failed: %v", id, err)
			}

			// 3. ConfigGenerator
			configGen := config_builders.NewConfigGenerator(filepath.Join(tmpDir, "configs"))
			if err := configGen.GenerateAllConfigs(); err != nil {
				t.Errorf("[Routine %d] ConfigGenerator failed: %v", id, err)
			}

			// 4. LifecycleGenerator
			lifecycleGen := lifecycle_builders.NewLifecycleGenerator(filepath.Join(tmpDir, "lifecycles"))
			if err := lifecycleGen.GenerateAllLifecycles(); err != nil {
				t.Errorf("[Routine %d] LifecycleGenerator failed: %v", id, err)
			}

			// 5. ProfileGenerator
			profileGen := profile_builders.NewProfileGenerator(filepath.Join(tmpDir, "profiles"))
			if err := profileGen.GenerateAllProfiles(); err != nil {
				t.Errorf("[Routine %d] ProfileGenerator failed: %v", id, err)
			}

			// 6. RoutingRuleGenerator
			routingGen := routing_builders.NewRoutingRuleGenerator(filepath.Join(tmpDir, "routing"))
			if err := routingGen.GenerateAllRoutingRules(); err != nil {
				t.Errorf("[Routine %d] RoutingRuleGenerator failed: %v", id, err)
			}

			// 7. TraitGenerator
			traitGen := trait_builders.NewTraitGenerator(filepath.Join(tmpDir, "traits"))
			if err := traitGen.GenerateAllTraits(); err != nil {
				t.Errorf("[Routine %d] TraitGenerator failed: %v", id, err)
			}

			// 8. ScenarioGenerator
			scenarioGen := generators.NewScenarioGenerator(filepath.Join(tmpDir, "scenarios"))
			scenarioSpec := mcptesting.ScenarioSpec{
				Name:        fmt.Sprintf("Stress scenario %d", id),
				Description: "Concurrent stress test scenario",
				Tests: []mcptesting.TestStep{
					{Name: "Echo Step", Tool: "zqk_test_echo"},
				},
			}
			if err := scenarioGen.GenerateFromSpecs([]mcptesting.ScenarioSpec{scenarioSpec}); err != nil {
				t.Errorf("[Routine %d] ScenarioGenerator failed: %v", id, err)
			}
		})
	}

	wg.Wait()
}

// TestBaseGeneratorConcurrency stresses core.BaseGenerator
type StressTestSpec struct {
	Name string
}

func (s StressTestSpec) Validate() error { return nil }
func (s StressTestSpec) GetName() string { return s.Name }

type StressTestArtifact struct {
	Value string
}

type StressTestBuilder struct {
	name string
}

func (b *StressTestBuilder) Build() StressTestArtifact {
	return StressTestArtifact{Value: "built-" + b.name}
}

type StressTestBuilderFactory struct{}

func (f *StressTestBuilderFactory) CreateBuilder(spec StressTestSpec) core.Builder[StressTestArtifact] {
	return &StressTestBuilder{name: spec.GetName()}
}

type StressTestWriter struct {
	mu    sync.Mutex
	files map[string]StressTestArtifact
}

func (w *StressTestWriter) Write(artifact StressTestArtifact, writer io.Writer) error {
	return nil
}

func (w *StressTestWriter) WriteToFile(artifact StressTestArtifact, path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.files[path] = artifact
	return nil
}

func (w *StressTestWriter) WriteToBytes(artifact StressTestArtifact) ([]byte, error) {
	return nil, nil
}

func (w *StressTestWriter) WriteToString(artifact StressTestArtifact) (string, error) {
	return "", nil
}

func TestBaseGeneratorConcurrency(t *testing.T) {
	t.Parallel()

	factory := &StressTestBuilderFactory{}
	writer := &StressTestWriter{files: make(map[string]StressTestArtifact)}
	bg := core.NewBaseGenerator[StressTestSpec, StressTestArtifact](factory, writer, filepath.Join(t.TempDir(), "out"))

	var specs []StressTestSpec
	for i := 0; i < 50; i++ {
		specs = append(specs, StressTestSpec{Name: fmt.Sprintf("spec-%d", i)})
	}

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		goroutinelabels.NewGoroutine("builders_test", "base generator specs worker").StartSimple(func() {
			defer wg.Done()
			// Concurrently call GenerateFromSpecs
			_, err := bg.GenerateFromSpecs(specs)
			if err != nil {
				t.Errorf("GenerateFromSpecs failed: %v", err)
			}

			// Concurrently call GenerateAndWriteFromSpecs
			err = bg.GenerateAndWriteFromSpecs(specs)
			if err != nil {
				t.Errorf("GenerateAndWriteFromSpecs failed: %v", err)
			}
		})
	}

	wg.Wait()
}

// Verify that instance_builders.BaseInstanceBuilder is NOT thread-safe for concurrent/shared usage.
// Calling SetField and Build concurrently on the SAME builder instance will cause logical race conditions.
func TestInstanceBuilderNotThreadSafe(t *testing.T) {
	t.Parallel()

	// Create a single shared instance of BaseInstanceBuilder
	fieldOrder := []string{"id", "value"}
	builder := instance_builders.NewBaseInstanceBuilder("account", "v2_0_0", fieldOrder, nil)

	// In a single-threaded execution, fields get cleared after Build.
	builder.ID("ACC-001")
	builder.SetField("value", "foo")

	inst, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst["id"] != "ACC-001" || inst["value"] != "foo" {
		t.Errorf("Unexpected built instance: %v", inst)
	}

	// Build again - should fail because b.fields was cleared!
	_, err = builder.Build()
	if err == nil {
		t.Error("Expected error on second Build because state was cleared, but got nil")
	}

	// Under concurrent execution, two goroutines setting fields and building on the SAME builder instance
	// will trigger logical races (one will overwrite or clear the other's fields).
	// We run this with a low goroutine count to verify the logical race/error behavior.
	const concurrentUsers = 3
	var errors []error
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(concurrentUsers)

	for i := 0; i < concurrentUsers; i++ {
		iID := i
		goroutinelabels.NewGoroutine("builders_test", "shared builder test worker").StartSimple(func() {
			id := iID
			defer wg.Done()
			builder.ID(fmt.Sprintf("ACC-%d", id))
			builder.SetField("value", fmt.Sprintf("val-%d", id))
			_, err := builder.Build()
			if err != nil {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	// Since they run concurrently on the same builder instance and Build resets fields,
	// at least one builder.Build call is expected to fail with "id is required" or get mismatched fields.
	t.Logf("Concurrent shared builder attempts returned %d errors (expected failures due to non-thread-safety)", len(errors))
	if len(errors) == 0 {
		t.Log("No errors returned in this run (timing-dependent), but builder state sharing is logically unsafe")
	}
}
