package outputtypes

import "testing"

const (
	testMIMENDJSON = "application/x-ndjson; charset=utf-8"
	testExtJSONL   = ".jsonl"
	testAliasNDJS  = "NDJSON"
	testAliasYML   = "yml"
)

func TestGet_JSONLDefinition(t *testing.T) {
	t.Parallel()
	def, ok := Get(IDJSONL)
	if !ok {
		t.Fatalf("expected %q definition to exist", IDJSONL)
	}
	if def.MIMEType != testMIMENDJSON {
		t.Fatalf("unexpected MIME type: %q", def.MIMEType)
	}
	if def.FileExtension != testExtJSONL {
		t.Fatalf("unexpected extension: %q", def.FileExtension)
	}
	if !def.LineDelimited {
		t.Fatal("expected jsonl to be line-delimited")
	}
}

func TestNormalize_Aliases(t *testing.T) {
	t.Parallel()
	if got := Normalize(testAliasNDJS); got != IDJSONL {
		t.Fatalf("Normalize(NDJSON)=%q, want %q", got, IDJSONL)
	}
	if got := Normalize(testAliasYML); got != IDYAML {
		t.Fatalf("Normalize(yml)=%q, want %q", got, IDYAML)
	}
}
