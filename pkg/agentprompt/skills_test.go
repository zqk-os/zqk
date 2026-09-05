package agentprompt

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/skill"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestGeneratePromptSection_refsNotBodies(t *testing.T) {
	s := &SkillEnforcement{RelevantSkills: []map[string]any{
		{
			objects.FieldKeyID:                  "ASK-test",
			objects.FieldKeyTitle:               "ZQK Expert Operating Protocol",
			objects.FieldKeyInstructionsSummary: "[orchestration-boot] boot",
			objects.FieldKeyInstructions:        "Never edit docs/process YAML by hand.\nUse VDS evaluate.",
			objects.FieldKeyFilePath:            ".zqk/skills/zqk-expert",
		},
	}}
	out := s.GeneratePromptSection()
	if strings.Contains(out, "#### Mandates") {
		t.Fatalf("default section must not inline mandates:\n%s", out)
	}
	if strings.Contains(out, "Never edit docs/process") {
		t.Fatalf("default section must not inline instruction bodies:\n%s", out)
	}
	if !strings.Contains(out, "ASK-test") || !strings.Contains(out, "[orchestration-boot]") {
		t.Fatalf("missing ref/summary:\n%s", out)
	}
	bodies := s.GeneratePromptSectionOpts(0)
	if !strings.Contains(bodies, "Never edit docs/process") {
		t.Fatalf("bodies helper should still inline:\n%s", bodies)
	}
}

func TestSkillMatchesQuery_titleTokens(t *testing.T) {
	if !skillMatchesQuery("run scheduler scan-tests bundle", "Scheduler Expert Protocol", "", "") {
		t.Fatal("expected scheduler token match")
	}
	if skillMatchesQuery("unrelated gardening", "Scheduler Expert Protocol", "", "") {
		t.Fatal("expected no match")
	}
}

func TestRelatedASKRefs(t *testing.T) {
	persona := map[string]any{
		objects.FieldKeyRelatedObjectRefs: []any{
			"ASK-1",
			"POL-AGENT-001",
			"ASK-2",
		},
	}
	got := relatedASKRefs(persona)
	if len(got) != 2 || got[0] != "ASK-1" || got[1] != "ASK-2" {
		t.Fatalf("got %#v", got)
	}
}

type testMockSP struct {
	storage.ObjectStorageProvider
	objects map[string]map[string]any
	created []map[string]any
}

func (m *testMockSP) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var objs []map[string]any
	for _, obj := range m.objects {
		if kind, ok := obj[objects.FieldKeyKind].(string); ok && kind == filter.Kind {
			objs = append(objs, obj)
		}
	}
	return &storage.QueryResult{Objects: objs}, nil
}

func (m *testMockSP) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.created = append(m.created, obj)
	if id, ok := obj[objects.FieldKeyID].(string); ok && id != "" {
		m.objects[id] = obj
	} else {
		// simulate ID generation
		obj[objects.FieldKeyID] = "ASK-new"
		m.objects["ASK-new"] = obj
	}
	return nil
}

func (m *testMockSP) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (m *testMockSP) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, obj map[string]any) error {
	m.objects[id] = obj
	return nil
}

func TestSyncASKTwins(t *testing.T) {
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	tempDir := t.TempDir()
	skillsDir := filepath.Join(tempDir, ".zqk", "skills")
	fileutil.MkdirAll(skillsDir, 0755)
	fileutil.MkdirAll(filepath.Join(skillsDir, "test-expert"), 0755)

	// Create a dummy .skill zip/file
	fileutil.WriteFile(filepath.Join(skillsDir, "foo.skill"), []byte("dummy"), 0644)

	sp := &testMockSP{
		objects: make(map[string]map[string]any),
	}

	err := SyncASKTwins(ctx, sp, secCtx, tempDir)
	if err != nil {
		t.Fatalf("SyncASKTwins failed: %v", err)
	}

	if len(sp.created) != 2 {
		t.Fatalf("expected 2 created skills, got %d", len(sp.created))
	}
}

