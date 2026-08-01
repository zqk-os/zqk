package translation

import (
	"regexp"
	"strings"
)

const (
	rdfXMLCloseClassTag = "</owl:Class>"
	rdfXMLCloseRootTag  = "</rdf:RDF>"
	rdfXMLFragmentSep   = "#"
	rdfXMLPathSep       = "/"
)

// Minimal RDF/XML parsing to extract owl:Class with rdfs:label, rdfs:comment, rdfs:subClassOf.
// ITEM-712: basic RDF/OWL import; sufficient for sample .rdf/.owl files in RDF/XML syntax.
// Not a full RDF/XML parser; handles common patterns like:
//   <owl:Class rdf:about="http://example.org/Organization">...</owl:Class>

var (
	// rdfXmlClassAbout matches <owl:Class rdf:about="..."> (with possible ns prefix on Class).
	rdfXmlClassAbout = regexp.MustCompile(`<(?:owl:)?Class\s+rdf:about\s*=\s*["']([^"']+)["']`)
	// rdfXmlLabel matches <rdfs:label>...</rdfs:label> or <rdfs:label xml:lang="en">...</rdfs:label>
	rdfXmlLabel = regexp.MustCompile(`<rdfs:label[^>]*>([^<]*)</rdfs:label>`)
	// rdfXmlComment matches <rdfs:comment>...</rdfs:comment>
	rdfXmlComment = regexp.MustCompile(`<rdfs:comment[^>]*>([^<]*)</rdfs:comment>`)
	// rdfXmlSubClassOfResource matches <rdfs:subClassOf rdf:resource="..."/>
	rdfXmlSubClassOfResource = regexp.MustCompile(`<rdfs:subClassOf\s+rdf:resource\s*=\s*["']([^"']+)["']`)
)

// parseRDFXMLClasses extracts owl:Class elements from RDF/XML and returns TurtleClass slices.
// Each class gets LocalName from the rdf:about URI (fragment or path segment); Label, Comment, SubClassOf from child elements.
func parseRDFXMLClasses(raw []byte) []TurtleClass {
	text := string(raw)
	// Find each <owl:Class rdf:about="..."> block (up to </owl:Class> or next <owl:Class).
	aboutMatches := rdfXmlClassAbout.FindAllStringSubmatchIndex(text, -1)
	if len(aboutMatches) == 0 {
		return nil
	}
	var classes []TurtleClass
	for i, m := range aboutMatches {
		aboutURI := text[m[2]:m[3]]
		localName := localNameFromRDFAbout(aboutURI)
		if localName == emptyValue {
			continue
		}
		// Block: from this tag to the next <owl:Class or </rdf:RDF or end of file
		blockStart := m[0]
		blockEnd := len(text)
		if i+1 < len(aboutMatches) {
			blockEnd = aboutMatches[i+1][0]
		} else {
			if idx := strings.Index(text[blockStart:], rdfXMLCloseClassTag); idx >= 0 {
				blockEnd = blockStart + idx + len(rdfXMLCloseClassTag)
			}
			if idx := strings.Index(text[blockStart:], rdfXMLCloseRootTag); idx >= 0 && blockStart+idx < blockEnd {
				blockEnd = blockStart + idx
			}
		}
		block := text[blockStart:blockEnd]
		label := firstMatch(block, rdfXmlLabel)
		comment := firstMatch(block, rdfXmlComment)
		subClassOfURI := firstMatch(block, rdfXmlSubClassOfResource)
		subClassOf := emptyValue
		if subClassOfURI != emptyValue {
			subClassOf = localNameFromRDFAbout(subClassOfURI)
		}
		classes = append(classes, TurtleClass{
			LocalName:  localName,
			Label:      strings.TrimSpace(label),
			Comment:    strings.TrimSpace(comment),
			SubClassOf: subClassOf,
		})
	}
	return classes
}

func firstMatch(text string, re *regexp.Regexp) string {
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return emptyValue
	}
	return m[1]
}

// localNameFromRDFAbout derives a slug from an rdf:about URI (e.g. http://example.org/org#Organization -> organization).
func localNameFromRDFAbout(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == emptyValue {
		return emptyValue
	}
	// Fragment: ...#Organization -> organization
	if idx := strings.LastIndex(uri, rdfXMLFragmentSep); idx >= 0 && idx < len(uri)-1 {
		return strings.ToLower(uri[idx+1:])
	}
	// Path: .../Organization -> organization
	if idx := strings.LastIndex(uri, rdfXMLPathSep); idx >= 0 && idx < len(uri)-1 {
		return strings.ToLower(uri[idx+1:])
	}
	return strings.ToLower(uri)
}
