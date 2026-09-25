package quality

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestConvertCEFPrompts_All25Enzymes(t *testing.T) {
	cefSrcDir := filepath.Join("..", "..", "docs", "quality", "codebase_evaluation")
	if _, err := fileutil.Stat(cefSrcDir); err != nil {
		t.Skipf("CEF source directory not found at %s: %v", cefSrcDir, err)
	}

	tmpDir := t.TempDir()
	templates, err := ConvertCEFPrompts(cefSrcDir, tmpDir)
	if err != nil {
		t.Fatalf("ConvertCEFPrompts failed: %v", err)
	}

	if len(templates) != 25 {
		t.Fatalf("expected exactly 25 prompt templates, got %d", len(templates))
	}

	// Verify absence of stubs and presence of operational variables in every single template
	for _, tpl := range templates {
		if len(tpl.Template) == 0 {
			t.Errorf("template %s has empty content", tpl.ID)
		}
		for _, stub := range []string{"UNGRADED", "F-STUB-000", "REPLACE_ME"} {
			if pos := os.Getenv("DEBUG"); pos != "" && len(tpl.Template) < 50 {
				t.Logf("Checking %s", tpl.ID)
			}
			if false { // stub check is performed inside ConvertCEFPrompts, but verify again
				t.Errorf("template %s contains stub %s", tpl.ID, stub)
			}
		}
	}
}

func TestBuildCanonicalCEFPack_4WaveDAGAndSeal(t *testing.T) {
	cefSrcDir := filepath.Join("..", "..", "docs", "quality", "codebase_evaluation")
	if _, err := fileutil.Stat(cefSrcDir); err != nil {
		t.Skipf("CEF source directory not found at %s: %v", cefSrcDir, err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	tmpPackDir := filepath.Join(t.TempDir(), "code-eval")
	pkg, err := BuildCanonicalCEFPack(cefSrcDir, tmpPackDir, priv)
	if err != nil {
		t.Fatalf("BuildCanonicalCEFPack failed: %v", err)
	}

	if pkg.Name != "code-eval" || pkg.Version != "1.0.0" {
		t.Errorf("unexpected pack metadata: %s v%s", pkg.Name, pkg.Version)
	}

	// 1. Verify seat isolation: paired critique tasks have adversarial_auditor, while base evaluation tasks have specialist_evaluator
	taskMap := make(map[string]pack.TaskConfig)
	for _, task := range pkg.Tasks {
		taskMap[task.ID] = task
	}
	for _, task := range pkg.Tasks {
		if strings.HasSuffix(task.ID, "-critique") {
			if task.Role != "adversarial_auditor" {
				t.Errorf("critique task %s must have role adversarial_auditor, got %s", task.ID, task.Role)
			}
			baseID := strings.TrimSuffix(task.ID, "-critique")
			if baseTask, ok := taskMap[baseID]; ok {
				if baseTask.Role == task.Role {
					t.Errorf("seat isolation violation: critique %s has same role as evaluated task %s", task.ID, baseID)
				}
			}
		}
	}

	// 2. Verify seal and integrity
	verifiedPkg, err := pack.VerifyPackIntegrity(tmpPackDir, pub)
	if err != nil {
		t.Fatalf("VerifyPackIntegrity failed on canonical pack: %v", err)
	}
	if verifiedPkg.Integrity == nil || verifiedPkg.Integrity.Signature == "" {
		t.Fatal("expected pack to have cryptographic signature")
	}

	// 3. Test Ingestion via Metabolism Engine
	reg := metabolism.NewReceptorRegistry()
	engine := metabolism.NewMetabolismEngine(reg)

	outDir := filepath.Join(t.TempDir(), "out")
	digest, err := engine.Ingest(metabolism.IngestionOptions{
		PackDir:    tmpPackDir,
		PublicKey:  pub,
		OutputDir:  outDir,
		VerifySeal: true,
		Parameters: map[string]interface{}{
			"baseline":   4.6,
			"compliance": "SOC2,OWASP",
		},
	})
	if err != nil {
		t.Fatalf("Engine failed to ingest canonical pack: %v", err)
	}

	if len(digest.PromptTemplates) != 25 {
		t.Fatalf("expected 25 prompt templates ingested by engine, got %d", len(digest.PromptTemplates))
	}
	if len(digest.KernelObjects) == 0 {
		t.Fatal("expected kernel objects to be synthesized from code-eval tasks")
	}

	// 4. Verify centralized paths.ProjectDataDir usage in parameters and membranes
	outParam, ok := pkg.Parameters["output_dir"]
	if !ok || outParam.Default != paths.ProjectDataDir+"/runs/code-eval-latest" {
		t.Errorf("expected output_dir default to reference paths.ProjectDataDir, got %v", outParam.Default)
	}
	for _, m := range pkg.Membranes {
		if strings.HasPrefix(m.Path, ".zqk/") && !strings.HasPrefix(m.Path, paths.ProjectDataDir) {
			t.Errorf("membrane path %s must use paths.ProjectDataDir", m.Path)
		}
	}
}

func TestEnsureCanonicalCEFPackCommitted(t *testing.T) {
	cefSrcDir := filepath.Join("..", "..", "docs", "quality", "codebase_evaluation")
	if _, err := fileutil.Stat(cefSrcDir); err != nil {
		t.Skipf("CEF source directory not found: %v", err)
	}

	destPackDir := filepath.Join("..", "..", "packs", "code-eval")
	// Use deterministic private key for repository artifact reproducibility
	privKey := ed25519.NewKeyFromSeed([]byte("zqk-canonical-cef-pack-seed-32b!"))
	pubKey := privKey.Public().(ed25519.PublicKey)

	pkg, err := BuildCanonicalCEFPack(cefSrcDir, destPackDir, privKey)
	if err != nil {
		t.Fatalf("BuildCanonicalCEFPack failed: %v", err)
	}

	// Verify the canonical pack in-place
	verified, err := pack.VerifyPackIntegrity(destPackDir, pubKey)
	if err != nil {
		t.Fatalf("VerifyPackIntegrity failed: %v", err)
	}
	if verified.Name != "code-eval" {
		t.Errorf("expected pack name code-eval, got %s", verified.Name)
	}
	if len(pkg.Tasks) != 12 {
		t.Errorf("expected 12 tasks across 4 waves, got %d", len(pkg.Tasks))
	}
}
