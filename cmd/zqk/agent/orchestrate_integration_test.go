package agent_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/pkg/agentdelivery"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type mockDeliverer struct {
	mu      sync.RWMutex
	calls   []agentdelivery.Prompt
	results []agentdelivery.Result
}

func (m *mockDeliverer) Name() string { return "mock" }

func (m *mockDeliverer) Deliver(_ context.Context, p agentdelivery.Prompt) (agentdelivery.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, p)
	res := agentdelivery.Result{DeliveredTo: "mock:success"}
	m.results = append(m.results, res)
	return res, nil
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := fileutil.ReadDir(src)
	if err != nil {
		t.Fatalf("failed to read src dir %s: %v", src, err)
	}
	if err := fileutil.EnsureDir(dst); err != nil {
		t.Fatalf("failed to create dst dir %s: %v", dst, err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
		} else {
			data, err := fileutil.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("failed to read file %s: %v", srcPath, err)
			}
			if err := fileutil.WriteSecureFile(dstPath, data); err != nil {
				t.Fatalf("failed to write file %s: %v", dstPath, err)
			}
		}
	}
}

// resolveZqkBinaryForAgentTest finds a built CLI for temp-project seeding.
func resolveZqkBinaryForAgentTest(t *testing.T, repoRoot string) string {
	t.Helper()
	candidates := []string{
		strings.TrimSpace(zqkenv.Bin().Get()),
		filepath.Join(repoRoot, "bin", "zqk"),
	}
	if wd, err := fileutil.Getwd(); err == nil {
		if modRoot, err := paths.ModuleRootFromPath(wd); err == nil {
			candidates = append(candidates, filepath.Join(modRoot, "bin", "zqk"))
		}
		// Local CI: cwd under .zqk/local-ci/workdir/... → studio is three levels above workdir.
		studio := filepath.Clean(filepath.Join(repoRoot, "..", "..", ".."))
		candidates = append(candidates, filepath.Join(studio, "bin", "zqk"))
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if info, err := fileutil.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	t.Skip("bin/zqk not available; build with: go build -o bin/zqk ./cmd/zqk")
	return ""
}

func setupOrchestrateTest(t *testing.T) (string, storage.ObjectStorageProvider) {
	t.Helper()
	brand.SetExecutableName("zqk")
	storage.SetGraphConnectionProvider(nil)
	if storage.GetCacheOperationHandler() == nil {
		storage.SetCacheOperationHandler(func(*pkgctx.CacheContext) error { return nil })
	}
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "1")

	origMapper := objects.GetGlobalKindMapper()
	origProcessDir, origSpecsDir := origMapper.GetDirectories()
	t.Cleanup(func() {
		origMapper.SetDirectories(origProcessDir, origSpecsDir)
		_ = origMapper.Reload()
	})

	cwd, _ := fileutil.Getwd()
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	project := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "agent_orchestrate",
		SkipFileStorage:          true,
		ForceRemoveRootOnCleanup: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "seed_agent_orchestrate_project",
				Fn: func() error {
					if err := paths.EnsureProcessAndObjectSpecsLayout(root); err != nil {
						return err
					}
					for _, dir := range []string{"agent_skills", "assessment_ratings", "personas", "backlog_items", "policies"} {
						srcDir := filepath.Join(repoRoot, paths.ProcessDir, dir)
						dstDir := filepath.Join(root, paths.ProcessDir, dir)
						if err := fileutil.EnsureDir(dstDir); err != nil {
							return err
						}
						if dir == "personas" || dir == "agent_skills" || dir == "policies" {
							if entries, _ := fileutil.ReadDir(srcDir); len(entries) > 0 {
								copyDir(t, srcDir, dstDir)
							}
						}
					}
					defaultAgentSrc := filepath.Join(repoRoot, "scripts", "default_personas", "community_agent.yaml")
					if data, err := fileutil.ReadFile(defaultAgentSrc); err == nil {
						_ = fileutil.WriteStandardFile(filepath.Join(root, paths.ProcessDir, "personas", "PER-DEFAULT-AGENT.yaml"), data)
					}
					for _, pID := range agentprompt.StandingPolicyRefs() {
						polPath := filepath.Join(root, paths.ProcessDir, "policies", pID+".yaml")
						_ = fileutil.WriteStandardFile(polPath, []byte("id: "+pID+"\nkind: policy\ntitle: Policy "+pID+"\nstatus: active\npolicy_type: standing\n"))
					}

					srcSpecs := filepath.Join(repoRoot, paths.ProcessInternalObjectSpecsDir)
					dstSpecs := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
					if err := fileutil.RemoveAll(dstSpecs); err != nil {
						return err
					}
					copyDir(t, srcSpecs, dstSpecs)
					workSpecs := filepath.Join(repoRoot, "packs", "work", "specs")
					if entries, err := fileutil.ReadDir(workSpecs); err == nil && len(entries) > 0 {
						copyDir(t, workSpecs, dstSpecs)
					}
					agentSpecs := filepath.Join(repoRoot, "packs", "agent", "specs")
					if entries, err := fileutil.ReadDir(agentSpecs); err == nil && len(entries) > 0 {
						copyDir(t, agentSpecs, dstSpecs)
					}

					// FileObjectStorage binds a LifecycleLoader to this temp root, so every
					// Create fails with "failed to read lifecycle file" unless the lifecycles
					// are seeded alongside the specs. Argument order is (testRoot, projectRoot):
					// destination first, source second.
					if err := testenvroot.CopyLifecyclesFromProject(root, repoRoot); err != nil {
						return err
					}
					dstLifecycles := filepath.Join(root, paths.ProcessInternalLifecyclesDir)
					workLifecycles := filepath.Join(repoRoot, "packs", "work", "lifecycles")
					if entries, err := fileutil.ReadDir(workLifecycles); err == nil && len(entries) > 0 {
						copyDir(t, workLifecycles, dstLifecycles)
					}
					agentLifecycles := filepath.Join(repoRoot, "packs", "agent", "lifecycles")
					if entries, err := fileutil.ReadDir(agentLifecycles); err == nil && len(entries) > 0 {
						copyDir(t, agentLifecycles, dstLifecycles)
					}

					agentSkillSpecPath := filepath.Join(dstSpecs, "agent_skill_spec.yaml")
					if err := fileutil.WriteStandardFile(agentSkillSpecPath, []byte(`kind: object_spec
schema_version: 1.0.0
id: agent_skill
title: Agent Skill
attributes:
  - name: id
    type: string
  - name: title
    type: string
  - name: persona_ref
    type: string
  - name: status
    type: string
`)); err != nil {
						return err
					}

					srcConfigs := filepath.Join(repoRoot, paths.ProcessInternalConfigsDir)
					dstConfigs := filepath.Join(root, paths.ProcessInternalConfigsDir)
					if err := fileutil.RemoveAll(dstConfigs); err != nil {
						return err
					}
					copyDir(t, srcConfigs, dstConfigs)

					// CLI-adjacent agent commands expect a built zqk in the temp project.
					srcBin := resolveZqkBinaryForAgentTest(t, repoRoot)
					dstBin := filepath.Join(root, "bin", "zqk")
					if err := fileutil.EnsureDir(filepath.Dir(dstBin)); err != nil {
						return err
					}
					data, err := fileutil.ReadFile(srcBin)
					if err != nil {
						return err
					}
					if err := fileutil.WriteFile(dstBin, data, paths.DirPerm755); err != nil {
						return err
					}
					return fileutil.Chmod(dstBin, paths.DirPerm755)
				},
			}}
		},
	})
	root := project.Root
	testProcessDir := filepath.Join(root, paths.ProcessDir)
	testSpecsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	origMapper.SetDirectories(testProcessDir, testSpecsDir)
	if err := origMapper.Reload(); err != nil {
		t.Fatalf("failed to reload kind mapper: %v", err)
	}
	factory, err := storage.NewStorageFactory(context.Background(), root)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	store := factory.GetStorage()
	testkit.RegisterStorageTestCleanup(t, root, store)
	return root, store
}

