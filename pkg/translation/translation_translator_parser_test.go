// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package translation

import (
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestCypherTranslator_EdgeCasesAndComprehensive(t *testing.T) {
	t.Parallel()

	tr := NewCypherTranslator()
	if tr.SourceFormat() != FormatCypher {
		t.Fatalf("expected format %q, got %q", FormatCypher, tr.SourceFormat())
	}

	opts := &TranslateOptions{SourceFile: "schema.cypher"}

	t.Run("empty_and_no_match", func(t *testing.T) {
		resEmpty, err := tr.Translate(nil, opts)
		if err != nil {
			t.Fatalf("unexpected error on nil input: %v", err)
		}
		if len(resEmpty.Objects) != 0 {
			t.Fatalf("expected 0 objects for nil input, got %d", len(resEmpty.Objects))
		}

		resZeroByte, err := tr.Translate([]byte(""), opts)
		if err != nil {
			t.Fatalf("unexpected error on zero byte input: %v", err)
		}
		if len(resZeroByte.Objects) != 0 {
			t.Fatalf("expected 0 objects for zero-byte input, got %d", len(resZeroByte.Objects))
		}

		resNoMatch, err := tr.Translate([]byte("MATCH (n) RETURN n LIMIT 10;"), opts)
		if err != nil {
			t.Fatalf("unexpected error on query without labels: %v", err)
		}
		if len(resNoMatch.Objects) != 0 {
			t.Fatalf("expected 0 objects for query without labels, got %d", len(resNoMatch.Objects))
		}
	})

	t.Run("create_nodes_and_constraints", func(t *testing.T) {
		cypherScript := `
			CREATE CONSTRAINT FOR (u:UserAccount) REQUIRE u.id IS UNIQUE;
			CREATE (g:UserGroup {name: "Engineers"});
			CREATE (:ProjectDomain);
			CREATE (u:UserAccount {status: "active"});
		`
		res, err := tr.Translate([]byte(cypherScript), opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 1 {
			t.Fatalf("expected 1 domain registry object, got %d", len(res.Objects))
		}
		obj := res.Objects[0]
		if obj.Kind != DomainRegistryKind {
			t.Fatalf("expected kind %s, got %s", DomainRegistryKind, obj.Kind)
		}
		domains, ok := obj.Data[KeyDomains].([]map[string]any)
		if !ok || len(domains) != 3 {
			t.Fatalf("expected 3 distinct domain entries, got %d", len(domains))
		}
		if len(res.Specs) != 3 {
			t.Fatalf("expected 3 specs, got %d", len(res.Specs))
		}
		for _, spec := range res.Specs {
			if spec[KeyExtends] != DefaultExtends {
				t.Fatalf("expected extends %s, got %v", DefaultExtends, spec[KeyExtends])
			}
			if spec[KeyVisibility] != DefaultVisibility {
				t.Fatalf("expected visibility %s, got %v", DefaultVisibility, spec[KeyVisibility])
			}
		}
	})
}

func TestJSONSchemaTranslator_EdgeCasesAndMetadata(t *testing.T) {
	t.Parallel()

	tr := NewJSONSchemaTranslator()
	if tr.SourceFormat() != FormatJSONSchema {
		t.Fatalf("expected format %q, got %q", FormatJSONSchema, tr.SourceFormat())
	}

	opts := &TranslateOptions{SourceFile: "schema.json"}

	t.Run("empty_and_invalid", func(t *testing.T) {
		res, err := tr.Translate(nil, opts)
		if err != nil {
			t.Fatalf("unexpected error on nil: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects, got %d", len(res.Objects))
		}

		res, err = tr.Translate([]byte("not valid json"), opts)
		if err != nil {
			t.Fatalf("unexpected error on invalid json: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects, got %d", len(res.Objects))
		}

		res, err = tr.Translate([]byte(`{"$schema": "draft-07"}`), opts)
		if err != nil {
			t.Fatalf("unexpected error on missing definitions: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects, got %d", len(res.Objects))
		}
	})

	t.Run("whitespace_definitions", func(t *testing.T) {
		raw := []byte(`{"definitions": {"   ": {"type": "object"}}}`)
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error on whitespace def: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects, got %d", len(res.Objects))
		}
	})

	t.Run("rich_metadata_matrix", func(t *testing.T) {
		raw := []byte(`{
			"definitions": {
				"FullMeta": {"type": "object", "title": "Full Title", "description": "Full Desc"},
				"TitleOnly": {"type": "object", "title": "Title Only"},
				"DescOnly": {"type": "object", "description": "Desc Only"},
				"BareNode": {"type": "object"}
			}
		}`)
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 1 {
			t.Fatalf("expected 1 registry object, got %d", len(res.Objects))
		}
		domains, ok := res.Objects[0].Data[KeyDomains].([]map[string]any)
		if !ok || len(domains) != 4 {
			t.Fatalf("expected 4 domains, got %d", len(domains))
		}
		if len(res.Specs) != 4 {
			t.Fatalf("expected 4 specs, got %d", len(res.Specs))
		}

		verifyJSONSchemaSpecs(t, res.Specs)
	})
}

func verifyJSONSchemaSpecs(t *testing.T, specs []map[string]any) {
	t.Helper()
	specsByOntology := make(map[string]map[string]any)
	for _, s := range specs {
		ont, ok := s[KeyOntology].(string)
		if ok {
			specsByOntology[ont] = s
		}
	}

	full := specsByOntology["FullMeta"]
	if full[KeyTitle] != "Full Title" || full[KeyDescription] != "Full Desc" {
		t.Fatalf("FullMeta spec mismatch: title=%v desc=%v", full[KeyTitle], full[KeyDescription])
	}

	titleOnly := specsByOntology["TitleOnly"]
	if titleOnly[KeyTitle] != "Title Only" || titleOnly[KeyDescription] != nil {
		t.Fatalf("TitleOnly spec mismatch: title=%v desc=%v", titleOnly[KeyTitle], titleOnly[KeyDescription])
	}

	descOnly := specsByOntology["DescOnly"]
	if descOnly[KeyTitle] != nil || descOnly[KeyDescription] != "Desc Only" {
		t.Fatalf("DescOnly spec mismatch: title=%v desc=%v", descOnly[KeyTitle], descOnly[KeyDescription])
	}

	bare := specsByOntology["BareNode"]
	if bare[KeyTitle] != nil || bare[KeyDescription] != nil {
		t.Fatalf("BareNode spec mismatch: title=%v desc=%v", bare[KeyTitle], bare[KeyDescription])
	}
}

func TestJSONLD_ParserBoundariesAndSubClassDerivation(t *testing.T) {
	t.Parallel()

	t.Run("invalid_and_empty_graph", func(t *testing.T) {
		classes, err := parseJSONLDClasses([]byte("{malformed"))
		if err == nil {
			t.Fatalf("expected error for malformed json")
		}
		if classes != nil {
			t.Fatalf("expected nil classes on error")
		}

		classes, err = parseJSONLDClasses([]byte(`{"@graph": []}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(classes) != 0 {
			t.Fatalf("expected 0 classes, got %d", len(classes))
		}
	})

	t.Run("graph_with_non_map_elements_and_empty_id", func(t *testing.T) {
		raw := []byte(`{
			"@graph": [
				"plain string item",
				12345,
				{"@id": "ex:Person", "@type": "schema:Person"},
				{"@id": "", "@type": "owl:Class"},
				{"@id": "   ", "@type": "owl:Class"},
				{"@id": "org:Valid", "@type": "owl:Class", "rdfs:label": "Valid Node"},
				{"@id": "org:Valid", "@type": "owl:Class", "rdfs:label": "Duplicate Node"}
			]
		}`)
		classes, err := parseJSONLDClasses(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(classes) != 1 {
			t.Fatalf("expected 1 class after duplicate & empty filtering, got %d", len(classes))
		}
		if classes[0].LocalName != "valid" {
			t.Fatalf("expected localName 'valid', got %q", classes[0].LocalName)
		}
	})

	t.Run("single_object_without_context", func(t *testing.T) {
		raw := []byte(`{"@id": "schema:Service", "@type": "Class", "label": "Service Class"}`)
		classes, err := parseJSONLDClasses(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(classes) != 1 || classes[0].LocalName != "service" {
			t.Fatalf("expected service class, got %+v", classes)
		}
	})
}

func TestJSONLD_OWLClassNodeIdentification(t *testing.T) {
	t.Parallel()

	if isOWLClassNode(map[string]any{}) {
		t.Fatalf("expected false for missing @type")
	}
	if isOWLClassNode(map[string]any{jsonLDKeyType: 999}) {
		t.Fatalf("expected false for non-string, non-slice @type")
	}
	if isOWLClassNode(map[string]any{jsonLDKeyType: "schema:Person"}) {
		t.Fatalf("expected false for non-class type")
	}
	if !isOWLClassNode(map[string]any{jsonLDKeyType: "owl:Class"}) {
		t.Fatalf("expected true for owl:Class string")
	}
	if !isOWLClassNode(map[string]any{jsonLDKeyType: "rdfs:Class"}) {
		t.Fatalf("expected true for rdfs:Class string")
	}

	// Slice of any types
	if isOWLClassNode(map[string]any{jsonLDKeyType: []any{123, false}}) {
		t.Fatalf("expected false for slice of ints/bools")
	}
	if isOWLClassNode(map[string]any{jsonLDKeyType: []any{"schema:Thing", "schema:Action"}}) {
		t.Fatalf("expected false for slice of non-class strings")
	}
	if !isOWLClassNode(map[string]any{jsonLDKeyType: []any{"schema:Thing", "owl:Class"}}) {
		t.Fatalf("expected true for slice containing owl:Class")
	}
}

func TestJSONLD_GetStringAndSubClassOfRefVariants(t *testing.T) {
	t.Parallel()

	t.Run("get_string_arrays_and_maps", func(t *testing.T) {
		m := map[string]any{
			"strKey":    "directValue",
			"arrStrKey": []any{"firstVal", "secondVal"},
			"arrMapKey": []any{map[string]any{jsonLDKeyValue: "localizedValue"}},
			"arrBadMap": []any{map[string]any{"otherKey": "ignored"}},
			"arrEmpty":  []any{},
			"intKey":    42,
		}

		if got := getString(m, "strKey"); got != "directValue" {
			t.Fatalf("expected directValue, got %q", got)
		}
		if got := getString(m, "arrStrKey"); got != "firstVal" {
			t.Fatalf("expected firstVal, got %q", got)
		}
		if got := getString(m, "arrMapKey"); got != "localizedValue" {
			t.Fatalf("expected localizedValue, got %q", got)
		}
		if got := getString(m, "arrBadMap"); got != "" {
			t.Fatalf("expected empty for bad map without @value, got %q", got)
		}
		if got := getString(m, "arrEmpty"); got != "" {
			t.Fatalf("expected empty for empty array, got %q", got)
		}
		if got := getString(m, "intKey"); got != "" {
			t.Fatalf("expected empty for int, got %q", got)
		}
		if got := getString(m, "nonExistentKey"); got != "" {
			t.Fatalf("expected empty for non-existent key, got %q", got)
		}
	})

	t.Run("get_sub_class_of_ref_variants", func(t *testing.T) {
		mNoKey := map[string]any{}
		if got := getSubClassOfRef(mNoKey); got != "" {
			t.Fatalf("expected empty for missing subclass key, got %q", got)
		}

		mStr := map[string]any{jsonLDKeySubClassOf: "org:Parent"}
		if got := getSubClassOfRef(mStr); got != "org:Parent" {
			t.Fatalf("expected org:Parent, got %q", got)
		}

		mMap := map[string]any{jsonLDKeySubClassOf: map[string]any{jsonLDKeyID: "org:ParentMap"}}
		if got := getSubClassOfRef(mMap); got != "org:ParentMap" {
			t.Fatalf("expected org:ParentMap, got %q", got)
		}

		mMapNoID := map[string]any{jsonLDKeySubClassOf: map[string]any{"title": "NoID"}}
		if got := getSubClassOfRef(mMapNoID); got != "" {
			t.Fatalf("expected empty for map without @id, got %q", got)
		}

		mArrStr := map[string]any{jsonLDKeySubClassOf: []any{"org:ParentArrStr"}}
		if got := getSubClassOfRef(mArrStr); got != "org:ParentArrStr" {
			t.Fatalf("expected org:ParentArrStr, got %q", got)
		}

		mArrMap := map[string]any{jsonLDKeySubClassOf: []any{map[string]any{jsonLDKeyID: "org:ParentArrMap"}}}
		if got := getSubClassOfRef(mArrMap); got != "org:ParentArrMap" {
			t.Fatalf("expected org:ParentArrMap, got %q", got)
		}

		mArrBadMap := map[string]any{jsonLDKeySubClassOf: []any{map[string]any{"title": "ArrNoID"}}}
		if got := getSubClassOfRef(mArrBadMap); got != "" {
			t.Fatalf("expected empty for array map without @id, got %q", got)
		}

		mArrEmpty := map[string]any{jsonLDKeySubClassOf: []any{}}
		if got := getSubClassOfRef(mArrEmpty); got != "" {
			t.Fatalf("expected empty for empty array, got %q", got)
		}

		mInt := map[string]any{jsonLDKeySubClassOf: 777}
		if got := getSubClassOfRef(mInt); got != "" {
			t.Fatalf("expected empty for int subclass, got %q", got)
		}
	})
}

func TestJSONLD_LocalNameParsingBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"TrailingHash#", "trailinghash#"},
		{"TrailingSlash/", "trailingslash/"},
		{"ns:TrailingColon:", "ns:trailingcolon:"},
		{"BareClassName", "bareclassname"},
		{"http://example.org/ns#ValidClass", "validclass"},
		{"http://example.org/ns/PathClass", "pathclass"},
		{"prefix:ColonClass", "colonclass"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run("id_"+tc.input, func(t *testing.T) {
			t.Parallel()
			got := localNameFromJSONLDID(tc.input)
			if got != tc.expected {
				t.Fatalf("localNameFromJSONLDID(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}

	if got := localNameFromJSONLDIDRef(""); got != "" {
		t.Fatalf("expected empty from empty ref, got %q", got)
	}
	if got := localNameFromJSONLDIDRef("prefix:SomeRef"); got != "someref" {
		t.Fatalf("expected someref, got %q", got)
	}
}

func TestOpenAPITranslator_FullBranchCoverage(t *testing.T) {
	t.Parallel()

	tr := NewOpenAPITranslator()
	if tr.SourceFormat() != FormatOpenAPI {
		t.Fatalf("expected format %q, got %q", FormatOpenAPI, tr.SourceFormat())
	}

	opts := &TranslateOptions{SourceFile: "api.yaml"}

	t.Run("nil_and_empty", func(t *testing.T) {
		res, err := tr.Translate(nil, opts)
		if err != nil {
			t.Fatalf("unexpected error on nil: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects, got %d", len(res.Objects))
		}
	})

	t.Run("empty_schemas_and_definitions", func(t *testing.T) {
		raw := []byte("openapi: 3.1.0\ninfo:\n  title: API\n")
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects when schemas/definitions absent, got %d", len(res.Objects))
		}
	})

	t.Run("whitespace_only_schema_names", func(t *testing.T) {
		raw := []byte("openapi: 3.0.0\ncomponents:\n  schemas:\n    '   ':\n      type: object\n")
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects for whitespace schema names, got %d", len(res.Objects))
		}
	})

	t.Run("yaml_definitions_fallback_and_metadata_variations", func(t *testing.T) {
		raw := []byte(`
swagger: "2.0"
definitions:
  FullMeta:
    type: object
    title: Full Title
    description: Full Desc
  TitleOnly:
    type: object
    title: Title Only
  NoTitle:
    type: object
    description: Should Not Be In Meta
`)
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 1 {
			t.Fatalf("expected 1 registry object, got %d", len(res.Objects))
		}
		if len(res.Specs) != 3 {
			t.Fatalf("expected 3 specs, got %d", len(res.Specs))
		}

		verifyOpenAPISpecs(t, res.Specs)
	})
}

func verifyOpenAPISpecs(t *testing.T, specs []map[string]any) {
	t.Helper()
	specsByOntology := make(map[string]map[string]any)
	for _, s := range specs {
		ont, ok := s[KeyOntology].(string)
		if ok {
			specsByOntology[ont] = s
		}
	}

	full := specsByOntology["FullMeta"]
	if full[KeyTitle] != "Full Title" || full[KeyDescription] != "Full Desc" {
		t.Fatalf("FullMeta spec mismatch: title=%v desc=%v", full[KeyTitle], full[KeyDescription])
	}

	titleOnly := specsByOntology["TitleOnly"]
	if titleOnly[KeyTitle] != "Title Only" || titleOnly[KeyDescription] != nil {
		t.Fatalf("TitleOnly spec mismatch: title=%v desc=%v", titleOnly[KeyTitle], titleOnly[KeyDescription])
	}

	noTitle := specsByOntology["NoTitle"]
	if noTitle[KeyTitle] != nil || noTitle[KeyDescription] != nil {
		t.Fatalf("NoTitle spec mismatch (title must be present for meta): title=%v desc=%v", noTitle[KeyTitle], noTitle[KeyDescription])
	}
}

func TestRDFOWLTranslator_ComprehensiveBranches(t *testing.T) {
	t.Parallel()

	tr := NewRDFOWLTranslator()
	opts := &TranslateOptions{SourceFile: "ontology.owl"}

	t.Run("nil_and_empty", func(t *testing.T) {
		res, err := tr.Translate(nil, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects, got %d", len(res.Objects))
		}
	})

	t.Run("unrecognized_syntax", func(t *testing.T) {
		res, err := tr.Translate([]byte("completely plain text with no markers"), opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects for plain text, got %d", len(res.Objects))
		}
	})

	t.Run("json_array_jsonld", func(t *testing.T) {
		raw := []byte(`[
			{"@id": "ex:ClassOne", "@type": "owl:Class", "rdfs:label": "Class One"},
			{"@id": "ex:ClassTwo", "@type": "owl:Class", "rdfs:label": "Class Two"}
		]`)
		// json array is not a top-level map, parseJSONLDClasses fails json.Unmarshal, falls back cleanly
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects when unmarshal fails, got %d", len(res.Objects))
		}
	})

	t.Run("turtle_markers_present_but_no_class", func(t *testing.T) {
		raw := []byte("rdfs:label \"Lonely label with no subject or class\" .")
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects, got %d", len(res.Objects))
		}
	})

	t.Run("rdf_xml_markers_present_but_no_class", func(t *testing.T) {
		raw := []byte("<rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\"></rdf:RDF>")
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects, got %d", len(res.Objects))
		}
	})

	t.Run("classes_with_blank_local_name", func(t *testing.T) {
		raw := []byte(`
			<rdf:RDF xmlns:owl="http://www.w3.org/2002/07/owl#">
				<owl:Class rdf:about="   ">
					<rdfs:label>Empty Local</rdfs:label>
				</owl:Class>
			</rdf:RDF>
		`)
		res, err := tr.Translate(raw, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Objects) != 0 {
			t.Fatalf("expected 0 objects when all classes have blank localName, got %d", len(res.Objects))
		}
	})
}

func TestRDFXML_ParserDetailedBranches(t *testing.T) {
	t.Parallel()

	t.Run("no_about_matches", func(t *testing.T) {
		classes := parseRDFXMLClasses([]byte("<root><something/></root>"))
		if classes != nil {
			t.Fatalf("expected nil classes when no matches found, got %v", classes)
		}
	})

	t.Run("single_class_with_close_tag_and_root_close", func(t *testing.T) {
		raw := `
			<rdf:RDF xmlns:owl="http://www.w3.org/2002/07/owl#">
				<owl:Class rdf:about="http://example.com/ns#Cluster">
					<rdfs:label>Cluster Label</rdfs:label>
					<rdfs:comment>Cluster Comment</rdfs:comment>
					<rdfs:subClassOf rdf:resource="http://example.com/ns#Infrastructure"/>
				</owl:Class>
			</rdf:RDF>
		`
		classes := parseRDFXMLClasses([]byte(raw))
		if len(classes) != 1 {
			t.Fatalf("expected 1 class, got %d", len(classes))
		}
		c := classes[0]
		if c.LocalName != "cluster" || c.Label != "Cluster Label" || c.Comment != "Cluster Comment" || c.SubClassOf != "infrastructure" {
			t.Fatalf("class mismatch: %+v", c)
		}
	})

	t.Run("single_class_without_close_tag_closing_at_root", func(t *testing.T) {
		raw := `
			<rdf:RDF xmlns:owl="http://www.w3.org/2002/07/owl#">
				<owl:Class rdf:about="http://example.com/ns#Pod">
					<rdfs:label>Pod Label</rdfs:label>
			</rdf:RDF>
		`
		classes := parseRDFXMLClasses([]byte(raw))
		if len(classes) != 1 {
			t.Fatalf("expected 1 class, got %d", len(classes))
		}
		if classes[0].LocalName != "pod" || classes[0].Label != "Pod Label" {
			t.Fatalf("unexpected class: %+v", classes[0])
		}
	})

	t.Run("local_name_from_rdf_about_edge_cases", func(t *testing.T) {
		if got := localNameFromRDFAbout(""); got != "" {
			t.Fatalf("expected empty for empty string, got %q", got)
		}
		if got := localNameFromRDFAbout("   "); got != "" {
			t.Fatalf("expected empty for whitespace, got %q", got)
		}
		if got := localNameFromRDFAbout("http://example.com/org#"); got != "org#" {
			t.Fatalf("expected org#, got %q", got)
		}
		if got := localNameFromRDFAbout("http://example.com/org/"); got != "http://example.com/org/" {
			t.Fatalf("expected full string, got %q", got)
		}
		if got := localNameFromRDFAbout("BareName"); got != "barename" {
			t.Fatalf("expected barename, got %q", got)
		}
	})

	t.Run("first_match_helper", func(t *testing.T) {
		if got := firstMatch("no match here", rdfXmlLabel); got != "" {
			t.Fatalf("expected empty for non-match, got %q", got)
		}
	})
}

func TestTurtle_ParserDetailedBranches(t *testing.T) {
	t.Parallel()

	t.Run("inline_subject_and_flush_boundaries", func(t *testing.T) {
		raw := `
			# Leading comment
			@prefix ex: <http://example.org/> .

			ex:FastClass a owl:Class ; rdfs:label "Fast Label" ; rdfs:comment "Fast Comment" ; rdfs:subClassOf ex:BaseClass .

			ex:MultilineClass a owl:Class ;
				rdfs:label "Multi Label" ;
				rdfs:comment "Multi Comment" ;
				rdfs:subClassOf ex:FastClass .
		`
		classes := parseTurtleClasses([]byte(raw))
		if len(classes) != 2 {
			t.Fatalf("expected 2 classes, got %d", len(classes))
		}
		byName := make(map[string]TurtleClass)
		for _, c := range classes {
			byName[c.LocalName] = c
		}
		fast := byName["fastclass"]
		if fast.Label != "Fast Label" || fast.Comment != "Fast Comment" || fast.SubClassOf != "baseclass" {
			t.Fatalf("fastclass mismatch: %+v", fast)
		}
		multi := byName["multilineclass"]
		if multi.Label != "Multi Label" || multi.Comment != "Multi Comment" || multi.SubClassOf != "fastclass" {
			t.Fatalf("multilineclass mismatch: %+v", multi)
		}
	})

	t.Run("flush_current_empty_handling", func(t *testing.T) {
		seen := make(map[string]TurtleClass)
		flushCurrent(seen, "", "Label", "Comment", "Sub")
		if len(seen) != 0 {
			t.Fatalf("expected 0 entries for empty subject")
		}
		flushCurrent(seen, "Subj", "", "Comment", "Sub")
		if len(seen) != 0 {
			t.Fatalf("expected 0 entries for empty label")
		}
		flushCurrent(seen, "Subj", "Label", "Comment", "Sub")
		if len(seen) != 1 {
			t.Fatalf("expected 1 entry when subject and label provided")
		}
	})

	t.Run("local_name_from_subject_edge_cases", func(t *testing.T) {
		cases := []struct {
			input    string
			expected string
		}{
			{"", ""},
			{"   ", ""},
			{"ex:Valid", "valid"},
			{"ex:", "ex:"},
			{"<#UriHash>", "urihash"},
			{"</path/UriSlash>", "urislash"},
			{"<no_separator>", ""},
			{"<trailing_hash#>", ""},
			{"<trailing_slash/>", ""},
			{"BareSubject", "baresubject"},
		}

		for _, tc := range cases {
			tc := tc
			t.Run("subj_"+tc.input, func(t *testing.T) {
				t.Parallel()
				got := localNameFromSubject(tc.input)
				if got != tc.expected {
					t.Fatalf("localNameFromSubject(%q) = %q, want %q", tc.input, got, tc.expected)
				}
			})
		}
	})

	t.Run("subject_with_blank_local_name_skipped_in_loop", func(t *testing.T) {
		raw := `
			<no_separator> a owl:Class ;
				rdfs:label "Should Be Skipped" .
		`
		classes := parseTurtleClasses([]byte(raw))
		if len(classes) != 0 {
			t.Fatalf("expected 0 classes when localName is blank, got %d", len(classes))
		}
	})
}

type dummyTestTranslator struct {
	format string
}

func (d *dummyTestTranslator) SourceFormat() string { return d.format }
func (d *dummyTestTranslator) Translate(_ []byte, _ *TranslateOptions) (*TranslateResult, error) {
	return &TranslateResult{
		Objects: []TranslatedObject{
			{Kind: "dummy_kind", Data: map[string]any{"id": "dummy-1"}},
		},
	}, nil
}

func TestRegistry_NilTranslatorsMapInitializationAndCustomRegistration(t *testing.T) {
	var orig map[string]Translator
	lockErr := concurrency.RunInLock(&registryMu, func() error {
		orig = registry.translators
		registry.translators = nil
		return nil
	})
	if lockErr != nil {
		t.Fatalf("failed to lock registry: %v", lockErr)
	}

	dummy := &dummyTestTranslator{format: "custom_format_v1"}
	Register(dummy)

	t.Cleanup(func() {
		cleanupErr := concurrency.RunInLock(&registryMu, func() error {
			for k, v := range orig {
				registry.translators[k] = v
			}
			delete(registry.translators, "custom_format_v1")
			return nil
		})
		if cleanupErr != nil {
			t.Errorf("failed to restore registry: %v", cleanupErr)
		}
	})

	fetched := Get("custom_format_v1")
	if fetched == nil || fetched.SourceFormat() != "custom_format_v1" {
		t.Fatalf("failed to retrieve registered custom translator")
	}

	res, err := TranslateByFormat("custom_format_v1", []byte("sample"), nil)
	if err != nil {
		t.Fatalf("TranslateByFormat failed for custom format: %v", err)
	}
	if len(res.Objects) != 1 || res.Objects[0].Kind != "dummy_kind" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRegistry_ConcurrencyStress(t *testing.T) {
	t.Parallel()

	formats := []string{FormatRDFOWL, FormatTurtle, FormatCypher, FormatJSONSchema, FormatOpenAPI}
	const numGoroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("registry_concurrency_stress", fmt.Sprintf("worker-%d", workerID))
		builder.StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				format := formats[(workerID+j)%len(formats)]
				tr := Get(format)
				if tr == nil {
					continue
				}
				opts := &TranslateOptions{SourceFile: fmt.Sprintf("test-%d-%d.src", workerID, j)}
				res, err := tr.Translate(nil, opts)
				if err != nil || res == nil {
					continue
				}
			}
		})
	}

	wg.Wait()
}

func TestTranslatedObjectAndSpecStructureInvariants(t *testing.T) {
	t.Parallel()

	obj := TranslatedObject{
		Kind: objects.KindDomainRegistry,
		Data: map[string]any{
			objects.FieldKeyID:   DomainRegistryPlaceholderID,
			objects.FieldKeyKind: DomainRegistryKind,
		},
	}

	if obj.Kind != objects.KindDomainRegistry {
		t.Fatalf("expected kind %s, got %s", objects.KindDomainRegistry, obj.Kind)
	}
	if obj.Data[objects.FieldKeyID] != DomainRegistryPlaceholderID {
		t.Fatalf("expected placeholder id, got %v", obj.Data[objects.FieldKeyID])
	}
}
