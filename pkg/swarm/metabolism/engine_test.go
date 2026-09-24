package metabolism

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
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
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	tplFile := filepath.Join(templatesDir, "rdb_eval.yaml")
	if err := os.WriteFile(tplFile, []byte("prompt: Evaluate readability invariants\n"), 0644); err != nil {
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
		var linkedCrit []map[string]any
		for _, c := range criteria {
			cReqs, _ := c[objects.FieldKeyRequirementRefs].([]string)
			for _, rRef := range cReqs {
				if rRef == reqID {
					linkedCrit = append(linkedCrit, c)
				}
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
