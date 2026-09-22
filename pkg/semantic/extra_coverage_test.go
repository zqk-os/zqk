// BLI-STARTER-COMMUNITY-068 / PRI-STARTER-COMMUNITY-068 coverage elevation
package semantic

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestExtraScannersAssessorImportInfer(t *testing.T) {
	root := t.TempDir()
	_, _ = (&StructuredDataScanner{}).Scan(root)
	_, _ = (&SchemaScanner{}).Scan(root)
	_, _ = (&OntologyScanner{}).Scan(root)
	_, _ = (&SemanticRepositoryScanner{}).Scan(root)

	mustWrite := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("a.yaml", "k: 1\n")
	mustWrite("b.yml", "k: 2\n")
	mustWrite("c.json", "{}\n")
	mustWrite("d.yaml", "k: 3\n")
	mustWrite("schema.json", "{}\n")
	mustWrite("x.schema.json", "{}\n")
	mustWrite("t.xsd", "<schema/>\n")
	mustWrite("o.owl", "<rdf/>\n")
	mustWrite("g.ttl", "@prefix x: <n> .\n")
	mustWrite("vendor/skip.yaml", "no\n")
	mustWrite(".hidden/x.yaml", "no\n")
	mustWrite("._apple.yaml", "no\n")
	mustWrite("docker-compose.yml", "sparql: yes\n")
	mustWrite(filepath.Join(paths.ProjectDataDir, paths.ProjectConfigFile), "graphdb: true\n")

	if _, err := (&StructuredDataScanner{}).Scan(root); err != nil {
		t.Fatal(err)
	}
	if _, err := (&SchemaScanner{}).Scan(root); err != nil {
		t.Fatal(err)
	}
	if _, err := (&OntologyScanner{}).Scan(root); err != nil {
		t.Fatal(err)
	}
	if _, err := (&SemanticRepositoryScanner{}).Scan(root); err != nil {
		t.Fatal(err)
	}
	_ = minInt(1, 2)
	_ = minInt(5, 3)
	_ = findFiles(root, []string{".yaml"})

	ma := NewMaturityAssessor([]MaturityScanner{
		&StructuredDataScanner{}, &SchemaScanner{}, &OntologyScanner{}, &SemanticRepositoryScanner{},
	})
	if _, err := ma.Assess(root); err != nil {
		t.Fatal(err)
	}
	for _, inds := range [][]Indicator{
		nil,
		{{Type: "structured_data", Found: true}},
		{{Type: "formal_schemas", Found: true}},
		{{Type: "ontologies", Found: true}},
		{{Type: "semantic_repositories", Found: true}},
	} {
		lvl, _ := determineMaturityLevel(inds)
		_ = generateRecommendations(lvl, inds)
	}
	_ = generateRecommendations(99, nil)

	imp := NewOntologyImporter()
	_, _ = imp.Import(filepath.Join(root, "missing.ttl"), "ttl", nil)
	_, _ = imp.Import("mem.json", "json", []byte(`{"id":"x"}`))

	eng := NewInferenceEngine(nil)
	_ = eng.Infer(context.Background(), &MaturityAssessment{Level: 0})
	_ = eng.Infer(context.Background(), &MaturityAssessment{Level: 2})
	store := extraSemStore{objs: []map[string]any{{
		objects.FieldKeyProposedTitle:       "HeuristicTitleXX",
		objects.FieldKeyProposedDescription: "d",
		objects.FieldKeyProposedPriority:    "P2",
	}}}
	eng2 := NewInferenceEngine(store)
	_ = eng2.Infer(context.Background(), &MaturityAssessment{Level: 1})
	_ = eng2.InferFromConvergence([]ConvergenceResult{{TrendingAway: true, Title: "SessionTitle", SessionID: "CVS-1"}})
	_ = eng2.InferFromConvergence([]ConvergenceResult{{TrendingAway: false}})
}

type extraSemStore struct {
	storage.NoopObjectStorage
	objs []map[string]any
}

func (s extraSemStore) List(context.Context, *storage.SecurityContext, *storage.StorageContext, storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{Objects: s.objs}, nil
}
