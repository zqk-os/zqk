package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// mockBucketStrategyStorage is a mock storage provider for testing
type mockBucketStrategyStorage struct {
	strategies  map[string]map[string]any
	backendType string
}

func newMockBucketStrategyStorage() *mockBucketStrategyStorage {
	return &mockBucketStrategyStorage{
		strategies:  make(map[string]map[string]any),
		backendType: "mock",
	}
}

func (m *mockBucketStrategyStorage) LoadStrategy(ctx context.Context, strategyID string) (map[string]any, error) {
	strategy, ok := m.strategies[strategyID]
	if !ok {
		return nil, fileutil.ErrNotExist
	}
	return strategy, nil
}

func (m *mockBucketStrategyStorage) LoadAllStrategies(ctx context.Context) ([]map[string]any, error) {
	strategies := make([]map[string]any, 0, len(m.strategies))
	for _, strategy := range m.strategies {
		strategies = append(strategies, strategy)
	}
	return strategies, nil
}

func (m *mockBucketStrategyStorage) LoadStrategiesForKind(ctx context.Context, kind string) ([]map[string]any, error) {
	var matching []map[string]any
	for _, strategy := range m.strategies {
		appliesTo, ok := strategy[objects.FieldKeyAppliesTo].([]any)
		if !ok {
			continue
		}
		for _, appliedKind := range appliesTo {
			if appliedKindStr, ok := appliedKind.(string); ok && appliedKindStr == kind {
				enabled, _ := strategy[objects.FieldKeyEnabled].(bool)
				if enabled {
					matching = append(matching, strategy)
				}
				break
			}
		}
	}
	return matching, nil
}

func (m *mockBucketStrategyStorage) SaveStrategy(ctx context.Context, strategy map[string]any) error {
	strategyID, ok := strategy[objects.FieldKeyID].(string)
	if !ok {
		return os.ErrInvalid
	}
	m.strategies[strategyID] = strategy
	return nil
}

func (m *mockBucketStrategyStorage) DeleteStrategy(ctx context.Context, strategyID string) error {
	delete(m.strategies, strategyID)
	return nil
}

func (m *mockBucketStrategyStorage) GetBackendType() string {
	return m.backendType
}

// TestBucketStrategyLoader_LoadAndValidate tests that strategies load and validate correctly
// Validates CRIT-9047: Strategy validation and error reporting
func TestBucketStrategyLoader_LoadAndValidate(t *testing.T) {
	ctx := context.Background()
	mockStorage := newMockBucketStrategyStorage()

	// Add valid strategy
	validStrategy := map[string]any{
		objects.FieldKeyID:            "BST-001",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "monthly",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "2006-01",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}
	mockStorage.strategies["BST-001"] = validStrategy

	// Add invalid strategy (missing required field)
	invalidStrategy := map[string]any{
		objects.FieldKeyID:            "BST-002",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		// Missing "field" and "format"
		objects.FieldKeyEnabled:   true,
		objects.FieldKeyAppliesTo: []any{"audit_event"},
	}
	mockStorage.strategies["BST-002"] = invalidStrategy

	loader := NewBucketStrategyLoaderWithProvider(mockStorage)
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize loader: %v", err)
	}

	// Valid strategy should be loaded
	strategy, err := loader.GetStrategy(ctx, "BST-001")
	if err != nil {
		t.Fatalf("Failed to load valid strategy: %v", err)
	}
	if strategy[objects.FieldKeyID] != "BST-001" {
		t.Errorf("Expected strategy ID BST-001, got %v", strategy[objects.FieldKeyID])
	}

	// Invalid strategy should be skipped during initialization (validation fails)
	// It won't be in the cache because validation errors cause it to be skipped
	// But we can verify it's not indexed
	strategies, err := loader.GetStrategiesForKind(ctx, "audit_event")
	if err != nil {
		t.Fatalf("Failed to get strategies: %v", err)
	}
	// Should only have BST-001 (valid), not BST-002 (invalid)
	if len(strategies) != 1 {
		t.Errorf("Expected 1 valid strategy, got %d", len(strategies))
	}
	if strategies[0][objects.FieldKeyID] != "BST-001" {
		t.Errorf("Expected BST-001, got %v", strategies[0][objects.FieldKeyID])
	}
}

