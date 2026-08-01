package translation

import (
	"testing"
)

const (
	sampleUnknownData = "x"
	rdfOwlPrefixLine  = "@prefix ex: <http://example.org/> ."
	unknownFormatName = "unknown_format"
)

func TestRegistry_GetAndTranslate(t *testing.T) {
	t.Parallel()
	// CRIT-7642: at least one format translator implemented and testable.
	t.Run("rdf_owl", func(t *testing.T) {
		tr := Get(FormatRDFOWL)
		if tr == nil {
			t.Fatal("Get(\"rdf_owl\") should return a translator")
		}
		if tr.SourceFormat() != FormatRDFOWL {
			t.Errorf("SourceFormat() = %q, want rdf_owl", tr.SourceFormat())
		}
		result, err := tr.Translate([]byte(rdfOwlPrefixLine), nil)
		if err != nil {
			t.Fatalf("Translate: %v", err)
		}
		if result == nil {
			t.Fatal("Translate result should be non-nil")
		}
		if result.Objects == nil {
			t.Error("result.Objects should be non-nil (may be empty)")
		}
	})
	t.Run("turtle", func(t *testing.T) {
		tr := Get(FormatTurtle)
		if tr == nil {
			t.Fatal("Get(\"turtle\") should return a translator")
		}
		if tr.SourceFormat() != FormatTurtle {
			t.Errorf("SourceFormat() = %q, want turtle", tr.SourceFormat())
		}
	})
	t.Run("cypher", func(t *testing.T) {
		tr := Get(FormatCypher)
		if tr == nil {
			t.Fatal("Get(\"cypher\") should return a translator")
		}
		result, err := tr.Translate([]byte("CREATE CONSTRAINT IF NOT EXISTS FOR (o:Organization) REQUIRE o.id IS UNIQUE;"), nil)
		if err != nil {
			t.Fatalf("Translate: %v", err)
		}
		if result == nil || len(result.Objects) == 0 {
			t.Fatal("Translate should produce at least one object (domain_registry)")
		}
		if len(result.Specs) == 0 {
			t.Error("Translate should produce at least one spec")
		}
	})
	t.Run("json_schema", func(t *testing.T) {
		tr := Get(FormatJSONSchema)
		if tr == nil {
			t.Fatal("Get(\"json_schema\") should return a translator")
		}
		raw := []byte(`{"$schema":"http://json-schema.org/draft-07/schema#","definitions":{"Organization":{"type":"object","title":"Org"},"Division":{"type":"object"}}}`)
		result, err := tr.Translate(raw, nil)
		if err != nil {
			t.Fatalf("Translate: %v", err)
		}
		if result == nil || len(result.Objects) == 0 {
			t.Fatal("Translate should produce domain_registry")
		}
		if len(result.Specs) < 2 {
			t.Errorf("expected at least 2 specs, got %d", len(result.Specs))
		}
	})
	t.Run("openapi", func(t *testing.T) {
		tr := Get(FormatOpenAPI)
		if tr == nil {
			t.Fatal("Get(\"openapi\") should return a translator")
		}
		raw := []byte("openapi: 3.0.0\ncomponents:\n  schemas:\n    Organization:\n      type: object\n      title: Org\n    Division:\n      type: object\n")
		result, err := tr.Translate(raw, nil)
		if err != nil {
			t.Fatalf("Translate: %v", err)
		}
		if result == nil || len(result.Objects) == 0 {
			t.Fatal("Translate should produce domain_registry")
		}
		if len(result.Specs) < 2 {
			t.Errorf("expected at least 2 specs, got %d", len(result.Specs))
		}
	})
	t.Run("unknown_format", func(t *testing.T) {
		tr := Get(FormatUnknown)
		if tr != nil {
			t.Errorf("Get(%q) should return nil", unknownFormatName)
		}
		_, err := TranslateByFormat(FormatUnknown, []byte(sampleUnknownData), nil)
		if err == nil {
			t.Fatal("TranslateByFormat(unknown) should return error")
		}
		if _, ok := err.(ErrNoTranslator); !ok {
			t.Errorf("expected ErrNoTranslator, got %T", err)
		}
	})
}
