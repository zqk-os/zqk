// Extracted from reverse_reference_index.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

func GetReferencedObjectIDs(obj map[string]any) []string {
	return extractReferenceIDsFromObject(obj)
}

// extractReferenceIDsFromObject extracts all referenced object IDs from an object
// Returns a slice of object IDs that this object references
func extractReferenceIDsFromObject(obj map[string]any) []string {
	if obj == nil {
		return nil
	}
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(obj)
	// Attribution / keystore identity fields (not *_ref suffix) — TRACK
	for _, fieldName := range []string{objects.FieldKeyCreatedBy, objects.FieldKeyUpdatedBy, objects.FieldKeyAccountID} {
		if v, ok := obj[fieldName]; ok && v != nil {
			refFields[fieldName] = v
		}
	}
	var refIDs []string
	for fieldName, refValue := range refFields {
		if refValue == nil {
			continue
		}
		// Skip leftover non-kernel *_ref names (git hashes, paths, tokens).
		if objects.IsLeftoverNonKernelRefField(fieldName) {
			continue
		}
		// Extract reference IDs from various formats
		switch v := refValue.(type) {
		case string:
			if v != emptyValue {
				refIDs = append(refIDs, extractObjectIDFromReference(v))
			}
		case []any:
			for _, item := range v {
				if str, ok := item.(string); ok && str != emptyValue {
					refIDs = append(refIDs, extractObjectIDFromReference(str))
				}
			}
		case []string:
			for _, str := range v {
				if str != emptyValue {
					refIDs = append(refIDs, extractObjectIDFromReference(str))
				}
			}
		}
	}
	// Remove duplicates and empty strings
	seen := make(map[string]bool)
	result := make([]string, 0, len(refIDs))
	for _, id := range refIDs {
		if id != emptyValue && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// extractObjectIDFromReference extracts the object ID from a reference string
// Handles various formats: "BLI-001", "backlog_item:BLI-001", "domain:process:backlog_item:BLI-001", "account:username"
func extractObjectIDFromReference(refStr string) string {
	if refStr == emptyValue {
		return ""
	}
	// Parse namespace format (e.g., "domain:process:backlog_item:BLI-002")
	parsed := validation.ParseNamespace(refStr)
	if parsed != nil && parsed.ObjectID != emptyValue {
		return parsed.ObjectID
	}
	// Handle "kind:id" format (e.g., "backlog_item:BLI-002")
	if strings.Contains(refStr, ":") {
		parts := strings.SplitN(refStr, ":", 2)
		if len(parts) == 2 {
			// Check if it's an account reference (keep full format)
			if parts[0] == objects.KindAccount {
				return refStr // Keep "account:username" format
			}
			return parts[1] // Return just the ID part
		}
		// Full namespace format (e.g., "domain:process:backlog_item:BLI-002")
		parts = strings.Split(refStr, ":")
		if len(parts) > 0 {
			return parts[len(parts)-1] // Last part is the ID
		}
	}
	// Simple format (e.g., "BLI-002")
	return refStr
}

func shouldIndexInReverseReferenceIndex(objectID string, obj map[string]any) bool {
	if objectID == emptyValue {
		return false
	}
	if strings.HasPrefix(objectID, "CHA-") || strings.HasPrefix(objectID, "AUD-") || strings.HasPrefix(objectID, "ATE-") {
		return false
	}
	if obj != nil {
		kind := objects.GetString(obj, objects.FieldKeyKind)
		if kind == objects.KindChangeJournalEntry || kind == objects.KindAuditEvent || kind == "agent_task_event" || objects.IsBypassKind(kind) {
			return false
		}
	}
	return true
}

// updateReverseReferenceIndexOnCreate updates the reverse reference index when an object is created
func updateReverseReferenceIndexOnCreate(objectID string, obj map[string]any) {
	if !shouldIndexInReverseReferenceIndex(objectID, obj) || obj == nil {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	refIDs := extractReferenceIDsFromObject(obj)
	for _, refID := range refIDs {
		if refID != emptyValue {
			index.AddReference(objectID, refID)
		}
	}
	afterReverseReferenceIndexMutation()
}

// updateReverseReferenceIndexOnUpdate updates the reverse reference index when an object is updated
func updateReverseReferenceIndexOnUpdate(objectID string, oldObj, newObj map[string]any) {
	if !shouldIndexInReverseReferenceIndex(objectID, newObj) {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	var oldRefIDs []string
	if oldObj != nil {
		oldRefIDs = extractReferenceIDsFromObject(oldObj)
	}
	var newRefIDs []string
	if newObj != nil {
		newRefIDs = extractReferenceIDsFromObject(newObj)
	}
	index.UpdateReferences(objectID, oldRefIDs, newRefIDs)
	afterReverseReferenceIndexMutation()
}

// updateReverseReferenceIndexOnDelete updates the reverse reference index when an object is deleted
func updateReverseReferenceIndexOnDelete(objectID string) {
	if objectID == emptyValue {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	index.RemoveObject(objectID)
	afterReverseReferenceIndexMutation()
}

// updateReverseReferenceIndexOnIDChange updates the reverse reference index when an object's ID changes
func updateReverseReferenceIndexOnIDChange(oldID, newID string, obj map[string]any) {
	if oldID == emptyValue || newID == emptyValue || oldID == newID {
		return
	}
	if !shouldIndexInReverseReferenceIndex(newID, obj) {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	// Remove old ID from all dependent lists
	index.RemoveObject(oldID)
	// Add new ID with same references
	refIDs := extractReferenceIDsFromObject(obj)
	for _, refID := range refIDs {
		if refID != emptyValue {
			index.AddReference(newID, refID)
		}
	}
	afterReverseReferenceIndexMutation()
}