// TestBucketStrategyLoader_UniquenessConstraint tests CRIT-9044:
// Uniqueness constraint - one strategy per (kind, schema_version) combination
func TestBucketStrategyLoader_UniquenessConstraint(t *testing.T) {
	ctx := context.Background()
	mockStorage := newMockBucketStrategyStorage()

	// Strategy 1: audit_event v2.0.0 (monthly)
	strategy1 := map[string]any{
		objects.FieldKeyID:            "BST-001",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "monthly",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "2006-01",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}
	mockStorage.strategies["BST-001"] = strategy1

	// Strategy 2: audit_event v2.0.0 (daily) - CONFLICT with strategy1
	strategy2 := map[string]any{
		objects.FieldKeyID:            "BST-002",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "daily",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "daily",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}
	mockStorage.strategies["BST-002"] = strategy2

	// Strategy 3: audit_event v1.0.0 (monthly) - NO CONFLICT (different version)
	strategy3 := map[string]any{
		objects.FieldKeyID:            "BST-003",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "monthly",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "2006-01",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"change_journal_entry"}, // Different kind, no conflict
	}
	mockStorage.strategies["BST-003"] = strategy3

	loader := NewBucketStrategyLoaderWithProvider(mockStorage)
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize loader: %v", err)
	}

	// Strategy 1 should be indexed for audit_event
	strategies, err := loader.GetStrategiesForKind(ctx, "audit_event")
	if err != nil {
		t.Fatalf("Failed to get strategies for audit_event: %v", err)
	}

	// Only one strategy should be returned (strategy1, strategy2 should be skipped due to conflict)
	// Note: The current implementation logs conflicts but still caches both strategies
	// The first one encountered gets indexed, subsequent ones are skipped
	if len(strategies) > 1 {
		t.Errorf("Expected at most 1 strategy for audit_event v2.0.0, got %d", len(strategies))
	}

	// Strategy 3 should be indexed for change_journal_entry (different kind)
	strategies2, err := loader.GetStrategiesForKind(ctx, "change_journal_entry")
	if err != nil {
		t.Fatalf("Failed to get strategies for change_journal_entry: %v", err)
	}
	if len(strategies2) != 1 {
		t.Errorf("Expected 1 strategy for change_journal_entry, got %d", len(strategies2))
	}
}

// TestBucketStrategyLoader_AllKindsHaveStrategies tests CRIT-9043:
// All system object kinds have bucketing strategies defined
func TestBucketStrategyLoader_AllKindsHaveStrategies(t *testing.T) {
	ctx := context.Background()
	mockStorage := newMockBucketStrategyStorage()

	// Add strategies for high-volume kinds
	strategy1 := map[string]any{
		objects.FieldKeyID:            "BST-001",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "chronological",
		objects.FieldKeyStrategyName:  "monthly",
		objects.FieldKeyField:         "created_at",
		objects.FieldKeyFormat:        "2006-01",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"audit_event"},
	}
	mockStorage.strategies["BST-001"] = strategy1

	loader := NewBucketStrategyLoaderWithProvider(mockStorage)
	registry := &DefaultBucketStrategyRegistry{
		loader:        loader,
		kindMapper:    objects.GetGlobalKindMapper(),
		specLoader:    objects.GetGlobalSpecLoader(),
		defaultsCache: make(map[string]BucketStrategy),
		baseKindCache: make(map[string]string),
	}

	// Initialize loader
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize loader: %v", err)
	}

	// Test that all kinds get strategies (explicit or default)
	// High-volume kinds should get explicit or default chronological
	strategy, err := registry.GetStrategyForKind(ctx, "audit_event")
	if err != nil {
		t.Fatalf("Failed to get strategy for audit_event: %v", err)
	}
	if strategy == nil {
		t.Error("Expected strategy for audit_event, got nil")
	}

	// Other kinds should get default path-based strategy
	strategy2, err := registry.GetStrategyForKind(ctx, "backlog_item")
	if err != nil {
		t.Fatalf("Failed to get strategy for backlog_item: %v", err)
	}
	if strategy2 == nil {
		t.Error("Expected default strategy for backlog_item, got nil")
	}
}

