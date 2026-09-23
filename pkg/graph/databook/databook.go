package databook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// W3C Holon context and vocabulary constants.
const (
	W3CHolonContextURL    = "https://www.w3.org/ns/holon/v1#"
	W3CActivityStreamsURL = "https://www.w3.org/ns/activitystreams"
	ZKNamespaceURL        = "https://zqk.os/ontology#"
	DataBookType          = "DataBook"
	HolonType             = "holon:Holon"
)

// DataBook represents a W3C Holon federation and serialization package.
type DataBook struct {
	Context     any     `json:"@context"`
	Type        string  `json:"@type"`
	ID          string  `json:"id"`
	GeneratedAt string  `json:"generated_at"`
	Generator   string  `json:"generator"`
	Holons      []Holon `json:"holons"`
}

// Holon represents an individual part-whole entity within the knowledge graph.
type Holon struct {
	AtID        string         `json:"@id"`
	AtType      []string       `json:"@type"`
	ID          string         `json:"holon:id"`
	Title       string         `json:"holon:title,omitempty"`
	Status      string         `json:"holon:status,omitempty"`
	Kind        string         `json:"holon:kind"`
	Parts       []string       `json:"holon:parts,omitempty"`
	Wholes      []string       `json:"holon:wholes,omitempty"`
	Invariants  []string       `json:"holon:invariants,omitempty"`
	Properties  map[string]any `json:"holon:properties,omitempty"`
}

// BuildDataBookFromObjects converts a slice of knowledge kernel objects into a W3C Holon DataBook.
func BuildDataBookFromObjects(objs []map[string]any, generator string) (*DataBook, error) {
	if generator == "" {
		generator = "zqk-kernel"
	}

	dbID := fmt.Sprintf("urn:zqk:databook:%d", time.Now().UnixNano())
	db := &DataBook{
		Context: []any{
			W3CActivityStreamsURL,
			W3CHolonContextURL,
			map[string]any{
				"holon": W3CHolonContextURL,
				"zqk":   ZKNamespaceURL,
			},
		},
		Type:        DataBookType,
		ID:          dbID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Generator:   generator,
		Holons:      make([]Holon, 0, len(objs)),
	}

	// Index whole/part relationships
	// For each object, extract referenced parents (wholes) and child relations (parts)
	for _, raw := range objs {
		id, _ := raw[objects.FieldKeyID].(string)
		if id == "" {
			continue
		}
		kind, _ := raw[objects.FieldKeyKind].(string)
		title, _ := raw[objects.FieldKeyTitle].(string)
		status, _ := raw[objects.FieldKeyStatus].(string)

		holonURN := fmt.Sprintf("urn:zqk:object:%s:%s", kind, id)
		classType := "zqk:" + toCamelCase(kind)

		var wholes []string
		var parts []string
		var invariants []string
		properties := make(map[string]any)

		for k, v := range raw {
			if k == objects.FieldKeyID || k == objects.FieldKeyKind || k == objects.FieldKeyTitle || k == objects.FieldKeyStatus {
				continue
			}

			// Identify parent/whole references
			if strings.HasSuffix(k, "_refs") || strings.HasSuffix(k, "_ref") {
				refs := extractStringList(v)
				if isParentRef(k) {
					for _, r := range refs {
						wholes = append(wholes, fmt.Sprintf("urn:zqk:object:ref:%s", r))
					}
				} else {
					for _, r := range refs {
						parts = append(parts, fmt.Sprintf("urn:zqk:object:ref:%s", r))
					}
				}
				continue
			}

			// Identify invariants (acceptance considerations, problem statement, completion criteria)
			if k == "acceptance_considerations" || k == "completion_criteria" || k == "problem_statement" {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					invariants = append(invariants, s)
				}
				continue
			}

			properties[k] = v
		}

		sort.Strings(wholes)
		sort.Strings(parts)
		sort.Strings(invariants)

		h := Holon{
			AtID:       holonURN,
			AtType:     []string{HolonType, classType},
			ID:         id,
			Title:      title,
			Status:     status,
			Kind:       kind,
			Parts:      parts,
			Wholes:     wholes,
			Invariants: invariants,
			Properties: properties,
		}
		db.Holons = append(db.Holons, h)
	}

	sort.Slice(db.Holons, func(i, j int) bool {
		return db.Holons[i].ID < db.Holons[j].ID
	})

	return db, nil
}

// ExportToJSONLD serializes the DataBook into formatted JSON-LD.
func ExportToJSONLD(db *DataBook) ([]byte, error) {
	return json.MarshalIndent(db, "", "  ")
}

// ExportToTurtle serializes the DataBook into W3C RDF Turtle format.
func ExportToTurtle(db *DataBook) string {
	var buf bytes.Buffer
	buf.WriteString("@prefix holon: <https://www.w3.org/ns/holon/v1#> .\n")
	buf.WriteString("@prefix zqk: <https://zqk.os/ontology#> .\n")
	buf.WriteString("@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n")
	buf.WriteString("@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .\n\n")

	// Emit DataBook container
	buf.WriteString(fmt.Sprintf("<%s> a holon:DataBook ;\n", db.ID))
	buf.WriteString(fmt.Sprintf("    holon:generatedAt %q^^xsd:dateTime ;\n", db.GeneratedAt))
	buf.WriteString(fmt.Sprintf("    holon:generator %q .\n\n", db.Generator))

	// Emit each Holon
	for _, h := range db.Holons {
		typeStr := strings.Join(h.AtType, ", ")
		buf.WriteString(fmt.Sprintf("<%s> a %s ;\n", h.AtID, typeStr))
		buf.WriteString(fmt.Sprintf("    holon:id %q ;\n", h.ID))
		buf.WriteString(fmt.Sprintf("    holon:kind %q ;\n", h.Kind))
		if h.Title != "" {
			buf.WriteString(fmt.Sprintf("    rdfs:label %q ;\n", escapeLiteral(h.Title)))
			buf.WriteString(fmt.Sprintf("    holon:title %q ;\n", escapeLiteral(h.Title)))
		}
		if h.Status != "" {
			buf.WriteString(fmt.Sprintf("    holon:status %q ;\n", h.Status))
		}

		for _, p := range h.Parts {
			buf.WriteString(fmt.Sprintf("    holon:part <%s> ;\n", p))
		}
		for _, w := range h.Wholes {
			buf.WriteString(fmt.Sprintf("    holon:whole <%s> ;\n", w))
		}
		for _, inv := range h.Invariants {
			buf.WriteString(fmt.Sprintf("    holon:invariant %q ;\n", escapeLiteral(inv)))
		}
		buf.WriteString("    holon:memberOf <" + db.ID + "> .\n\n")
	}

	return buf.String()
}

func isParentRef(key string) bool {
	switch key {
	case "goal_refs", "goal_ref", "milestone_refs", "milestone_ref", "requirement_refs", "requirement_ref", "priority_plan_ref", "workstream_refs", "workstream_ref", "epic_refs", "epic_ref":
		return true
	default:
		return false
	}
}

func extractStringList(v any) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		var res []string
		for _, item := range val {
			if s, ok := item.(string); ok && s != "" {
				res = append(res, s)
			}
		}
		return res
	case string:
		if val != "" {
			return []string{val}
		}
	}
	return nil
}

func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func escapeLiteral(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
