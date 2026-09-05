package translation

import (
	"testing"
)

const (
	rdfXMLOrganizationID    = "organization"
	rdfXMLDivisionID        = "division"
	rdfXMLOrganizationLabel = "Organization"
	rdfXMLDivisionLabel     = "Division"
)

const sampleRDFXML = `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <owl:Class rdf:about="http://example.com/org#Organization">
    <rdfs:label>Organization</rdfs:label>
    <rdfs:comment>An organizational entity</rdfs:comment>
  </owl:Class>
  <owl:Class rdf:about="http://example.com/org#Division">
    <rdfs:label>Division</rdfs:label>
    <rdfs:comment>A division within an organization</rdfs:comment>
    <rdfs:subClassOf rdf:resource="http://example.com/org#Organization"/>
  </owl:Class>
</rdf:RDF>`

func TestParseRDFXMLClasses(t *testing.T) {
	t.Parallel()
	classes := parseRDFXMLClasses([]byte(sampleRDFXML))
	if len(classes) != 2 {
		t.Fatalf("expected 2 classes, got %d", len(classes))
	}
	byName := make(map[string]TurtleClass)
	for _, c := range classes {
		byName[c.LocalName] = c
	}
	if c := byName[rdfXMLOrganizationID]; c.Label != rdfXMLOrganizationLabel {
		t.Errorf("organization label = %q, want Organization", c.Label)
	} else if c.Comment != "An organizational entity" {
		t.Errorf("organization comment = %q", c.Comment)
	}
	if c := byName[rdfXMLDivisionID]; c.Label != rdfXMLDivisionLabel {
		t.Errorf("division label = %q", c.Label)
	} else if c.SubClassOf != rdfXMLOrganizationID {
		t.Errorf("division sub_class_of = %q, want organization", c.SubClassOf)
	}
}

func TestRDFOWLTranslator_Translate_RDFXML_ProducesDomainRegistry(t *testing.T) {
	t.Parallel()
	tr := NewRDFOWLTranslator()
	result, err := tr.Translate([]byte(sampleRDFXML), nil)
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
}

func TestLocalNameFromRDFAbout(t *testing.T) {
	t.Parallel()
	tests := []struct{ uri, want string }{
		{"http://example.com/org#Organization", "organization"},
		{"http://example.org/Division", "division"},
		{"https://schema.org/Person", "person"},
	}
	for _, tt := range tests {
		got := localNameFromRDFAbout(tt.uri)
		if got != tt.want {
			t.Errorf("localNameFromRDFAbout(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
