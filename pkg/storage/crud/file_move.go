package crud

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/validation"
)

func ObjectFieldReferencesID(fieldValue any, oldID, kind string) bool {
	switch v := fieldValue.(type) {
	case string:
		return ReferenceMatches(v, oldID, kind)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && ReferenceMatches(s, oldID, kind) {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if ReferenceMatches(s, oldID, kind) {
				return true
			}
		}
	}
	return false
}

func BuildReferenceWithNewID(oldRef, oldID, newID string) string {
	parsed := validation.ParseNamespace(oldRef)
	if parsed != nil && parsed.ObjectID == oldID {
		if parsed.Layer != emptyValue && parsed.Domain != emptyValue {
			return fmt.Sprintf("%s:%s:%s:%s", parsed.Layer, parsed.Domain, parsed.ObjectType, newID)
		}
		if parsed.Layer != emptyValue {
			return fmt.Sprintf("%s:%s:%s", parsed.Layer, parsed.ObjectType, newID)
		}
		if parsed.ObjectType != emptyValue {
			return fmt.Sprintf("%s:%s", parsed.ObjectType, newID)
		}
		return newID
	}
	if strings.Contains(oldRef, ":") {
		parts := strings.SplitN(oldRef, ":", 2)
		if len(parts) == 2 && parts[1] == oldID {
			return fmt.Sprintf("%s:%s", parts[0], newID)
		}
	}
	if oldRef == oldID {
		return newID
	}
	return oldRef
}

func ReferenceMatches(refStr, objectID, kind string) bool {
	parsed := validation.ParseNamespace(refStr)
	if parsed != nil && parsed.ObjectID == objectID {
		if parsed.ObjectType == kind || parsed.ObjectType == emptyValue {
			return true
		}
	}

	if strings.Contains(refStr, ":") {
		parts := strings.SplitN(refStr, ":", 2)
		if len(parts) == 2 && parts[0] == kind && parts[1] == objectID {
			return true
		}
	}

	return refStr == objectID
}

func BuildNewReference(oldRef, objectID, newKind string) string {
	parsed := validation.ParseNamespace(oldRef)
	if parsed != nil {
		if parsed.Layer != emptyValue && parsed.Domain != emptyValue {
			return fmt.Sprintf("%s:%s:%s:%s", parsed.Layer, parsed.Domain, newKind, objectID)
		}
		if parsed.Layer != emptyValue {
			return fmt.Sprintf("%s:%s:%s", parsed.Layer, newKind, objectID)
		}
	}

	return fmt.Sprintf("%s:%s", newKind, objectID)
}
