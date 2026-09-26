package supervision

import (
	"strings"
	"sync"
)

// ExemplarRegistry maintains a catalog of canonical exemplars indexable by category and tag.
type ExemplarRegistry struct {
	mu        sync.RWMutex
	exemplars map[string][]CanonicalExemplar
}

var (
	defaultRegistry     *ExemplarRegistry
	defaultRegistryOnce sync.Once
)

// DefaultExemplarRegistry returns the global singleton exemplar registry.
func DefaultExemplarRegistry() *ExemplarRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewExemplarRegistry()
		defaultRegistry.seedDefaultExemplars()
	})
	return defaultRegistry
}

// NewExemplarRegistry creates a new isolated ExemplarRegistry.
func NewExemplarRegistry() *ExemplarRegistry {
	return &ExemplarRegistry{
		exemplars: make(map[string][]CanonicalExemplar),
	}
}

// Register adds an exemplar under a specific category key.
func (r *ExemplarRegistry) Register(category string, ex CanonicalExemplar) {
	r.mu.Lock()
	defer r.mu.Unlock()

	normCat := strings.ToLower(strings.TrimSpace(category))
	r.exemplars[normCat] = append(r.exemplars[normCat], ex)
}

// Resolve retrieves exemplars matching any of the specified categories or tags.
func (r *ExemplarRegistry) Resolve(categories ...string) []CanonicalExemplar {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []CanonicalExemplar
	seenTitles := make(map[string]bool)

	for _, cat := range categories {
		normCat := strings.ToLower(strings.TrimSpace(cat))
		for _, ex := range r.exemplars[normCat] {
			if !seenTitles[ex.Title] {
				result = append(result, ex)
				seenTitles[ex.Title] = true
			}
		}
	}

	return result
}

func (r *ExemplarRegistry) seedDefaultExemplars() {
	r.Register("testing", CanonicalExemplar{
		Category:    "testing",
		Title:       "Deterministic Unit Test Pattern",
		Description: "Write isolated, hermetic unit tests with deterministic assertions and cleanup.",
		PatternCode: `func TestOperation_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	svc := NewService()

	// Act
	result, err := svc.Execute(ctx, "target")

	// Assert
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result == nil || result.Status != "OK" {
		t.Errorf("unexpected result: %+v", result)
	}
}`,
		AntiPatternCode: `func TestOperation_Broken(t *testing.T) {
	svc := NewService()
	svc.Execute(nil, "target") // Ignored error, nil context, no assertions
}`,
	})

	r.Register("error_handling", CanonicalExemplar{
		Category:    "error_handling",
		Title:       "Structured Error Context Wrapping",
		Description: "Wrap operational errors with actionable context and preserve causal chains.",
		PatternCode: `if err := doOperation(); err != nil {
	return fmt.Errorf("operation failed for %s: %w", targetID, err)
}`,
		AntiPatternCode: `if err := doOperation(); err != nil {
	return errors.New("failed") // Error chain swallowed, zero context
}`,
	})

	r.Register("builder", CanonicalExemplar{
		Category:    "builder",
		Title:       "Fluent Builder Invariant Validation",
		Description: "Fluent builders must fail closed on invalid inputs during build/finalize.",
		PatternCode: `func (b *Builder) Build() (*Object, error) {
	if b.id == "" {
		return nil, errors.New("id is required")
	}
	return &Object{id: b.id}, nil
}`,
	})
}