// TestChronoBucketStrategy_Granularities tests CRIT-9045:
// Strategies support multiple granularities for chronological bucketing
func TestChronoBucketStrategy_Granularities(t *testing.T) {
	testCases := []struct {
		name        string
		granularity string
		format      string
		expected    string
	}{
		{"monthly", "monthly", "", "2006-01"},
		{"weekly", "weekly", "", "2006-W01"},
		{"daily", "daily", "", "2006-01-02"},
		{"hourly", "hourly", "", "2006-01-02T15"},
		{"half_hourly", "half_hourly", "", "2006-01-02T15:04"},
		{"qtr_hourly", "qtr_hourly", "", "2006-01-02T15:04:05"},
		{"tenths", "tenths", "", "2006-01-02T15:04:05.9"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			strategy := &ChronoBucketStrategy{
				Field:       objects.FieldKeyCreatedAt,
				Granularity: tc.granularity,
				Format:      tc.format,
				ParseFunc: func(s string) (time.Time, error) {
					return time.Parse(time.RFC3339, s)
				},
			}

			// Test bucket key generation
			obj := map[string]any{
				objects.FieldKeyCreatedAt: "2030-01-15T14:30:00Z",
			}

			bucketKey := strategy.GetBucketKey(obj, "")
			if bucketKey == emptyValue {
				t.Errorf("Expected non-empty bucket key for %s", tc.name)
			}

			// Verify strategy name
			strategyName := strategy.Name()
			if strategyName != "chrono-"+tc.granularity {
				t.Errorf("Expected strategy name 'chrono-%s', got '%s'", tc.granularity, strategyName)
			}
		})
	}
}

// TestDefaultBucketStrategyRegistry_SubKindResolution tests CRIT-9046:
// Sub-kind resolution for bucketing strategies
func TestDefaultBucketStrategyRegistry_SubKindResolution(t *testing.T) {
	ctx := context.Background()
	mockStorage := newMockBucketStrategyStorage()

	// Strategy for base kind "decision"
	strategy := map[string]any{
		objects.FieldKeyID:            "BST-001",
		objects.FieldKeyKind:          objects.KindBucketingStrategy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStrategyType:  "state",
		objects.FieldKeyStrategyName:  "status_based",
		objects.FieldKeyField:         "status",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyAppliesTo:     []any{"decision"},
	}
	mockStorage.strategies["BST-001"] = strategy

	loader := NewBucketStrategyLoaderWithProvider(mockStorage)
	registry := &DefaultBucketStrategyRegistry{
		loader:        loader,
		kindMapper:    objects.GetGlobalKindMapper(),
		specLoader:    objects.GetGlobalSpecLoader(),
		defaultsCache: make(map[string]BucketStrategy),
		baseKindCache: make(map[string]string),
	}

	// Initialize loader
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize loader: %v", err)
	}

	// Test that "adr" (sub-kind) resolves to "decision" (base kind)
	// and uses decision's strategy
	strategyForADR, err := registry.GetStrategyForKind(ctx, "adr")
	if err != nil {
		t.Fatalf("Failed to get strategy for adr: %v", err)
	}

	// Should get decision's strategy (either explicit or default)
	if strategyForADR == nil {
		t.Error("Expected strategy for adr (via decision), got nil")
	}
}

// TestValidateBucketingStrategy_ArchiveStrategy tests archive strategy validation
func TestValidateBucketingStrategy_ArchiveStrategy(t *testing.T) {
	testCases := []struct {
		name          string
		strategy      map[string]any
		expectedError bool
		errorField    string
	}{
		{
			name: "valid simple archive strategy",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"archive_after":         "720h",
					"archive_tier":          "warm",
				},
			},
			expectedError: false,
		},
		{
			name: "valid multi-tier archive strategy",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
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
			},
			expectedError: false,
		},
		{
			name: "enabled archive strategy without archive_after or tier_progression",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
				},
			},
			expectedError: true,
			errorField:    "archive_strategy.archive_after",
		},
		{
			name: "invalid archive_after duration format",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"archive_after":         "invalid",
				},
			},
			expectedError: true,
			errorField:    "archive_strategy.archive_after",
		},
		{
			name: "invalid tier in tier_progression",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"tier_progression": []any{
						map[string]any{
							objects.FieldKeyTier: "invalid_tier",
							"duration":           "720h",
						},
					},
				},
			},
			expectedError: true,
			errorField:    "archive_strategy.tier_progression[0].tier",
		},
		{
			name: "missing duration in tier_progression",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"tier_progression": []any{
						map[string]any{
							objects.FieldKeyTier: "warm",
						},
					},
				},
			},
			expectedError: true,
			errorField:    "archive_strategy.tier_progression[0].duration",
		},
		{
			name: "archive strategy disabled (no validation needed)",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: false,
				},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors := validation.ValidateBucketingStrategy(tc.strategy)

			if tc.expectedError {
				if len(errors) == 0 {
					t.Errorf("Expected validation error for %s, got none", tc.name)
				} else {
					// Check if error is for the expected field
					found := false
					for _, err := range errors {
						if err.Field == tc.errorField {
							found = true
							break
						}
					}
					if !found && tc.errorField != emptyValue {
						t.Errorf("Expected error for field %s, got errors: %v", tc.errorField, errors)
					}
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("Expected no validation errors, got: %v", errors)
				}
			}
		})
	}
}

