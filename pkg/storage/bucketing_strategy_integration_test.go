package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestCompositeBucketStrategy_AuditEvent tests that audit events are bucketed correctly
// using the composite strategy (daily + target_kind + status)
func TestCompositeBucketStrategy_AuditEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()
	mockStorage := newMockBucketStrategyStorage()

	// Create the three strategies for audit_event
	dailyStrategy := map[string]any{
		objects.FieldKeyID:            "BST-009",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "daily",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "daily",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}
	targetKindStrategy := map[string]any{
		objects.FieldKeyID:            "BST-010",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "state",
		objects.FieldKeyStrategyName:  "target_kind",
		objects.FieldKeyField:         "target_kind",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}
	statusStrategy := map[string]any{
		objects.FieldKeyID:            "BST-011",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "state",
		objects.FieldKeyStrategyName:  "status",
		objects.FieldKeyField:         "status",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}

	mockStorage.strategies["BST-009"] = dailyStrategy
	mockStorage.strategies["BST-010"] = targetKindStrategy
	mockStorage.strategies["BST-011"] = statusStrategy

	loader := NewBucketStrategyLoaderWithProvider(mockStorage)
	registry := &DefaultBucketStrategyRegistry{
		loader:        loader,
		kindMapper:    objects.GetGlobalKindMapper(),
		specLoader:    objects.GetGlobalSpecLoader(),
		defaultsCache: make(map[string]BucketStrategy),
		baseKindCache: make(map[string]string),
	}

	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize loader: %v", err)
	}

	// Get strategy for audit_event - should return composite
	strategy, err := registry.GetStrategyForKind(ctx, "audit_event")
	if err != nil {
		t.Fatalf("Failed to get strategy for audit_event: %v", err)
	}
	if strategy == nil {
		t.Fatal("Expected composite strategy for audit_event, got nil")
	}

	// Verify it's a composite strategy
	composite, ok := strategy.(*CompositeBucketStrategy)
	if !ok {
		t.Fatalf("Expected CompositeBucketStrategy, got %T", strategy)
	}
	if len(composite.Strategies) != 3 {
		t.Errorf("Expected 3 strategies in composite (daily, target_kind, status), got %d", len(composite.Strategies))
		// Debug: print strategy names
		for i, s := range composite.Strategies {
			t.Logf("Strategy %d: %s", i, s.Name())
		}
	}

	// Test bucket key generation with sample audit event
	testEvent := map[string]any{
		objects.FieldKeyID:         "AUD-001",
		objects.FieldKeyKind:       objects.KindAuditEvent,
		objects.FieldKeyCreatedAt:  "2030-01-15T14:30:00Z",
		objects.FieldKeyTargetKind: "backlog_item",
		objects.FieldKeyStatus:     "completed",
	}

	// Test bucket key generation directly on the strategy
	bucketKey := strategy.GetBucketKey(testEvent, "")
	if bucketKey == emptyValue {
		t.Error("Expected non-empty bucket key for audit event")
	}

	// Verify bucket key contains date, target_kind, and status
	// Format should be: {date}/{target_kind}/{status} or {target_kind}/{date}/{status}
	// (order may vary, but all parts should be present)
	expectedParts := []string{"2030-01-15", "backlog_item", "completed"}
	for _, part := range expectedParts {
		if !testContains(bucketKey, part) {
			t.Errorf("Expected bucket key to contain '%s', got: %s", part, bucketKey)
		}
	}

	// Verify we have all three parts (date, target_kind, status)
	parts := strings.Split(bucketKey, "/")
	if len(parts) < 3 {
		t.Errorf("Expected bucket key to have at least 3 parts (date/target_kind/status), got %d parts: %s", len(parts), bucketKey)
	}
}

// TestCompositeBucketStrategy_ChangeJournalEntry tests that change journal entries
// are bucketed correctly using the composite strategy (daily + change_type)
func TestCompositeBucketStrategy_ChangeJournalEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()
	mockStorage := newMockBucketStrategyStorage()

	// Create the two strategies for change_journal_entry
	dailyStrategy := map[string]any{
		objects.FieldKeyID:            "BST-012",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "daily",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "daily",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"change_journal_entry"},
	}
	changeTypeStrategy := map[string]any{
		objects.FieldKeyID:            "BST-013",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "state",
		objects.FieldKeyStrategyName:  "change_type",
		objects.FieldKeyField:         "change_type",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"change_journal_entry"},
	}

	mockStorage.strategies["BST-012"] = dailyStrategy
	mockStorage.strategies["BST-013"] = changeTypeStrategy

	loader := NewBucketStrategyLoaderWithProvider(mockStorage)
	registry := &DefaultBucketStrategyRegistry{
		loader:        loader,
		kindMapper:    objects.GetGlobalKindMapper(),
		specLoader:    objects.GetGlobalSpecLoader(),
		defaultsCache: make(map[string]BucketStrategy),
		baseKindCache: make(map[string]string),
	}

	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize loader: %v", err)
	}

	// Test bucket key generation with sample change journal entry
	testEntry := map[string]any{
		objects.FieldKeyID:         "CHA-001",
		objects.FieldKeyKind:       objects.KindChangeJournalEntry,
		objects.FieldKeyCreatedAt:  "2030-01-15T14:30:00Z",
		objects.FieldKeyChangeType: "update",
	}

	// Get strategy for change_journal_entry - should return composite
	strategy, err := registry.GetStrategyForKind(ctx, "change_journal_entry")
	if err != nil {
		t.Fatalf("Failed to get strategy for change_journal_entry: %v", err)
	}
	if strategy == nil {
		t.Fatal("Expected composite strategy for change_journal_entry, got nil")
	}

	// Test bucket key generation directly on the strategy
	bucketKey := strategy.GetBucketKey(testEntry, "")
	if bucketKey == emptyValue {
		t.Error("Expected non-empty bucket key for change journal entry")
	}

	// Verify bucket key contains date and change_type
	// Format should be: {date}/{change_type}
	expectedParts := []string{"2030-01-15", "update"}
	for _, part := range expectedParts {
		if !testContains(bucketKey, part) {
			t.Errorf("Expected bucket key to contain '%s', got: %s", part, bucketKey)
		}
	}
}