func TestLoadRelevantSkillsOpts_EmbedsBootASKs(t *testing.T) {
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	sp := &testMockSP{
		objects: map[string]map[string]any{
			"ASK-BOOT": {
				objects.FieldKeyKind:                objects.KindAgentSkill,
				objects.FieldKeyID:                  "ASK-BOOT",
				objects.FieldKeyInstructionsSummary: "[orchestration-boot] required",
			},
			"PER-123": {
				objects.FieldKeyKind:              objects.KindPersona,
				objects.FieldKeyID:                "PER-123",
				objects.FieldKeyRelatedObjectRefs: []string{"ASK-OTHER"},
			},
		},
	}

	opt := SkillLoadOptions{PersonaID: "PER-123"}
	_, err := LoadRelevantSkillsOpts(ctx, sp, secCtx, opt)
	if err != nil {
		t.Fatalf("LoadRelevantSkillsOpts failed: %v", err)
	}

	// Verify the persona was updated with ASK-BOOT
	updated, _ := sp.Read(ctx, secCtx, "PER-123")
	refs := relatedASKRefs(updated)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}

	found := false
	for _, r := range refs {
		if r == "ASK-BOOT" {
			found = true
		}
	}
	if !found {
		t.Fatal("ASK-BOOT was not embedded into persona")
	}
}

func TestLoadRelevantSkillsOpts_SealVerification(t *testing.T) {
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, ".zqk", "skills", "test-expert")
	fileutil.MkdirAll(skillDir, 0755)

	seal := skill.GenerateSealData("\nbody", "1.0.0", "test", time.Now())
	validContent := "---\n" + seal + "\n---\nbody"

	fileutil.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(validContent), 0644)

	sp := &testMockSP{
		objects: map[string]map[string]any{
			"ASK-VALID": {
				objects.FieldKeyKind:                objects.KindAgentSkill,
				objects.FieldKeyID:                  "ASK-VALID",
				objects.FieldKeyInstructionsSummary: "test valid",
				objects.FieldKeyFilePath:            skillDir,
			},
		},
	}

	opt := SkillLoadOptions{TaskDescription: "test valid"}
	res, err := LoadRelevantSkillsOpts(ctx, sp, secCtx, opt)
	if err != nil {
		t.Fatalf("LoadRelevantSkillsOpts failed: %v", err)
	}
	if len(res.RelevantSkills) != 1 {
		t.Fatalf("expected 1 skill loaded, got %d", len(res.RelevantSkills))
	}

	// Test fail-closed
	invalidContent := "---\nseal_hash: invalid\n---\nbody"
	fileutil.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(invalidContent), 0644)

	_, err = LoadRelevantSkillsOpts(ctx, sp, secCtx, opt)
	if err == nil {
		t.Fatalf("expected error for invalid seal, got nil")
	}
}

func TestLoadRelevantSkillsOpts_MissingFileFailClosed(t *testing.T) {
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	root := t.TempDir()
	sp := &testMockSP{
		objects: map[string]map[string]any{
			"ASK-MISSING": {
				objects.FieldKeyKind:                objects.KindAgentSkill,
				objects.FieldKeyID:                  "ASK-MISSING",
				objects.FieldKeyInstructionsSummary: "missing file",
				objects.FieldKeyFilePath:            filepath.Join(".zqk", "skills", "gone"),
			},
		},
	}

	_, err := LoadRelevantSkillsOpts(ctx, sp, secCtx, SkillLoadOptions{
		TaskDescription: "missing file",
		ProjectRoot:     root,
	})
	if err == nil {
		t.Fatal("expected fail-closed error for missing skill file")
	}
	if !strings.Contains(err.Error(), "cannot read") && !strings.Contains(err.Error(), "seal verification") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Relative path under ProjectRoot succeeds when sealed.
	skillDir := filepath.Join(root, ".zqk", "skills", "gone")
	if err := fileutil.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "\nbody"
	seal := skill.GenerateSealData(body, "1.0.0", "test", time.Now())
	content := "---\n" + seal + "\n---" + body
	if err := fileutil.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadRelevantSkillsOpts(ctx, sp, secCtx, SkillLoadOptions{
		TaskDescription: "missing file",
		ProjectRoot:     root,
	})
	if err != nil {
		t.Fatalf("expected success after creating sealed file: %v", err)
	}
	if len(res.RelevantSkills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(res.RelevantSkills))
	}
}
