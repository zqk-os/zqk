package workpack

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Message and format constants for the redundancy audit tooling.
const (
	msgSpecClean   = "spec %s: clean"
	msgSpecViolate = "spec %s: redundant=[%s] duplicates=[%s]"
	msgPackEnabled = "work pack not enabled; verification failed: %v"
	errReadSpecKey = "read spec file"
)

// Base fields are owned by the kernel for every object. A kind spec that
// re-declares one of these as its own field is redundant with the base object
// contract and must be removed instead of re-specified per kind.
var baseFields = map[string]struct{}{
	"id":          {},
	"ontology":    {},
	"plane":       {},
	"status":      {},
	"created_at":  {},
	"updated_at":  {},
	"archived_at": {},
	"archived_by": {},
}

// Structural top-level spec keys that are not object fields.
var specMetaKeys = map[string]struct{}{
	"ontology":                 {},
	"plane":                    {},
	"status":                   {},
	"lifecycle":                {},
	"inherits":                 {},
	"description":              {},
	"summary":                  {},
	"fields":                   {},
	"completeness_validation":  {},
	"change_log":               {},
	"artifacts":                {},
	"context":                  {},
}

var (
	camelAcronymRe = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	camelBoundaryRe = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	multiSepRe     = regexp.MustCompile(`[_\-]{2,}`)
)

// NormalizeField reduces a field name to its canonical snake_case form so that
// redundant declarations differing only in casing or separator style compare equal.
func NormalizeField(name string) string {
	s := camelAcronymRe.ReplaceAllString(name, "${1}_${2}")
	s = camelBoundaryRe.ReplaceAllString(s, "${1}_${2}")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "_")
	s = multiSepRe.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

// BaseFieldSet returns the canonical set of kernel-owned base fields.
func BaseFieldSet() map[string]bool {
	out := make(map[string]bool, len(baseFields))
	for f := range baseFields {
		out[f] = true
	}
	return out
}

// RedundancyReport is the result of auditing one object spec.
// Redundant lists canonical field names that duplicate a kernel base field;
// Duplicates lists canonical names declared more than once in the same spec.
type RedundancyReport struct {
	Path       string
	Redundant  []string
	Duplicates []string
}

// Clean reports whether the spec declares no redundant or duplicated fields.
func (r *RedundancyReport) Clean() bool {
	return len(r.Redundant) == 0 && len(r.Duplicates) == 0
}

// String renders a stable, deterministic summary.
func (r *RedundancyReport) String() string {
	if r.Clean() {
		return fmt.Sprintf(msgSpecClean, r.Path)
	}
	return fmt.Sprintf(msgSpecViolate, r.Path,
		strings.Join(sortedCopy(r.Redundant), " "), strings.Join(sortedCopy(r.Duplicates), " "))
}

// sortedCopy returns a sorted copy of in to keep report output deterministic.
func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// unique appends v to out unless it is already present.
func unique(out []string, v string) []string {
	for _, e := range out {
		if e == v {
			return out
		}
	}
	return append(out, v)
}

