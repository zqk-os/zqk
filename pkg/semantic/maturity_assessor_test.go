package semantic

import (
	"testing"
)

// mockScanner is a stub scanner for testing
type mockScanner struct {
	indicators []Indicator
}

func (m *mockScanner) Scan(_ string) ([]Indicator, error) {
	return m.indicators, nil
}

func TestMaturityAssessor_Assess(t *testing.T) {
	tests := []struct {
		name          string
		indicators    []Indicator
		expectedLevel int
	}{
		{
			name:          "Level 0 - Naive",
			indicators:    []Indicator{},
			expectedLevel: 0,
		},
		{
			name: "Level 1 - Aware",
			indicators: []Indicator{
				{Type: "structured_data", Found: true, Sophistication: "basic"},
			},
			expectedLevel: 1,
		},
		{
			name: "Level 2 - Practicing",
			indicators: []Indicator{
				{Type: "structured_data", Found: true, Sophistication: "basic"},
				{Type: "formal_schemas", Found: true, Sophistication: "intermediate"},
			},
			expectedLevel: 2,
		},
		{
			name: "Level 3 - Advanced",
			indicators: []Indicator{
				{Type: "structured_data", Found: true, Sophistication: "basic"},
				{Type: "formal_schemas", Found: true, Sophistication: "intermediate"},
				{Type: "ontologies", Found: true, Sophistication: "advanced"},
			},
			expectedLevel: 3,
		},
		{
			name: "Level 4 - Expert",
			indicators: []Indicator{
				{Type: "structured_data", Found: true, Sophistication: "basic"},
				{Type: "formal_schemas", Found: true, Sophistication: "intermediate"},
				{Type: "ontologies", Found: true, Sophistication: "advanced"},
				{Type: "semantic_repositories", Found: true, Sophistication: "expert"},
			},
			expectedLevel: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := &mockScanner{indicators: tt.indicators}
			assessor := NewMaturityAssessor([]MaturityScanner{scanner})

			assessment, err := assessor.Assess("/tmp/mock-project")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if assessment.Level != tt.expectedLevel {
				t.Errorf("expected Level %d, got %d", tt.expectedLevel, assessment.Level)
			}

			if len(assessment.Recommendations) == 0 {
				t.Errorf("expected recommendations, got none")
			}
		})
	}
}
