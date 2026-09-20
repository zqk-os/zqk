package translation

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	turtleOrganizationID          = "organization"
	turtleDivisionID              = "division"
	turtleOrganizationLabel       = "Organization"
	turtleDivisionLabel           = "Division"
	turtleOrganizationDescription = "An organizational entity"
	turtleDivisionDescription     = "A division within an organization"
	turtleExtensibleObject        = "extensible_object"
	turtleNoClassSample           = "@prefix ex: <http://example.org/> .\nex:foo a ex:Thing ."
)

const sampleTurtle = `
@prefix org: <http://example.com/org#> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

org:Organization a owl:Class ;
    rdfs:label "Organization" ;
    rdfs:comment "An organizational entity" .

org:Division a owl:Class ;
    rdfs:label "Division" ;
    rdfs:comment "A division within an organization" ;
    rdfs:subClassOf org:Organization .
`

func TestParseTurtleClasses(t *testing.T) {
	t.Parallel()
	classes := parseTurtleClasses([]byte(sampleTurtle))
	if len(classes) != 2 {
		t.Fatalf("expected 2 classes, got %d", len(classes))
	}
	byName := make(map[string]TurtleClass)
	for _, c := range classes {
		byName[c.LocalName] = c
	}
	if c := byName[turtleOrganizationID]; c.Label != turtleOrganizationLabel {
		t.Errorf("organization label = %q, want Organization", c.Label)
	} else if c.Comment != turtleOrganizationDescription {
		t.Errorf("organization comment = %q, want An organizational entity", c.Comment)
	}
	if c := byName[turtleDivisionID]; c.Label != turtleDivisionLabel {
		t.Errorf("division label = %q, want Division", c.Label)
	} else if c.Comment != turtleDivisionDescription {
		t.Errorf("division comment = %q, want A division within an organization", c.Comment)
	} else if c.SubClassOf != turtleOrganizationID {
		t.Errorf("division sub_class_of = %q, want organization", c.SubClassOf)
	}
}

func TestRDFOWLTranslator_Translate_ProducesDomainRegistry(t *testing.T) {
	t.Parallel()
	tr := NewRDFOWLTranslator()
	result, err := tr.Translate([]byte(sampleTurtle), nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if len(result.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(result.Objects))
	}
	obj := result.Objects[0]
	if obj.Kind != objects.KindDomainRegistry {
		t.Errorf("kind = %q, want %s", obj.Kind, objects.KindDomainRegistry)
	}
	domainsSlice, _ := obj.Data[KeyDomains].([]map[string]any)
	if len(domainsSlice) != 2 {
		t.Errorf("domains length = %d, want 2", len(domainsSlice))
	}
	// CRIT-7642: domains include title/description/sub_class_of from Turtle.
	var orgEntry, divEntry map[string]any
	for _, d := range domainsSlice {
		id, _ := d[objects.FieldKeyID].(string)
		switch id {
		case turtleOrganizationID:
			orgEntry = d
		case turtleDivisionID:
			divEntry = d
		}
	}
	if orgEntry != nil {
		if title, _ := orgEntry[KeyTitle].(string); title != turtleOrganizationLabel {
			t.Errorf("organization title = %q, want Organization", title)
		}
		if desc, _ := orgEntry[KeyDescription].(string); desc != turtleOrganizationDescription {
			t.Errorf("organization description = %q, want An organizational entity", desc)
		}
	}
	if divEntry != nil {
		if sub, _ := divEntry[KeySubClassOf].(string); sub != turtleOrganizationID {
			t.Errorf("division sub_class_of = %q, want organization", sub)
		}
	}
	// Object-spec-like output: one spec per class.
	if len(result.Specs) != 2 {
		t.Errorf("expected 2 specs, got %d", len(result.Specs))
	}
	for _, spec := range result.Specs {
		ontology, _ := spec[KeyOntology].(string)
		extends, _ := spec[KeyExtends].(string)
		desc, _ := spec[KeyDescription].(string)
		switch ontology {
		case turtleOrganizationID:
			if extends != turtleExtensibleObject {
				t.Errorf("organization spec extends = %q, want extensible_object", extends)
			}
			if desc != turtleOrganizationDescription {
				t.Errorf("organization spec description = %q", desc)
			}
		case turtleDivisionID:
			if extends != turtleOrganizationID {
				t.Errorf("division spec extends = %q, want organization", extends)
			}
		}
	}
}

func TestLocalNameFromSubject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		subject string
		want    string
	}{
		{"org:Organization", "organization"},
		{"org:Division", "division"},
		{"rdfs:label", "label"},
	}
	for _, tt := range tests {
		got := localNameFromSubject(tt.subject)
		if got != tt.want {
			t.Errorf("localNameFromSubject(%q) = %q, want %q", tt.subject, got, tt.want)
		}
	}
}

func TestParseTurtleClasses_Empty(t *testing.T) {
	t.Parallel()
	classes := parseTurtleClasses([]byte(""))
	if len(classes) != 0 {
		t.Errorf("expected 0 classes, got %d", len(classes))
	}
}

func TestParseTurtleClasses_NoClass(t *testing.T) {
	t.Parallel()
	classes := parseTurtleClasses([]byte(turtleNoClassSample))
	if len(classes) != 0 {
		t.Errorf("expected 0 classes (no owl:Class), got %d", len(classes))
	}
}
