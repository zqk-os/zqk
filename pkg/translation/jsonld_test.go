package translation

import (
	"testing"
)

const (
	domainIDOrganization    = "organization"
	domainIDDivision        = "division"
	labelOrganization       = "Organization"
	labelDivision           = "Division"
	descriptionOrganization = "An organizational entity"
)

const sampleJSONLD = `{
  "@context": {
    "rdf": "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
    "rdfs": "http://www.w3.org/2000/01/rdf-schema#",
    "owl": "http://www.w3.org/2002/07/owl#",
    "org": "http://example.com/org#"
  },
  "@graph": [
    {
      "@id": "org:Organization",
      "@type": "owl:Class",
      "rdfs:label": "Organization",
      "rdfs:comment": "An organizational entity"
    },
    {
      "@id": "org:Division",
      "@type": "owl:Class",
      "rdfs:label": "Division",
      "rdfs:comment": "A division within an organization",
      "rdfs:subClassOf": { "@id": "org:Organization" }
    }
  ]
}
`

func TestParseJSONLDClasses(t *testing.T) {
	t.Parallel()
	classes, err := parseJSONLDClasses([]byte(sampleJSONLD))
	if err != nil {
		t.Fatalf("parseJSONLDClasses: %v", err)
	}
	if len(classes) != 2 {
		t.Fatalf("expected 2 classes, got %d", len(classes))
	}
	byName := make(map[string]TurtleClass)
	for _, c := range classes {
		byName[c.LocalName] = c
	}
	if c := byName[domainIDOrganization]; c.Label != labelOrganization {
		t.Errorf("organization label = %q, want Organization", c.Label)
	} else if c.Comment != descriptionOrganization {
		t.Errorf("organization comment = %q", c.Comment)
	}
	if c := byName[domainIDDivision]; c.Label != labelDivision {
		t.Errorf("division label = %q", c.Label)
	} else if c.SubClassOf != domainIDOrganization {
		t.Errorf("division sub_class_of = %q, want organization", c.SubClassOf)
	}
}

func TestRDFOWLTranslator_Translate_JSONLD_ProducesDomainRegistry(t *testing.T) {
	t.Parallel()
	tr := NewRDFOWLTranslator()
	result, err := tr.Translate([]byte(sampleJSONLD), nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if len(result.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(result.Objects))
	}
	obj := result.Objects[0]
	if obj.Kind != DomainRegistryKind {
		t.Errorf("kind = %q, want domain_registry", obj.Kind)
	}
	domainsSlice, _ := obj.Data[KeyDomains].([]map[string]any)
	if len(domainsSlice) != 2 {
		t.Errorf("domains length = %d, want 2", len(domainsSlice))
	}
	// Object-spec-like output.
	if len(result.Specs) != 2 {
		t.Errorf("expected 2 specs, got %d", len(result.Specs))
	}
}

func TestParseJSONLDClasses_Empty(t *testing.T) {
	t.Parallel()
	classes, err := parseJSONLDClasses([]byte("{}"))
	if err != nil {
		t.Fatalf("parseJSONLDClasses: %v", err)
	}
	if len(classes) != 0 {
		t.Errorf("expected 0 classes, got %d", len(classes))
	}
}

func TestLocalNameFromJSONLDID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id   string
		want string
	}{
		{"org:Organization", "organization"},
		{"http://example.com/org#Division", "division"},
		{"http://example.com/ns/Thing", "thing"},
	}
	for _, tt := range tests {
		got := localNameFromJSONLDID(tt.id)
		if got != tt.want {
			t.Errorf("localNameFromJSONLDID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}