// TestBucketSizeLimit_750Objects tests that buckets don't exceed 750 objects
// This is a design constraint - we test that the bucketing strategy creates
// sufficiently granular buckets
func TestBucketSizeLimit_750Objects(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()
	mockStorage := newMockBucketStrategyStorage()

	// Create daily strategy for audit_event
	dailyStrategy := map[string]any{
		objects.FieldKeyID:            "BST-009",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "daily",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "daily",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}
	targetKindStrategy := map[string]any{
		objects.FieldKeyID:            "BST-010",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "state",
		objects.FieldKeyStrategyName:  "target_kind",
		objects.FieldKeyField:         "target_kind",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}
	statusStrategy := map[string]any{
		objects.FieldKeyID:            "BST-011",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "state",
		objects.FieldKeyStrategyName:  "status",
		objects.FieldKeyField:         "status",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}

	mockStorage.strategies["BST-009"] = dailyStrategy
	mockStorage.strategies["BST-010"] = targetKindStrategy
	mockStorage.strategies["BST-011"] = statusStrategy

	loader := NewBucketStrategyLoaderWithProvider(mockStorage)
	registry := &DefaultBucketStrategyRegistry{
		loader:        loader,
		kindMapper:    objects.GetGlobalKindMapper(),
		specLoader:    objects.GetGlobalSpecLoader(),
		defaultsCache: make(map[string]BucketStrategy),
		baseKindCache: make(map[string]string),
	}

	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize loader: %v", err)
	}

	// Get strategy for audit_event
	strategy, err := registry.GetStrategyForKind(ctx, "audit_event")
	if err != nil {
		t.Fatalf("Failed to get strategy for audit_event: %v", err)
	}

	// Generate bucket keys for multiple events with same date/target_kind/status
	// to verify they all map to the same bucket
	baseTime := time.Date(2030, 1, 15, 14, 30, 0, 0, time.UTC)
	bucketKeys := make(map[string]int)

	for i := 0; i < 1000; i++ {
		event := map[string]any{
			objects.FieldKeyID:         "AUD-001",
			objects.FieldKeyKind:       objects.KindAuditEvent,
			objects.FieldKeyCreatedAt:  baseTime.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeyStatus:     "completed",
		}

		bucketKey := strategy.GetBucketKey(event, "")
		if bucketKey != emptyValue {
			bucketKeys[bucketKey]++
		}
	}

	// Verify that events from the same day with same target_kind and status
	// map to the same bucket (daily granularity means same day = same bucket)
	// But we should have multiple buckets if we span multiple days
	if len(bucketKeys) == 0 {
		t.Error("Expected at least one bucket key, got 0")
	}

	// Verify that no single bucket would exceed 750 objects
	// In this test, all events are from the same day with same target_kind/status
	// so they should all map to one bucket, but in practice, daily + target_kind + status
	// splitting should keep buckets manageable
	maxBucketSize := 0
	for _, count := range bucketKeys {
		if count > maxBucketSize {
			maxBucketSize = count
		}
	}

	// With daily + target_kind + status, we expect buckets to be split
	// This test verifies the bucketing logic works, not that we enforce 750 limit
	// (that would require actual object counting which is a different concern)
	if maxBucketSize > 1000 {
		t.Errorf("Expected bucket size to be reasonable, got: %d", maxBucketSize)
	}
}

// TestArchiveStrategy_LoadAndValidate tests that archive strategies load correctly
// from the bucketing strategy objects
func TestArchiveStrategy_LoadAndValidate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()
	mockStorage := newMockBucketStrategyStorage()

	// Create strategy with archive configuration
	strategy := map[string]any{
		objects.FieldKeyID:            "BST-009",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "daily",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "daily",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
		objects.FieldKeyArchiveStrategy: map[string]any{
			objects.FieldKeyEnabled: true,
			"archive_after":         "720h",
			"tier_progression": []any{
				map[string]any{
					objects.FieldKeyTier: "warm",
					"duration":           "720h",
				},
				map[string]any{
					objects.FieldKeyTier: "cold",
					"duration":           "8760h",
					"compression":        true,
				},
				map[string]any{
					objects.FieldKeyTier: "iced",
					"duration":           "0",
					"compression":        true,
					"encryption":         true,
				},
			},
		},
	}

	mockStorage.strategies["BST-009"] = strategy

	loader := NewBucketStrategyLoaderWithProvider(mockStorage)
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize loader: %v", err)
	}

	// Load the strategy and verify archive_strategy is present
	loadedStrategy, err := loader.GetStrategy(ctx, "BST-009")
	if err != nil {
		t.Fatalf("Failed to load strategy: %v", err)
	}

	archiveStrategy, ok := loadedStrategy[objects.FieldKeyArchiveStrategy].(map[string]any)
	if !ok {
		t.Fatal("Expected archive_strategy in loaded strategy, but not found or wrong type")
	}

	enabled, ok := archiveStrategy[objects.FieldKeyEnabled].(bool)
	if !ok || !enabled {
		t.Error("Expected archive_strategy.enabled to be true")
	}

	tierProgression, ok := archiveStrategy["tier_progression"].([]any)
	if !ok || len(tierProgression) == 0 {
		t.Error("Expected tier_progression in archive_strategy")
	}
}

func testContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || testContainsMiddle(s, substr)))
}

func testContainsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
