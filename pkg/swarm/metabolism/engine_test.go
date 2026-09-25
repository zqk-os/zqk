package metabolism

import (
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow"
)

func TestMetabolismEngine_IngestAndSynthesize(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "swarm.yaml")
	if err := pack.EnsureSampleSwarm(manifestPath); err != nil {
		t.Fatalf("EnsureSampleSwarm failed: %v", err)
	}

	// Add templates to pack
	templatesDir := filepath.Join(tmpDir, "templates")
	if err := fileutil.MkdirAll(templatesDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	tplFile := filepath.Join(templatesDir, "rdb_eval.yaml")
	if err := fileutil.WriteFile(tplFile, []byte("prompt: Evaluate readability invariants\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Seal pack
	if _, err := pack.SealPack(tmpDir, priv, "architect@zqk.dev"); err != nil {
		t.Fatalf("SealPack failed: %v", err)
	}

	// Initialize engine
	registry := NewReceptorRegistry()
	engine := NewMetabolismEngine(registry)

	outDir := filepath.Join(t.TempDir(), "output")
	opts := IngestionOptions{
		PackDir:    tmpDir,
		PublicKey:  pub,
		OutputDir:  outDir,
		VerifySeal: true,
		Parameters: map[string]interface{}{
			"baseline": 4.6,
		},
	}

	digest, err := engine.Ingest(opts)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if digest.Lease == nil {
		t.Fatal("expected active lease in digest, got nil")
	}

	// 1. Verify Prompt Templates
	if len(digest.PromptTemplates) != 1 {
		t.Fatalf("expected 1 prompt template, got %d", len(digest.PromptTemplates))
	}
	if digest.PromptTemplates[0].ID != "TPL-SAMPLE_REFACTOR_SWARM-RDB_EVAL" {
		t.Errorf("unexpected template ID: %s", digest.PromptTemplates[0].ID)
	}

	// 2. Verify Ontological Graph Synthesis
	var goal, mls, pri, cvs map[string]any
	var reqs, criteria, tests, blis []map[string]any

	for _, obj := range digest.KernelObjects {
		kind, _ := obj[objects.FieldKeyKind].(string)
		switch kind {
		case "goal":
			goal = obj
		case "milestone":
			mls = obj
		case "priority_plan":
			pri = obj
		case "convergence_session":
			cvs = obj
		case "requirement":
			reqs = append(reqs, obj)
		case "criteria":
			criteria = append(criteria, obj)
		case "test_case":
			tests = append(tests, obj)
		case "backlog_item":
			blis = append(blis, obj)
		}
	}

	if goal == nil || mls == nil || pri == nil || cvs == nil {
		t.Fatalf("missing core spine objects: goal=%v mls=%v pri=%v cvs=%v", goal, mls, pri, cvs)
	}

	// Check sample pack tasks count (2 tasks: task-refactor, task-verify)
	if len(reqs) != 2 || len(blis) != 2 || len(tests) != 2 {
		t.Fatalf("expected 2 reqs, 2 blis, 2 tests; got %d reqs, %d blis, %d tests", len(reqs), len(blis), len(tests))
	}

	// Each requirement must have 3 criteria (Three-Fold Formula) -> total 6 criteria
	if len(criteria) != 6 {
		t.Fatalf("expected 6 criteria (3 per task), got %d", len(criteria))
	}

	// 3. Verify that each synthesized requirement satisfies the Anti-Superficiality Shovel-Ready Gate!
	for _, req := range reqs {
		reqID, _ := req[objects.FieldKeyID].(string)
		critRefs, _ := req[objects.FieldKeyCriteriaRefs].([]string)
		critSet := make(map[string]bool)
		for _, ref := range critRefs {
			critSet[ref] = true
		}
		var linkedCrit []map[string]any
		for _, c := range criteria {
			cID, _ := c[objects.FieldKeyID].(string)
			if critSet[cID] {
				linkedCrit = append(linkedCrit, c)
			}
		}

		if err := workflow.CheckShovelReadyQuality(req, linkedCrit); err != nil {
			t.Fatalf("synthesized requirement %s failed shovel-ready quality gate: %v", reqID, err)
		}
	}

	// 4. Overdose / Receptor Saturation Test
	_, err = engine.Ingest(opts)
	if err == nil {
		t.Fatal("expected ErrReceptorSaturated on concurrent ingest, got nil")
	}

	// 5. Complete Ingestion
	if err := engine.CompleteIngestion(digest, "converged"); err != nil {
		t.Fatalf("CompleteIngestion failed: %v", err)
	}

	// Re-ingest now succeeds and advances lineage epoch
	opts.SessionID = "session-epoch-2"
	digest2, err := engine.Ingest(opts)
	if err != nil {
		t.Fatalf("re-ingest after completion failed: %v", err)
	}
	if digest2.Lease.Epoch != 2 {
		t.Errorf("expected epoch 2 on re-ingest, got %d", digest2.Lease.Epoch)
	}
}

func TestMetabolismEngine_TeamConfiguration(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	t.Run("RefMode", func(t *testing.T) {
		tmpDir := t.TempDir()
		manifestContent := `
name: archetype-swarm
version: 1.0.0
description: Swarm using referenced team archetype
team_configuration_ref: TCFG-CEF-DIAMOND-EVALUATION
tasks:
  - id: eval-task
    title: Evaluate Codebase
`
		if err := fileutil.WriteFile(filepath.Join(tmpDir, "swarm.yaml"), []byte(manifestContent), paths.FilePerm644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if _, err := pack.SealPack(tmpDir, priv, "signer@zqk.dev"); err != nil {
			t.Fatalf("seal failed: %v", err)
		}

		engine := NewMetabolismEngine(NewReceptorRegistry())
		digest, err := engine.Ingest(IngestionOptions{
			PackDir:    tmpDir,
			PublicKey:  pub,
			OutputDir:  filepath.Join(t.TempDir(), "output"),
			VerifySeal: true,
		})
		if err != nil {
			t.Fatalf("ingest failed: %v", err)
		}

		var pri map[string]any
		for _, obj := range digest.KernelObjects {
			if k, _ := obj[objects.FieldKeyKind].(string); k == "priority_plan" {
				pri = obj
				break
			}
		}
		if pri == nil {
			t.Fatal("priority_plan not synthesized")
		}
		if ref, _ := pri[objects.FieldKeyTeamConfigurationRef].(string); ref != "TCFG-CEF-DIAMOND-EVALUATION" {
			t.Errorf("expected team_configuration_ref TCFG-CEF-DIAMOND-EVALUATION, got %q", ref)
		}
	})

	t.Run("InlineMode", func(t *testing.T) {
		tmpDir := t.TempDir()
		manifestContent := `
name: inline-pod-swarm
version: 1.0.0
description: Swarm using inline team configuration
team_configuration:
  id: TCFG-INLINE-POD
  cell_type: neuron
  focus_area: architecture_evaluation
  persona_allocations:
    - persona_ref: PRS-SYSTEMS-ARCHITECT
      role: architect
      count: 2
tasks:
  - id: pod-task
    title: Architect Assessment
`
		if err := fileutil.WriteFile(filepath.Join(tmpDir, "swarm.yaml"), []byte(manifestContent), paths.FilePerm644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if _, err := pack.SealPack(tmpDir, priv, "signer@zqk.dev"); err != nil {
			t.Fatalf("seal failed: %v", err)
		}

		engine := NewMetabolismEngine(NewReceptorRegistry())
		digest, err := engine.Ingest(IngestionOptions{
			PackDir:    tmpDir,
			PublicKey:  pub,
			OutputDir:  filepath.Join(t.TempDir(), "output"),
			VerifySeal: true,
		})
		if err != nil {
			t.Fatalf("ingest failed: %v", err)
		}

		var pri, tcfg map[string]any
		for _, obj := range digest.KernelObjects {
			switch k, _ := obj[objects.FieldKeyKind].(string); k {
			case "priority_plan":
				pri = obj
			case "team_configuration":
				tcfg = obj
			}
		}
		if pri == nil {
			t.Fatal("priority_plan not synthesized")
		}
		if tcfg == nil {
			t.Fatal("team_configuration not synthesized into kernel objects")
		}
		if id, _ := tcfg[objects.FieldKeyID].(string); id != "TCFG-INLINE-POD" {
			t.Errorf("expected TCFG-INLINE-POD, got %s", id)
		}
		if ct, _ := tcfg["cell_type"].(string); ct != "neuron" {
			t.Errorf("expected cell_type neuron, got %s", ct)
		}
		if ref, _ := pri[objects.FieldKeyTeamConfigurationRef].(string); ref != "TCFG-INLINE-POD" {
			t.Errorf("expected priority_plan to reference TCFG-INLINE-POD, got %s", ref)
		}
		personas, _ := pri[objects.FieldKeyPersonaRefs].([]string)
		if len(personas) != 1 || personas[0] != "PRS-SYSTEMS-ARCHITECT" {
			t.Errorf("expected persona PRS-SYSTEMS-ARCHITECT in priority_plan, got %v", personas)
		}
	})
}

