package translation

import (
	"regexp"
	"strings"
)

const (
	turtleCommentPrefix = "#"
	turtleLineSeparator = "\n"
	turtleSpace         = " "
	turtleTerminator    = "."
	turtlePrefixURITrim = "<>"
)

// TurtleClass holds extracted owl:Class metadata for domain_registry translation.
// CRIT-7642: richer parsing (label, comment, subClassOf) for domain model mapping.
type TurtleClass struct {
	LocalName  string // Slug from subject (e.g. "organization")
	Label      string // rdfs:label
	Comment    string // rdfs:comment (description)
	SubClassOf string // Parent class local name from rdfs:subClassOf (e.g. "organization")
}

// Minimal Turtle parsing to extract owl:Class, rdfs:label, rdfs:comment, rdfs:subClassOf.
// Not a full Turtle parser; sufficient for sample .ttl files with owl:Class and rdfs terms.

var (
	// prefixDecl matches @prefix pfx: <uri> .
	prefixDecl = regexp.MustCompile(`^\s*@prefix\s+(\S+)\s*:\s*<\S+>\s*\.?\s*$`)
	// classSubject matches subject a owl:Class (subject is prefix:LocalName or <uri>).
	classSubject = regexp.MustCompile(`(\S+)\s+a\s+owl:Class\s*[;.]`)
	// rdfsLabel matches rdfs:label "value" .
	rdfsLabel = regexp.MustCompile(`rdfs:label\s+"([^"]*)"\s*[;.]`)
	// rdfsComment matches rdfs:comment "value" .
	rdfsComment = regexp.MustCompile(`rdfs:comment\s+"([^"]*)"\s*[;.]`)
	// rdfsSubClassOf matches rdfs:subClassOf prefix:LocalName or <uri> (capture full object).
	rdfsSubClassOf = regexp.MustCompile(`rdfs:subClassOf\s+(\S+)\s*[;.]`)
)

// parseTurtleClasses extracts owl:Class subjects with rdfs:label, rdfs:comment, rdfs:subClassOf.
// Returns a list of TurtleClass; LocalName is derived from subject (prefix:LocalName or URI).
func parseTurtleClasses(raw []byte) (classes []TurtleClass) {
	text := string(raw)
	lines := strings.Split(text, turtleLineSeparator)
	var currentSubject string
	var currentLabel, currentComment, currentSubClassOf string
	seen := make(map[string]TurtleClass) // subject -> class metadata

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == emptyValue || strings.HasPrefix(line, turtleCommentPrefix) {
			continue
		}
		if prefixDecl.MatchString(line) {
			continue
		}
		if m := classSubject.FindStringSubmatch(line); len(m) == 2 {
			flushCurrent(seen, currentSubject, currentLabel, currentComment, currentSubClassOf)
			currentSubject = m[1]
			currentLabel = emptyValue
			currentComment = emptyValue
			currentSubClassOf = emptyValue
			if lm := rdfsLabel.FindStringSubmatch(line); len(lm) == 2 {
				currentLabel = lm[1]
			}
			if cm := rdfsComment.FindStringSubmatch(line); len(cm) == 2 {
				currentComment = cm[1]
			}
			if sm := rdfsSubClassOf.FindStringSubmatch(line); len(sm) == 2 {
				currentSubClassOf = localNameFromSubject(sm[1])
			}
			continue
		}
		if lm := rdfsLabel.FindStringSubmatch(line); len(lm) == 2 {
			currentLabel = lm[1]
			continue
		}
		if cm := rdfsComment.FindStringSubmatch(line); len(cm) == 2 {
			currentComment = cm[1]
			continue
		}
		if sm := rdfsSubClassOf.FindStringSubmatch(line); len(sm) == 2 {
			currentSubClassOf = localNameFromSubject(sm[1])
			continue
		}
		if strings.HasSuffix(strings.TrimRight(line, turtleSpace), turtleTerminator) {
			flushCurrent(seen, currentSubject, currentLabel, currentComment, currentSubClassOf)
			currentSubject = emptyValue
			currentLabel = emptyValue
			currentComment = emptyValue
			currentSubClassOf = emptyValue
		}
	}
	flushCurrent(seen, currentSubject, currentLabel, currentComment, currentSubClassOf)

	for subj, tc := range seen {
		localName := localNameFromSubject(subj)
		if localName == emptyValue {
			continue
		}
		tc.LocalName = localName
		classes = append(classes, tc)
	}
	return classes
}

func flushCurrent(seen map[string]TurtleClass, subject, label, comment, subClassOf string) {
	if subject == emptyValue || label == emptyValue {
		return
	}
	seen[subject] = TurtleClass{Label: label, Comment: comment, SubClassOf: subClassOf}
}

// localNameFromSubject derives a slug from prefix:LocalName or <uri>#Name> or </path/Name>.
func localNameFromSubject(s string) string {
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return emptyValue
	}
	// prefix:LocalName
	if idx := strings.LastIndex(s, ":"); idx >= 0 && idx < len(s)-1 {
		return strings.ToLower(s[idx+1:])
	}
	// <uri#LocalName> or <uri/LocalName>
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		inner := strings.Trim(s, turtlePrefixURITrim)
		if idx := strings.LastIndexAny(inner, "#/"); idx >= 0 && idx < len(inner)-1 {
			return strings.ToLower(inner[idx+1:])
		}
		return emptyValue
	}
	return strings.ToLower(s)
}
