package memgraph

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
)

func TestNewMemGraphProvider(t *testing.T) {
	t.Parallel()
	config := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		Username: "admin",
		Password: "password",
		Database: "test",
		PoolSize: 10,
	}

	mgProvider := NewMemGraphProvider(&config)
	if mgProvider == nil {
		t.Fatal("NewMemGraphProvider returned nil")
	}

	if mgProvider.config.Host != "localhost" {
		t.Errorf("Expected Host to be 'localhost', got %s", mgProvider.config.Host)
	}
	if mgProvider.config.Port != 7687 {
		t.Errorf("Expected Port to be 7687, got %d", mgProvider.config.Port)
	}
}

func TestMemGraphProvider_SupportsFeature(t *testing.T) {
	t.Parallel()
	mgProvider := NewMemGraphProvider(&MemGraphConfig{})

	tests := []struct {
		name     string
		feature  provider.Feature
		expected bool
	}{
		{"CypherQuery", provider.FeatureCypherQuery, true},
		{"VectorSearch", provider.FeatureVectorSearch, true},
		{"MultiHopTraversal", provider.FeatureMultiHopTraversal, true},
		{"Transactions", provider.FeatureTransactions, true},
		{"BatchOperations", provider.FeatureBatchOperations, true},
		{"SPARQLQuery", provider.FeatureSPARQLQuery, false},
		{"StreamingQueries", provider.FeatureStreamingQueries, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mgProvider.SupportsFeature(tt.feature)
			if result != tt.expected {
				t.Errorf("SupportsFeature(%s) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestMemGraphProvider_GetCapabilities(t *testing.T) {
	t.Parallel()
	mgProvider := NewMemGraphProvider(&MemGraphConfig{})
	capabilities := mgProvider.GetCapabilities()

	if !capabilities.TransactionSupport {
		t.Error("Expected TransactionSupport to be true")
	}
	if !capabilities.VectorSearch {
		t.Error("Expected VectorSearch to be true")
	}
	if !capabilities.MultiHopTraversal {
		t.Error("Expected MultiHopTraversal to be true")
	}
	if !capabilities.BatchOperations {
		t.Error("Expected BatchOperations to be true")
	}
	if capabilities.StreamingQueries {
		t.Error("Expected StreamingQueries to be false")
	}

	// Check query languages
	foundCypher := false
	for _, lang := range capabilities.QueryLanguages {
		if lang == provider.QueryLanguageCypher {
			foundCypher = true
			break
		}
	}
	if !foundCypher {
		t.Error("Expected Cypher to be in QueryLanguages")
	}
}

func TestMemGraphProvider_GetProviderInfo(t *testing.T) {
	t.Parallel()
	mgProvider := NewMemGraphProvider(&MemGraphConfig{})
	info := mgProvider.GetProviderInfo()

	if info.Name != "MemGraph" {
		t.Errorf("Expected Name to be 'MemGraph', got %s", info.Name)
	}
	if info.Version != "1.0.0" {
		t.Errorf("Expected Version to be '1.0.0', got %s", info.Version)
	}
	if info.Description == emptyValue {
		t.Error("Expected Description to be non-empty")
	}
}

func TestMemGraphProvider_CreatePool(t *testing.T) {
	t.Parallel()
	mgProvider := NewMemGraphProvider(&MemGraphConfig{})

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	config := provider.ConnectionConfig{
		Host:     "localhost",
		Port:     7687,
		Username: "admin",
		Password: "password",
		Database: "test",
		MaxConns: 5,
	}

	pool, err := mgProvider.CreatePool(ctx, config)
	if err != nil {
		t.Fatalf("CreatePool failed: %v", err)
	}
	defer pool.Close()

	if pool == nil {
		t.Fatal("CreatePool returned nil pool")
	}

	// Verify pool stats
	stats := pool.Stats()
	if stats.MaxSize != 5 {
		t.Errorf("Expected MaxSize to be 5, got %d", stats.MaxSize)
	}
}

func TestMemGraphProvider_CreatePool_DefaultPoolSize(t *testing.T) {
	t.Parallel()
	mgProvider := NewMemGraphProvider(&MemGraphConfig{})

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	config := provider.ConnectionConfig{
		Host: "localhost",
		Port: 7687,
		// MaxConns not set, should default to 10
	}

	pool, err := mgProvider.CreatePool(ctx, config)
	if err != nil {
		t.Fatalf("CreatePool failed: %v", err)
	}
	defer pool.Close()

	stats := pool.Stats()
	if stats.MaxSize != 10 {
		t.Errorf("Expected default MaxSize to be 10, got %d", stats.MaxSize)
	}
}