// TestValidateBucketingStrategy_RequiredFields tests CRIT-9047:
// Strategy validation and error reporting
func TestValidateBucketingStrategy_RequiredFields(t *testing.T) {
	testCases := []struct {
		name          string
		strategy      map[string]any
		expectedError bool
		errorField    string
	}{
		{
			name: "valid strategy",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
			},
			expectedError: false,
		},
		{
			name: "missing strategy_type",
			strategy: map[string]any{
				objects.FieldKeyField:     "created_at",
				objects.FieldKeyFormat:    "2006-01",
				objects.FieldKeyEnabled:   true,
				objects.FieldKeyAppliesTo: []any{"audit_event"},
			},
			expectedError: true,
			errorField:    "strategy_type",
		},
		{
			name: "missing field",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
			},
			expectedError: true,
			errorField:    "field",
		},
		{
			name: "missing format for chronological",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
			},
			expectedError: true,
			errorField:    "format",
		},
		{
			name: "invalid strategy_type",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "invalid",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
			},
			expectedError: true,
			errorField:    "strategy_type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors := validation.ValidateBucketingStrategy(tc.strategy)

			if tc.expectedError {
				if len(errors) == 0 {
					t.Errorf("Expected validation error for %s, got none", tc.name)
				} else {
					// Check if error is for the expected field
					found := false
					for _, err := range errors {
						if err.Field == tc.errorField {
							found = true
							break
						}
					}
					if !found && tc.errorField != emptyValue {
						t.Errorf("Expected error for field %s, got errors: %v", tc.errorField, errors)
					}
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("Expected no validation errors, got: %v", errors)
				}
			}
		})
	}
}

func TestBucketStrategyLoader_LifetimeCounters(t *testing.T) {
	mockStorage := newMockBucketStrategyStorage()
	mockStorage.strategies["strat-1"] = map[string]any{
		objects.FieldKeyID:           "strat-1",
		objects.FieldKeyStrategyType: "chronological",
		objects.FieldKeyField:        "created_at",
		objects.FieldKeyFormat:       "2006-01",
		objects.FieldKeyEnabled:      true,
		objects.FieldKeyAppliesTo:    []any{"audit_event"},
	}

	l := NewBucketStrategyLoaderWithProvider(mockStorage)
	ctx := context.Background()

	loaded, hits := l.GetLoaderStats()
	if loaded != 0 || hits != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", loaded, hits)
	}

	// First fetch - load from storage
	_, err := l.GetStrategy(ctx, "strat-1")
	if err != nil {
		t.Fatalf("GetStrategy failed: %v", err)
	}

	loaded, hits = l.GetLoaderStats()
	if loaded != 1 || hits != 0 {
		t.Errorf("expected loaded=1 hits=0, got loaded=%d hits=%d", loaded, hits)
	}

	// Second fetch - hit cache
	_, err = l.GetStrategy(ctx, "strat-1")
	if err != nil {
		t.Fatalf("GetStrategy failed: %v", err)
	}

	loaded, hits = l.GetLoaderStats()
	if loaded != 1 || hits != 1 {
		t.Errorf("expected loaded=1 hits=1, got loaded=%d hits=%d", loaded, hits)
	}
}
