package authcred

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestVocabularySchemesForPersona_cachedUntilMtimeChanges(t *testing.T) {
	root := t.TempDir()
	dir := paths.PersonasDirPath(root)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	const personaID = "PER-VOCAB-001"
	const hash = "aaaabbbbccccdddd"
	body := "id: " + personaID + "\nkind: persona\nvocabulary_scheme_refs:\n  - VS-ONE\n  - VS-TWO\n"
	if err := fileutil.WriteStandardFile(paths.PersonaYAMLPath(root, hash), []byte(body)); err != nil {
		t.Fatal(err)
	}
	idx := `{"mappings":{"` + personaID + `":"` + hash + `"}}`
	if err := fileutil.WriteStandardFile(paths.PersonaIndexPath(root), []byte(idx)); err != nil {
		t.Fatal(err)
	}

	got := VocabularySchemesForPersona(root, personaID)
	if len(got) != 2 || got[0] != "VS-ONE" || got[1] != "VS-TWO" {
		t.Fatalf("first load=%v", got)
	}
	again := VocabularySchemesForPersona(root, personaID)
	if len(again) != 2 || again[0] != "VS-ONE" {
		t.Fatalf("repeat load=%v", again)
	}

	updated := "id: " + personaID + "\nkind: persona\nvocabulary_scheme_refs:\n  - VS-THREE\n"
	if err := fileutil.WriteStandardFile(paths.PersonaYAMLPath(root, hash), []byte(updated)); err != nil {
		t.Fatal(err)
	}
	got = VocabularySchemesForPersona(root, personaID)
	if len(got) != 1 || got[0] != "VS-THREE" {
		t.Fatalf("after yaml change=%v", got)
	}
}

func TestVocabularySchemesForPersona_missing(t *testing.T) {
	if VocabularySchemesForPersona(t.TempDir(), "PER-NONE") != nil {
		t.Fatal("expected nil for missing persona")
	}
	if VocabularySchemesForPersona("", "PER-NONE") != nil {
		t.Fatal("expected nil for empty root")
	}
}
