package migration

import (
	"regexp"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/storage/id_generation"
	"github.com/lanceman/zqk/pkg/validation"
)

var (
	reUUIDMigrationPrefixFromID = regexp.MustCompile(`^([A-Z]+(?:-[A-Z]+)*)-(\d+)$`)
)

// uuidMigrationHelper provides helper functions for UUID-based ID migrations
type uuidMigrationHelper struct {
	storageProvider storage.ObjectStorageProvider
	projectRoot     string
	logger          logging.Logger
	idValidator     *validation.IDValidator
	uuidStrategy    *id_generation.UUIDStrategy
}

// newUUIDMigrationHelper creates a new UUID migration helper
func newUUIDMigrationHelper(storageProvider storage.ObjectStorageProvider, projectRoot string, logger logging.Logger) (*uuidMigrationHelper, error) {
	idValidator := validation.GetIDValidator()
	if err := idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf("failed to load ID patterns").Wrap(err)
	}

	// Create UUID strategy with 8-character length for readability
	uuidStrategy := id_generation.NewUUIDStrategyWithParams(map[string]any{
		"length": 8,
	})

	return &uuidMigrationHelper{
		storageProvider: storageProvider,
		projectRoot:     projectRoot,
		logger:          logger,
		idValidator:     idValidator,
		uuidStrategy:    uuidStrategy,
	}, nil
}

// extractPrefixFromID extracts the prefix from an ID (e.g., "ITEM-001" -> "BLI", "AGENT-ARCH-001" -> "AGENT-ARCH")
func (h *uuidMigrationHelper) extractPrefixFromID(id string) (string, error) {
	// Pattern: PREFIX-NUMBERS (e.g., ITEM-001, GOAL-123, AGENT-ARCH-001)
	// Extract everything before the last dash and number sequence
	matches := reUUIDMigrationPrefixFromID.FindStringSubmatch(id)
	if len(matches) >= 2 {
		return matches[1], nil
	}

	// Fallback: try simple split on last dash
	parts := strings.Split(id, "-")
	if len(parts) >= 2 {
		// Check if last part is numeric
		lastPart := parts[len(parts)-1]
		if isAllDigits(lastPart) {
			// Take all parts except the last (which should be the number)
			return strings.Join(parts[:len(parts)-1], "-"), nil
		}
	}

	return "", errfmt.Errorf("cannot extract prefix from ID: %s (expected format: PREFIX-NUMBER)", id)
}

func isAllDigits(s string) bool {
	if s == emptyValue {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// transformToUUIDID transforms an object's ID from sequential format to UUID format
// Old format: PREFIX-001 (e.g., ITEM-001, GOAL-123, AGENT-ARCH-001)
// New format: PREFIX-{8-char-uuid} (e.g., ITEM-a1b2c3d4, GOAL-5e6f7a8b, AGENT-ARCH-9c0d1e2f)
func (h *uuidMigrationHelper) transformToUUIDID(obj map[string]any) (map[string]any, error) {
	// Create a copy to avoid modifying the original
	transformed := make(map[string]any)
	for k, v := range obj {
		transformed[k] = v
	}

	// Get old ID
	oldID, ok := obj[objects.FieldKeyID].(string)
	if !ok || oldID == emptyValue {
		return nil, errfmt.Errorf("object missing id field")
	}

	// Extract prefix from old ID (preserve multi-part prefixes like AGENT-ARCH)
	prefix, err := h.extractPrefixFromID(oldID)
	if err != nil {
		return nil, errfmt.Errorf("failed to extract prefix from ID %s: %w", oldID, err)
	}

	// Generate new UUID ID using UUID strategy directly with extracted prefix
	// We use the strategy directly to preserve the exact prefix from the old ID
	ctx := pkgctx.NewSystemContext()
	// UUID strategy doesn't need existingIDs for uniqueness (random hex)
	newID, err := h.uuidStrategy.GenerateNextID(ctx, "", prefix, nil)
	if err != nil {
		return nil, errfmt.Newf("failed to generate UUID ID").Wrap(err)
	}

	// Update the ID field
	transformed[objects.FieldKeyID] = newID

	// Get kind for logging
	kind := h.idValidator.InferKindFromID(oldID)
	logging.Fluent(h.logger).Debug("Transformed object ID to UUID format").
		String("old_id", oldID).
		String("new_id", newID).
		Kind(kind).
		String("prefix", prefix).
		Log()

	return transformed, nil
}
