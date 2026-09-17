package system

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestEmbeddedDefaultAgentSeatingTemplates(t *testing.T) {
	personas := embeddedDefaultPersonaTemplates()
	if len(personas) < 2 {
		t.Fatalf("want >=2 personas, got %d", len(personas))
	}
	ids := map[string]bool{}
	for _, p := range personas {
		id := objects.GetString(p, objects.FieldKeyID)
		if id == "" || objects.GetString(p, objects.FieldKeyRole) == "" {
			t.Fatalf("persona missing id/role: %v", p)
		}
		ids[id] = true
	}
	for _, want := range []string{"PER-DEFAULT-OPERATOR", "PER-DEFAULT-AGENT"} {
		if !ids[want] {
			t.Fatalf("missing persona %s", want)
		}
	}

	skills := embeddedDefaultAgentSkillTemplates()
	if len(skills) < 1 {
		t.Fatal("want >=1 skill")
	}
	if objects.GetString(skills[0], objects.FieldKeyID) != "ASK-DEFAULT-FEED-CORRESPONDENCE" {
		t.Fatalf("skill id=%q", objects.GetString(skills[0], objects.FieldKeyID))
	}
	if skills[0][objects.FieldKeyProvider] != "zqk" {
		t.Fatalf("provider=%v", skills[0][objects.FieldKeyProvider])
	}
}

func TestSeedKernelFromAnswerFile(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "system.seed_kernel_answer_file",
		SeedSchemaPlane: true,
	})
	root := proj.Root
	answerFile := root + "/answer_seed.yaml"
	content := `
- kind: persona
  id: PER-SWARM-LEAD
  name: Swarm Lead
  title: Swarm Lead
  role: agent
  status: proposed
  schema_version: "` + objects.DefaultSchemaVersion + `"
  created_at: "2026-01-02T00:00:00Z"
  created_by: ACC-1785920548450214012-68b850c0
  updated_at: "2026-01-02T00:00:00Z"
  updated_by: ACC-1785920548450214012-68b850c0
  namespace_id: zqk:kernel
- kind: policy
  id: POL-CODE-901
  title: Swarm Safety Policy
  status: draft
  policy_type: requirement
  category: security
  body: "Swarm safety rules for kernel seed tests."
  schema_version: "` + objects.DefaultSchemaVersion + `"
  enforcement_level: required
  namespace_id: zqk:kernel
  created_at: "2026-01-02T00:00:00Z"
  created_by: ACC-1785920548450214012-68b850c0
  updated_at: "2026-01-02T00:00:00Z"
  updated_by: ACC-1785920548450214012-68b850c0
  origin_project: zqk
  origin_system: zqk
`
	if err := fileutil.WriteFile(answerFile, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	created, err := SeedKernelFromAnswerFile(root, answerFile, nil)
	if err != nil {
		t.Fatalf("SeedKernelFromAnswerFile: %v", err)
	}
	if created < 1 {
		t.Fatalf("want at least 1 object created, got %d", created)
	}
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), root)
	if err != nil {
		t.Fatalf("storage factory: %v", err)
	}
	sp := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()
	for _, id := range []string{"PER-SWARM-LEAD", "POL-CODE-901"} {
		if _, err := sp.Read(pkgctx.NewSystemContext(), secCtx, id); err != nil {
			t.Fatalf("seeded object %s not readable: %v", id, err)
		}
	}
}
