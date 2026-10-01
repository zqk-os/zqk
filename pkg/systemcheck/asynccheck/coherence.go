package asynccheck

import (
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

const (
	CategoryCacheLag       = "CacheLag"
	CategoryCacheCoherence = "cache_coherence"
	CategoryGhostRef       = validation.CategoryGhostRef
)

// IsTransientCacheCoherenceIssue reports whether an issue describes cache state at validation
// time rather than anything permanently wrong with the object.
func IsTransientCacheCoherenceIssue(category string) bool {
	return category == CategoryCacheLag || category == CategoryCacheCoherence
}

// InferKindFromID returns the schema kind derived from the object identifier pattern.
func InferKindFromID(id string) string {
	idValidator := validation.GetIDValidator()
	if err := idValidator.LoadPatterns(); err != nil {
		// Patterns may already be loaded or defaults apply; continue
		return idValidator.InferKindFromID(id)
	}
	return idValidator.InferKindFromID(id)
}

// ShouldUseCachedState returns true if the cached validation state is valid to use as a cache hit.
func ShouldUseCachedState(objectID, filePath string, state *validation.ValidationState, bypassCacheWithIssues bool) bool {
	if state == nil {
		return false
	}
	if bypassCacheWithIssues && len(state.Issues) > 0 {
		return false
	}
	if !isKindAndPathConsistent(objectID, filePath, state) {
		return false
	}
	info, err := fileutil.Stat(filePath)
	if err != nil || info.ModTime().After(state.LastValidated) {
		return false
	}
	return hasNoInvalidatingIssues(state.Issues)
}

func isKindAndPathConsistent(objectID, filePath string, state *validation.ValidationState) bool {
	effectiveKind := InferKindFromID(objectID)
	if effectiveKind != "" && state.ObjectKind != effectiveKind {
		return false
	}
	if state.FilePath != "" && filePath != "" && filepath.Clean(state.FilePath) != filepath.Clean(filePath) {
		return false
	}
	return true
}

func hasNoInvalidatingIssues(issues []validation.ValidationIssue) bool {
	for _, issue := range issues {
		if isInvalidatingIssue(issue) {
			return false
		}
	}
	return true
}

func isInvalidatingIssue(issue validation.ValidationIssue) bool {
	if issue.Category == validation.CategoryIntegrity {
		return true
	}
	if issue.Tier == 1 {
		if issue.Category == validation.CategoryInstanceValidation ||
			issue.Category == validation.CategoryValidationError ||
			issue.Category == validation.CategoryValidationTimeout {
			return true
		}
		if strings.Contains(issue.Message, "validation timeout after") {
			return true
		}
		if issue.Category == CategoryGhostRef || strings.EqualFold(issue.Category, CategoryGhostRef) {
			return true
		}
	}
	if IsTransientCacheCoherenceIssue(issue.Category) {
		return true
	}
	return false
}
