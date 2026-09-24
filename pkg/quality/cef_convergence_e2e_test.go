package quality

import (
	"crypto/ed25519"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow"
)

func TestCEFConvergence_EndToEnd(t *testing.T) {
	packDir := filepath.Join("..", "..", "packs", "code-eval")
	if _, err := fileutil.Stat(packDir); err != nil {
		t.Skipf("packs/code-eval not found: %v", err)
	}

	// 1. Verify Immune Membrane & Attestation
	// Public key corresponding to seed "zqk-canonical-cef-pack-seed-32b!"
	privKey := ed25519.NewKeyFromSeed([]byte("zqk-canonical-cef-pack-seed-32b!"))
	pubKey := privKey.Public().(ed25519.PublicKey)

	verifiedPack, err := pack.VerifyPackIntegrity(packDir, pubKey)
	if err != nil {
		t.Fatalf("Immune membrane failed to verify packs/code-eval: %v", err)
	}
	if verifiedPack.Name != "code-eval" {
		t.Fatalf("expected pack name code-eval, got %s", verifiedPack.Name)
	}

	// 2. Ingest through Metabolism Engine
	reg := metabolism.NewReceptorRegistry()
	engine := metabolism.NewMetabolismEngine(reg)
	outDir := filepath.Join(t.TempDir(), "run-exhaust")

	baselineRequired := 4.5
	digest, err := engine.Ingest(metabolism.IngestionOptions{
		PackDir:    packDir,
		PublicKey:  pubKey,
		OutputDir:  outDir,
		VerifySeal: true,
		Parameters: map[string]interface{}{
			"baseline":   baselineRequired,
			"compliance": "SOC2,OWASP",
		},
	})
	if err != nil {
		t.Fatalf("Metabolism engine ingestion failed: %v", err)
	}

	// Assert 25 Prompt Templates
	if len(digest.PromptTemplates) != 25 {
		t.Fatalf("expected 25 prompt templates, got %d", len(digest.PromptTemplates))
	}

	// 3. Verify Ontological Graph Rigor & Shovel-Ready Gate
	var goalObj, mlsObj, priObj, cvsObj map[string]any
	var reqList, critList, bliList, tstList []map[string]any

	for _, o := range digest.KernelObjects {
		kind, _ := o[objects.FieldKeyKind].(string)
		switch kind {
		case "goal":
			goalObj = o
		case "milestone":
			mlsObj = o
		case "priority_plan":
			priObj = o
		case "convergence_session":
			cvsObj = o
		case "requirement":
			reqList = append(reqList, o)
		case "criteria":
			critList = append(critList, o)
		case "backlog_item":
			bliList = append(bliList, o)
		case "test_case":
			tstList = append(tstList, o)
		}
	}

	if goalObj == nil || mlsObj == nil || priObj == nil || cvsObj == nil {
		t.Fatalf("missing spine objects: goal=%v mls=%v pri=%v cvs=%v", goalObj, mlsObj, priObj, cvsObj)
	}

	// 12 tasks in code-eval swarm.yaml -> 12 reqs, 36 criteria (3-Fold), 12 BLIs, 12 test cases
	if len(reqList) != 12 || len(bliList) != 12 || len(tstList) != 12 {
		t.Fatalf("expected 12 reqs/blis/tests; got reqs=%d blis=%d tests=%d", len(reqList), len(bliList), len(tstList))
	}
	if len(critList) != 36 {
		t.Fatalf("expected 36 criteria (3 per requirement), got %d", len(critList))
	}

	// Verify all synthesized requirements pass the Anti-Superficiality Quality Gate
	for _, req := range reqList {
		reqID, _ := req[objects.FieldKeyID].(string)
		var linked []map[string]any
		for _, c := range critList {
			rRefs, _ := c[objects.FieldKeyRequirementRefs].([]string)
			for _, r := range rRefs {
				if r == reqID {
					linked = append(linked, c)
				}
			}
		}
		if err := workflow.CheckShovelReadyQuality(req, linked); err != nil {
			t.Fatalf("requirement %s failed shovel-ready gate: %v", reqID, err)
		}
	}

	// 4. Simulate Dual-Stream Execution
	// Stream B: Route Findings & Scorecard
	testFinding := metabolism.Finding{
		ID:          "FINDING-CEF-RDB-001",
		Lens:        "L-ARCHITECTURE",
		Severity:    "E1",
		Title:       "Cyclic Package Boundary Dependency Detected",
		Description: "Bidirectional import cycle between storage and cache subsystems.",
		Evidence:    "ast_graph: import cycle detected between pkg/a and pkg/b",
		Remediation: "Introduce interface inversion facade in pkg/facade.",
		Files:       []string{"pkg/storage/store.go", "pkg/cache/cache.go"},
	}

	if err := digest.Router.RouteFinding(testFinding); err != nil {
		t.Fatalf("RouteFinding failed: %v", err)
	}

	testScorecard := metabolism.Scorecard{
		PackURN:     digest.URN.String(),
		SessionID:   digest.Lease.SessionID,
		EnvelopeMin: 4.6, // Exceeds baseline of 4.5
		Scores: map[string]float64{
			"RDB": 4.6,
			"MNT": 4.8,
			"TST": 4.9,
			"REL": 4.7,
			"OBS": 4.6,
			"RCV": 4.8,
			"SEC": 4.7,
			"ROB": 4.8,
		},
		Status: "converged",
	}

	if err := digest.Router.RouteScorecard(testScorecard); err != nil {
		t.Fatalf("RouteScorecard failed: %v", err)
	}

	// Stream A: Synthesize actionable Remediation BLI for E1 finding
	mlsID, _ := mlsObj[objects.FieldKeyID].(string)
	remediationBundle, err := FindingToRemediationBLI(testFinding, mlsID)
	if err != nil {
		t.Fatalf("FindingToRemediationBLI failed: %v", err)
	}

	if remediationBundle.BacklogItem[objects.FieldKeyPriority] != "p1" {
		t.Errorf("expected p1 for E1 finding, got %v", remediationBundle.BacklogItem[objects.FieldKeyPriority])
	}

	// 5. Convergence Homeostasis Verification
	if testScorecard.EnvelopeMin < baselineRequired {
		t.Fatalf("scorecard %f does not achieve baseline %f", testScorecard.EnvelopeMin, baselineRequired)
	}

	// Complete ingestion lifecycle
	if err := engine.CompleteIngestion(digest, "converged"); err != nil {
		t.Fatalf("CompleteIngestion failed: %v", err)
	}

	// Verify receptor lineage recorded completed run
	lineage, ok := reg.GetLineage(digest.URN.String())
	if !ok || lineage.State != metabolism.StateClosed {
		t.Fatalf("expected receptor state closed, got %v", lineage.State)
	}
	if len(lineage.History) != 1 || lineage.History[0].Outcome != "converged" {
		t.Errorf("expected 1 converged history entry, got %+v", lineage.History)
	}
}
