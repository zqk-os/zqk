package semantic

// Indicator represents a sign of semantic maturity found during assessment.
type Indicator struct {
	Type           string   `json:"type"`
	Found          bool     `json:"found"`
	Examples       []string `json:"examples"`
	Sophistication string   `json:"sophistication"`
}

// Recommendation provides next steps based on semantic maturity assessment.
type Recommendation struct {
	Recommendation string `json:"recommendation"`
	Priority       string `json:"priority"`
	Effort         string `json:"effort"`
	Benefit        string `json:"benefit"`
}

// MaturityAssessment holds the result of a maturity scan.
type MaturityAssessment struct {
	Level           int              `json:"level"`
	LevelName       string           `json:"level_name"`
	Indicators      []Indicator      `json:"indicators"`
	Recommendations []Recommendation `json:"recommendations"`
}

// MaturityScanner defines the interface for different types of maturity scanners.
type MaturityScanner interface {
	Scan(projectRoot string) ([]Indicator, error)
}

// MaturityAssessor evaluates the semantic maturity of a project.
type MaturityAssessor struct {
	scanners []MaturityScanner
}

// NewMaturityAssessor creates a new MaturityAssessor with the given scanners.
func NewMaturityAssessor(scanners []MaturityScanner) *MaturityAssessor {
	return &MaturityAssessor{
		scanners: scanners,
	}
}

// Assess evaluates the given projectRoot and returns a semantic maturity assessment.
func (ma *MaturityAssessor) Assess(projectRoot string) (*MaturityAssessment, error) {
	var indicators []Indicator
	for _, scanner := range ma.scanners {
		if foundIndicators, err := scanner.Scan(projectRoot); err == nil {
			indicators = append(indicators, foundIndicators...)
		}
	}

	level, levelName := determineMaturityLevel(indicators)
	recommendations := generateRecommendations(level, indicators)

	return &MaturityAssessment{
		Level:           level,
		LevelName:       levelName,
		Indicators:      indicators,
		Recommendations: recommendations,
	}, nil
}

// determineMaturityLevel determines the maturity level based on indicators
func determineMaturityLevel(indicators []Indicator) (level int, description string) {
	hasOntologies := false
	hasSemanticRepos := false
	hasFormalSchemas := false
	hasStructuredData := false

	for _, indicator := range indicators {
		switch indicator.Type {
		case "ontologies":
			hasOntologies = indicator.Found
		case "semantic_repositories":
			hasSemanticRepos = indicator.Found
		case "formal_schemas":
			hasFormalSchemas = indicator.Found
		case "structured_data":
			hasStructuredData = indicator.Found
		}
	}

	// Level 4: Expert - Has semantic repositories
	if hasSemanticRepos {
		return 4, "Expert"
	}

	// Level 3: Advanced - Has ontologies
	if hasOntologies {
		return 3, "Advanced"
	}

	// Level 2: Practicing - Has formal schemas
	if hasFormalSchemas {
		return 2, "Practicing"
	}

	// Level 1: Aware - Has structured data
	if hasStructuredData {
		return 1, "Aware"
	}

	// Level 0: Naive - No structured data
	return 0, "Naive"
}

// generateRecommendations generates recommendations based on maturity level
func generateRecommendations(level int, _ []Indicator) []Recommendation {
	recommendations := []Recommendation{}

	switch level {
	case 0: // Naive
		recommendations = append(recommendations,
			Recommendation{
				Recommendation: "Start with ZQK default organizational ontology",
				Priority:       "high",
				Effort:         "low",
				Benefit:        "Immediate organizational modeling capability",
			},
			Recommendation{
				Recommendation: "Enable guided discovery for domain ontologies",
				Priority:       "medium",
				Effort:         "low",
				Benefit:        "Progressive enhancement without complexity",
			})

	case 1: // Aware
		recommendations = append(recommendations,
			Recommendation{
				Recommendation: "Import existing structured data into ZQK format",
				Priority:       "high",
				Effort:         "medium",
				Benefit:        "Unified data model and improved consistency",
			},
			Recommendation{
				Recommendation: "Create formal schemas for key data structures",
				Priority:       "medium",
				Effort:         "medium",
				Benefit:        "Better validation and type safety",
			})

	case 2: // Practicing
		recommendations = append(recommendations,
			Recommendation{
				Recommendation: "Convert schemas to RDF/OWL ontologies",
				Priority:       "high",
				Effort:         "high",
				Benefit:        "Semantic reasoning and advanced querying",
			},
			Recommendation{
				Recommendation: "Set up semantic repository (SPARQL endpoint)",
				Priority:       "medium",
				Effort:         "high",
				Benefit:        "Advanced semantic operations",
			})

	case 3: // Advanced
		recommendations = append(recommendations,
			Recommendation{
				Recommendation: "Set up semantic repository for advanced querying",
				Priority:       "high",
				Effort:         "medium",
				Benefit:        "SPARQL queries and semantic reasoning",
			},
			Recommendation{
				Recommendation: "Integrate multiple ontologies with ZQK",
				Priority:       "medium",
				Effort:         "high",
				Benefit:        "Unified semantic architecture",
			})

	case 4: // Expert
		recommendations = append(recommendations,
			Recommendation{
				Recommendation: "Integrate ZQK with existing semantic infrastructure",
				Priority:       "high",
				Effort:         "medium",
				Benefit:        "Bidirectional sync and advanced features",
			},
			Recommendation{
				Recommendation: "Enable semantic reasoning and inference",
				Priority:       "medium",
				Effort:         "low",
				Benefit:        "Automated relationship discovery",
			})
	}

	return recommendations
}