// collectFieldNames extracts declared field names from a spec document.
// It prefers the spec's explicit "fields" mapping; when absent it falls back to
// the top-level keys that carry field-definition values.
func collectFieldNames(doc map[string]any) []string {
	names := []string{}
	if fields, ok := doc["fields"].(map[string]any); ok {
		for k := range fields {
			names = append(names, k)
		}
	} else {
		for k, v := range doc {
			if _, meta := specMetaKeys[NormalizeField(k)]; meta {
				continue
			}
			if _, isMap := v.(map[string]any); !isMap {
				continue
			}
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return names
}

// AuditSpec audits one decoded spec document for redundant and duplicated field
// declarations.
func AuditSpec(doc map[string]any) *RedundancyReport {
	report := &RedundancyReport{}
	canonical := map[string]int{}
	for _, raw := range collectFieldNames(doc) {
		canon := NormalizeField(raw)
		if canon == "" {
			continue
		}
		canonical[canon]++
	}
	for canon, count := range canonical {
		if count > 1 {
			report.Duplicates = unique(report.Duplicates, canon)
		}
		if _, isBase := baseFields[canon]; isBase {
			report.Redundant = unique(report.Redundant, canon)
		}
	}
	return report
}

var specEntryRe = regexp.MustCompile(`^( *)([A-Za-z0-9_\-]+):\s*(.*)$`)

// AuditSpecFile loads a YAML object-spec file and audits it for redundant or
// duplicated field declarations.
func AuditSpecFile(path string) (*RedundancyReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", errReadSpecKey, path, err)
	}
	report := AuditSpec(decodeSpecDocument(data))
	report.Path = path
	return report, nil
}

type specScope struct {
	indent int
	keys   []string
}

// decodeSpecDocument parses the YAML subset used by kernel object specs:
// scalar values, nested mappings, and list markers. Anchors and multi-line
// scalars are not part of the spec grammar and are skipped.
func decodeSpecDocument(data []byte) map[string]any {
	root := map[string]any{}
	var scope specScope
	for _, raw := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] == '#' {
			continue
		}
		if trimmed[0] == '-' {
			continue // list item: not a field key
		}
		indent := 0
		for raw[indent] == ' ' {
			indent++
		}
		content := raw[indent:]
		m := specEntryRe.FindSubmatch(content)
		if m == nil {
			continue
		}
		key := string(m[2])
		value := bytes.TrimSpace(m[3])

		// Pop scopes at the same or deeper indentation.
		for len(scope.keys) > 0 && scope.indent >= indent {
			scope.keys = scope.keys[:len(scope.keys)-1]
		}
		if len(scope.keys) > 0 && scope.indent >= indent {
			scope.keys = scope.keys[:0]
		}

		parent := lookupScope(root, scope.keys)
		if len(value) == 0 {
			child, ok := parent[key].(map[string]any)
			if !ok {
				child = map[string]any{}
				parent[key] = child
			}
			scope.keys = append(scope.keys, key)
			scope.indent = indent
			continue
		}
		parent[key] = parseScalar(string(value))
		scope = specScope{}
	}
	return root
}

// lookupScope descends into the recorded scope keys and returns the innermost
// mapping a new key belongs to.
func lookupScope(root map[string]any, keys []string) map[string]any {
	cur := root
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			break
		}
		cur = next
	}
	return cur
}

// parseScalar converts a YAML scalar token into a typed Go value.
func parseScalar(value string) any {
	value = strings.Trim(value, `"'`)
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	if n, err := strconv.Atoi(value); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	return value
}

// AuditAll audits every verified spec file in this pack and returns a combined
// report. It is the hardening gate for object-spec redundancy in this pack.
func AuditAll() (*RedundancyReport, error) {
	if !Enabled() {
		return nil, fmt.Errorf(msgPackEnabled, VerifyError())
	}
	combined := &RedundancyReport{Path: SpecDir}
	paths := VerifiedPaths()
	for kind := range paths {
		if kind == "" {
			continue
		}
		report, err := AuditSpecFile(paths[kind])
		if err != nil {
			return nil, err
		}
		combined.Redundant = append(combined.Redundant, report.Redundant...)
		combined.Duplicates = append(combined.Duplicates, report.Duplicates...)
	}
	combined.Redundant = uniqueMany(combined.Redundant)
	combined.Duplicates = uniqueMany(combined.Duplicates)
	return combined, nil
}

// uniqueMany sorts and de-duplicates a list for stable combined reports.
func uniqueMany(in []string) []string {
	out := sortedCopy(in)
	for i := 1; i < len(out); {
		if out[i] == out[i-1] {
			out = append(out[:i], out[i+1:]...)
			continue
		}
		i++
	}
	return out
}

// VerifiedPaths returns a copy of the spec paths recorded at Enable time.
func VerifiedPaths() map[string]string {
	if verifiedSpecs == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(verifiedSpecs))
	for k, v := range verifiedSpecs {
		out[k] = v
	}
	return out
}
