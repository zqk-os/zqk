package mutation

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// LLMClient defines the required LLM operations for the Generative Mutation Engine.
type LLMClient interface {
	// GenerateCode mutates or combines existing skill code into new skill code.
	GenerateCode(ctx context.Context, prompt string, parentCode ...string) (string, error)
	// GenerateDocs generates documentation for the newly created skill.
	GenerateDocs(ctx context.Context, code string) (string, error)
}

// SemanticEngine defines the interface for semantic validation of the generated skill.
type SemanticEngine interface {
	// ValidateCode checks if the generated code is semantically valid.
	ValidateCode(ctx context.Context, code string) (bool, error)
}

// Engine is the Generative Mutation Engine responsible for creating novel skill code.
type Engine struct {
	llm      LLMClient
	semantic SemanticEngine
	dbPool   provider.ConnectionPool
}

// NewEngine creates a new Generative Mutation Engine.
func NewEngine(llm LLMClient, semantic SemanticEngine, dbPool provider.ConnectionPool) *Engine {
	return &Engine{
		llm:      llm,
		semantic: semantic,
		dbPool:   dbPool,
	}
}

// MutateSkill generates a novel skill by mutating/breeding parent skills.
func (e *Engine) MutateSkill(ctx context.Context, skillName string, prompt string, parentSkillIDs []string) (string, string, error) {
	var parentCode []string

	conn, err := e.dbPool.GetConnection(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get db connection: %w", err)
	}
	defer func() {
		_ = e.dbPool.ReturnConnection(conn)
	}()

	// Fetch parent code
	for _, parentID := range parentSkillIDs {
		node, err := conn.GetNode(ctx, parentID, []string{"Skill"})
		if err != nil {
			return "", "", fmt.Errorf("failed to get parent skill %s: %w", parentID, err)
		}
		if node != nil && node.Properties != nil {
			if codeVal, ok := node.Properties["code"].(string); ok {
				parentCode = append(parentCode, codeVal)
			}
		}
	}

	// Generate new code
	newCode, err := e.llm.GenerateCode(ctx, prompt, parentCode...)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate code: %w", err)
	}

	// Validate semantics
	valid, err := e.semantic.ValidateCode(ctx, newCode)
	if err != nil {
		return "", "", fmt.Errorf("semantic validation error: %w", err)
	}
	if !valid {
		return "", "", fmt.Errorf("generated code failed semantic validation")
	}

	// Generate documentation
	docs, err := e.llm.GenerateDocs(ctx, newCode)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate docs: %w", err)
	}

	// Store new skill in graph db
	props := map[string]any{
		objects.FieldKeyName: skillName,
		"code":               newCode,
		"docs":               docs,
	}

	err = conn.CreateNode(ctx, provider.Node{
		ID:         skillName,
		Labels:     []string{"Skill"},
		Properties: props,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to store mutated skill: %w", err)
	}

	return newCode, docs, nil
}
