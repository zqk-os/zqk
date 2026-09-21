package authcred

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestVocabularySchemesForPersona(t *testing.T) {
	root := t.TempDir()
	dir := paths.PersonasDirPath(root)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	const personaID = "PER-VOCAB-001"
	const hash = "aaaabbbbccccdddd"
	body := "id: " + personaID + "\nkind: persona\nvocabulary_scheme_refs:\n  - VS-ONE\n"
	if err := fileutil.WriteStandardFile(paths.PersonaYAMLPath(root, hash), []byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.PersonaIndexPath(root), []byte(`{"mappings":{"`+personaID+`":"`+hash+`"}}`)); err != nil {
		t.Fatal(err)
	}

	got := VocabularySchemesForPersona(root, personaID)
	if len(got) != 1 || got[0] != "VS-ONE" {
		t.Fatalf("first load=%v", got)
	}
	again := VocabularySchemesForPersona(root, personaID)
	if len(again) != 1 || again[0] != "VS-ONE" {
		t.Fatalf("repeat load=%v", again)
	}

	updated := "id: " + personaID + "\nkind: persona\nvocabulary_scheme_refs:\n  - VS-TWO\n"
	if err := fileutil.WriteStandardFile(paths.PersonaYAMLPath(root, hash), []byte(updated)); err != nil {
		t.Fatal(err)
	}
	got = VocabularySchemesForPersona(root, personaID)
	if len(got) != 1 || got[0] != "VS-TWO" {
		t.Fatalf("after yaml change=%v", got)
	}
}

func TestVocabularySchemesForPersona_honorPathCache(t *testing.T) {
	root := t.TempDir()
	customDir := filepath.Join("custom", "personas")
	paths.ReplacePathCache(root, map[string]string{
		paths.PathAliasPersonas:     customDir,
		paths.PathAliasPersonaIndex: filepath.Join(customDir, paths.PersonaIndexFile),
	})
	if err := fileutil.EnsureDir(paths.PersonasDirPath(root)); err != nil {
		t.Fatal(err)
	}
	const personaID = "PER-CACHED-DIR"
	const hash = "hashpersona001"
	if err := fileutil.WriteStandardFile(paths.PersonaYAMLPath(root, hash), []byte("vocabulary_scheme_refs: [VS-CACHE]\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.PersonaIndexPath(root), []byte(`{"mappings":{"`+personaID+`":"`+hash+`"}}`)); err != nil {
		t.Fatal(err)
	}
	got := VocabularySchemesForPersona(root, personaID)
	if len(got) != 1 || got[0] != "VS-CACHE" {
		t.Fatalf("path-cache lookup=%v", got)
	}
}

func TestLookupBoundAccount(t *testing.T) {
	root := t.TempDir()
	const accID = "ACC-BOUND-001"
	const hash = "accfixturehash01"
	if err := fileutil.EnsureDir(paths.AccountsDirPath(root)); err != nil {
		t.Fatal(err)
	}
	body := "id: " + accID + "\nkind: account\nroles:\n  - coder_agent\npersona_ref: PER-TEST-001\n"
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(root, hash), []byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(root), []byte(`{"mappings":{"`+accID+`":"`+hash+`"}}`)); err != nil {
		t.Fatal(err)
	}

	got, err := LookupBoundAccount(root, accID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PersonaRef != "PER-TEST-001" || len(got.Roles) != 1 || got.Roles[0] != "coder_agent" {
		t.Fatalf("bound=%+v", got)
	}
	again, err := LookupBoundAccount(root, accID)
	if err != nil || again.PersonaRef != "PER-TEST-001" {
		t.Fatalf("repeat bound=%+v err=%v", again, err)
	}
	if _, err := LookupBoundAccount(root, "ACC-MISSING"); err == nil {
		t.Fatal("expected missing account")
	}
}

func TestLoadRoleRecords_usesIndexWithoutDirScan(t *testing.T) {
	root := t.TempDir()
	const roleID = "ROL-CACHE-001"
	const hash = "rolehashfixture1"
	if err := fileutil.EnsureDir(paths.RolesDirPath(root)); err != nil {
		t.Fatal(err)
	}
	body := "id: " + roleID + "\nrole_id: coder_agent\nstatus: active\npermissions:\n  - read:*\n"
	if err := fileutil.WriteStandardFile(paths.RoleYAMLPath(root, hash), []byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.RoleIndexPath(root), []byte(`{"mappings":{"`+roleID+`":"`+hash+`"}}`)); err != nil {
		t.Fatal(err)
	}

	dir := NewDiskSeatDirectory(root)
	first := dir.Roles()
	if len(first) != 1 || first[0].ID != roleID {
		t.Fatalf("first roles=%+v", first)
	}
	second := dir.Roles()
	if len(second) != 1 || second[0].ID != roleID {
		t.Fatalf("cached roles=%+v", second)
	}
	perms := PermissionsForAssignedRoles(dir, []string{"coder_agent"})
	if len(perms) != 1 || perms[0] != "read:*" {
		t.Fatalf("perms=%v", perms)
	}
}

func TestListAuthStrategyRecords(t *testing.T) {
	root := t.TempDir()
	dir := paths.AuthStrategiesDirPath(root)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	body := "id: AST-CACHE-001\nstrategy_type: api_key\nenabled: true\nstatus: active\n"
	if err := fileutil.WriteStandardFile(filepath.Join(dir, "ast.yaml"), []byte(body)); err != nil {
		t.Fatal(err)
	}
	first := ListAuthStrategyRecords(root)
	if len(first) != 1 || first[0].ID != "AST-CACHE-001" || first[0].Type != "api_key" {
		t.Fatalf("first=%+v", first)
	}
	second := ListAuthStrategyRecords(root)
	if len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("cached=%+v", second)
	}
}
