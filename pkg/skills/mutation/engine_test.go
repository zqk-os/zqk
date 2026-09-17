package mutation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/graph/memgraph"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// MockLLMClient is a mock LLMClient for testing.
type MockLLMClient struct{}

func (m *MockLLMClient) GenerateCode(ctx context.Context, prompt string, parentCode ...string) (string, error) {
	if prompt == "error" {
		return "", fmt.Errorf("llm error")
	}
	return "func Mutated() { fmt.Println(\"Hello\") }", nil
}

func (m *MockLLMClient) GenerateDocs(ctx context.Context, code string) (string, error) {
	return "Mutated skill docs", nil
}

// MockSemanticEngine is a mock SemanticEngine for testing.
type MockSemanticEngine struct{}

func (m *MockSemanticEngine) ValidateCode(ctx context.Context, code string) (bool, error) {
	if code == "" {
		return false, fmt.Errorf("empty code")
	}
	return true, nil
}

func TestEngine_MutateSkill(t *testing.T) {
	// Only run this test if a MemGraph instance is available
	mgConfig := &memgraph.MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		Username: "",
		Password: "",
		PoolSize: 2,
	}

	mgProvider := memgraph.NewMemGraphProvider(mgConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connConfig := provider.ConnectionConfig{
		Host:     mgConfig.Host,
		Port:     mgConfig.Port,
		Username: mgConfig.Username,
		Password: mgConfig.Password,
		MaxConns: mgConfig.PoolSize,
	}
	pool, err := mgProvider.CreatePool(ctx, connConfig)
	if err != nil {
		t.Skipf("Failed to create memgraph pool: %v. Skipping test.", err)
	}
	defer pool.Close()

	// Try to get a connection to see if DB is really there
	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Skipf("Failed to connect to memgraph: %v. Skipping test.", err)
	}

	// Setup initial parent skills in the DB
	err = conn.CreateNode(ctx, provider.Node{
		ID:     "parent1",
		Labels: []string{"Skill"},
		Properties: map[string]any{
			objects.FieldKeyCode: "func Parent1() {}",
		},
	})
	if err != nil {
		_ = pool.ReturnConnection(conn)
		t.Skipf("Failed to create node: %v. Database might not be fully operational.", err)
	}

	err = conn.CreateNode(ctx, provider.Node{
		ID:     "parent2",
		Labels: []string{"Skill"},
		Properties: map[string]any{
			objects.FieldKeyCode: "func Parent2() {}",
		},
	})
	if err != nil {
		_ = pool.ReturnConnection(conn)
		t.Skipf("Failed to create node: %v. Database might not be fully operational.", err)
	}

	_ = pool.ReturnConnection(conn)

	engine := NewEngine(&MockLLMClient{}, &MockSemanticEngine{}, pool)

	newCode, newDocs, err := engine.MutateSkill(ctx, "MutatedSkill", "make it better", []string{"parent1", "parent2"})
	if err != nil {
		t.Fatalf("MutateSkill failed: %v", err)
	}

	if newCode != "func Mutated() { fmt.Println(\"Hello\") }" {
		t.Errorf("Unexpected new code: %s", newCode)
	}

	if newDocs != "Mutated skill docs" {
		t.Errorf("Unexpected new docs: %s", newDocs)
	}

	// Verify the new skill is in the DB
	verifyConn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection for verification: %v", err)
	}
	defer verifyConn.Close()

	node, err := verifyConn.GetNode(ctx, "MutatedSkill", []string{"Skill"})
	if err != nil {
		t.Fatalf("Failed to get new skill node: %v", err)
	}
	if node == nil {
		t.Fatalf("New skill node not found")
	}

	if node.Properties[objects.FieldKeyCode] != newCode {
		t.Errorf("DB code mismatch: expected %s, got %v", newCode, node.Properties[objects.FieldKeyCode])
	}

	// Cleanup test data
	_ = verifyConn.DeleteNode(ctx, "parent1", []string{"Skill"})
	_ = verifyConn.DeleteNode(ctx, "parent2", []string{"Skill"})
	_ = verifyConn.DeleteNode(ctx, "MutatedSkill", []string{"Skill"})

	_ = pool.ReturnConnection(verifyConn)
}