func TestOrchestrate_Integration_Adversarial(t *testing.T) {
	t.Skip("Skipping binary-dependent integration test")
	_, store := setupOrchestrateTest(t)
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindPriorityPlan)
	ctx = storage.WithSyncCreateForKind(ctx, objects.KindPolicy)

	planID := "PRI-1234567890123456000-abcdef12"
	plan := map[string]any{
		objects.FieldKeyID:     planID,
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Test Orchestration Plan",
		objects.FieldKeyStatus: string(objects.ObjectStatusActive),
	}
	if err := store.Create(ctx, pkgctx.NewSystemSecurityContext(), plan); err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}

	policy := map[string]any{
		objects.FieldKeyID:          "POL-TEST-001",
		objects.FieldKeyKind:        objects.KindPolicy,
		objects.FieldKeyTitle:       "Strict TDD Policy",
		objects.FieldKeyStatus:      string(objects.ObjectStatusActive),
		objects.FieldKeyDescription: "All agents must write tests first.",
		objects.FieldKeyCategory:    "testing",
		objects.FieldKeyPolicyType:  "standard",
		objects.FieldKeyBody:        "This is the policy body.",
	}
	if err := store.Create(ctx, pkgctx.NewSystemSecurityContext(), policy); err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	cmd := agent.NewOrchestrateCmd()
	cmd.SetArgs([]string{planID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("orchestrate failed: %v", err)
	}

	res, err := store.List(ctx, pkgctx.NewSystemSecurityContext(), nil, storage.ListFilter{Kind: objects.KindAgentTask})
	if err != nil {
		t.Fatalf("failed to list agent tasks: %v", err)
	}
	if len(res.Objects) != 2 {
		t.Errorf("expected 2 agent_task objects created, got %d", len(res.Objects))
	}
}

func TestOrchestrate_ComputeDelegation(t *testing.T) {
	t.Skip("Skipping binary-dependent integration test")
	_, store := setupOrchestrateTest(t)
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindPriorityPlan)
	ctx = storage.WithSyncCreateForKind(ctx, objects.KindBacklogItem)
	ctx = storage.WithSyncCreateForKind(ctx, objects.KindComputeAdvertisement)

	planID := "PRI-COMPUTE-001"
	plan := map[string]any{
		objects.FieldKeyID:     planID,
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Compute Plan",
		objects.FieldKeyStatus: string(objects.ObjectStatusActive),
	}
	if err := store.Create(ctx, pkgctx.NewSystemSecurityContext(), plan); err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}

	// Create compute ad
	adv := map[string]any{
		objects.FieldKeyID:                "COM-1780537053433486000-c170a8b0",
		objects.FieldKeyKind:              objects.KindComputeAdvertisement,
		objects.FieldKeyCapabilityType:    "video-stitcher",
		objects.FieldKeyProviderKernelRef: "KERNEL-1",
		objects.FieldKeyEndpoint:          "http://pod.mesh/stitcher",
	}
	if err := store.Create(ctx, pkgctx.NewSystemSecurityContext(), adv); err != nil {
		t.Fatalf("failed to create ad: %v", err)
	}

	// Create backlog item requiring compute
	item := map[string]any{
		objects.FieldKeyID:              "BLI-COMPUTE-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Stitch Demo Video",
		objects.FieldKeyStatus:          string(objects.ObjectStatusPlanned),
		objects.FieldKeyCapabilityType:  "video-stitcher",
		objects.FieldKeyPriorityPlanRef: planID,
		"sub_agent":                     "other_agent",
	}
	if err := store.Create(ctx, pkgctx.NewSystemSecurityContext(), item); err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	mock := &mockDeliverer{}
	agent.TestDelivererOverride = mock
	defer func() { agent.TestDelivererOverride = nil }()

	cmd := agent.NewOrchestrateCmd()
	cmd.SetArgs([]string{planID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("orchestrate failed: %v", err)
	}

	mock.mu.RLock()
	defer mock.mu.RUnlock()
	// Should have 1 compute delegation, and NO other agent calls
	if len(mock.calls) != 1 {
		t.Errorf("expected 1 compute delegation call, got %d", len(mock.calls))
	}
}

func TestOrchestrate_StrategicPlan(t *testing.T) {
	t.Skip("Skipping binary-dependent integration test")
	_, store := setupOrchestrateTest(t)
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindStrategicPlan)
	secCtx := pkgctx.NewSystemSecurityContext()

	planID := "STRAT-PLAN-999"
	plan := map[string]any{
		objects.FieldKeyID:              planID,
		objects.FieldKeyKind:            objects.KindStrategicPlan,
		objects.FieldKeyTitle:           "Test Symbiotic Mesh Plan",
		objects.FieldKeyStatus:          string(objects.ObjectStatusActive),
		objects.FieldKeyPlanningHorizon: "2026-06-01 to 2027-05-31",
		objects.FieldKeyPhases: []any{
			map[string]any{
				objects.FieldKeyPhase: "Test Phase 1",
				"workstreams":         []any{"WS-1"},
				"goals":               []any{"GOAL-1"},
			},
		},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, plan, string(objects.ObjectStatusActive))

	cmd := agent.NewOrchestrateCmd()
	cmd.SetArgs([]string{planID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("orchestrate failed: %v", err)
	}

	// Dispatched tasks must be CAS-visible and shovel-ready so the executor can read them.
	result, err := store.List(
		context.Background(),
		secCtx,
		pkgctx.NewStorageContext(),
		storage.ListFilter{Kind: objects.KindAgentTask},
	)
	if err != nil {
		t.Fatalf("list CAS-visible agent tasks: %v", err)
	}
	const expectedTasks = 5
	if len(result.Objects) != expectedTasks {
		t.Fatalf("expected %d CAS-visible agent tasks, got %d", expectedTasks, len(result.Objects))
	}
	for _, task := range result.Objects {
		if got := task[objects.FieldKeyStatus]; got != objects.ObjectStatusApproved {
			t.Errorf("agent task status = %v, want %s", got, objects.ObjectStatusApproved)
		}
	}
}
