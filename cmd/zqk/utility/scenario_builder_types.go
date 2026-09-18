package utility

import (
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ScenarioBuilderProfileName is the name of the CLI profile used by the scenario builder
const ScenarioBuilderProfileName = "scenario_builder"

const (
	scenarioBuilderStatusProgress  = "progress"
	scenarioBuilderStatusWarning   = "warning"
	scenarioBuilderStatusError     = "error"
	scenarioBuilderStatusComplete  = "complete"
	scenarioBuilderMetadataEvent   = "event"
	scenarioBuilderSchemaV2        = objects.DefaultSchemaVersion
	scenarioBuilderStatusActive    = "active"
	scenarioBuilderStatusExploring = "exploring"
	scenarioBuilderStatusInactive  = "inactive"
	scenarioBuilderStatusSuspended = "suspended"
	scenarioBuilderStatusProposed  = "proposed"
	scenarioBuilderSourceBuiltIn   = "built-in"
	scenarioBuilderKindScenario    = objects.KindScenario
	scenarioBuilderKindBacklogItem = objects.KindBacklogItem
	scenarioBuilderAccountSystem   = objects.DefaultSystemAccountID
	scenarioBuilderOriginZQK       = "zqk"
)

// ScenarioBuilderConfig holds configuration for scenario generation
type ScenarioBuilderConfig struct {
	// Target scenario directory
	TargetDir string

	// Object generation settings
	Kinds            []string       // Object kinds to generate
	Counts           map[string]int // Count per kind (overrides default)
	DefaultCount     int            // Default count if not specified per kind
	Diversity        map[string]int // Diversity level per kind (1-10, affects field variation)
	DefaultDiversity int            // Default diversity level

	// Relationship settings
	LinkProbability float64  // Probability of creating links between objects (0.0-1.0)
	LinkTypes       []string // Types of links to create (e.g., ["links_to_goals", "has_requirements"])

	// Temporal settings
	TimeRange time.Duration // Time range for created_at timestamps
	StartTime time.Time     // Start time for temporal distribution

	// Status distribution
	StatusDistribution map[string]map[string]float64 // kind -> status -> probability
}

// ScenarioBuilder implements the builder pattern for test scenarios
type ScenarioBuilder struct {
	config               *ScenarioBuilderConfig
	storage              storage.ObjectStorageProvider
	secCtx               *pkgctx.SecurityContext
	projectRoot          string
	logger               *logging.EventLogger      // Use EventLogger for consistency with Processor pattern
	coordinator          *coordination.Coordinator // Coordinator for output routing
	generated            map[string]int            // kind -> count generated
	generatedMu          sync.Mutex                // Protects generated map for concurrent access
	scenarioID           string                    // ID of the scenario object (if loaded from one)
	copyConfig           *ScenarioCopyConfig       // Configuration for copying objects from project
	dataFileObjects      []map[string]any          // Objects loaded from data file (flattened, for backward compatibility)
	dataFileObjectLayers [][]map[string]any        // Objects grouped by dependency layers (for parallel processing)
	force                bool                      // Force overwrite existing objects
}

// NewScenarioBuilder creates a new scenario builder
// Coordinator is required - all events must flow through the coordinator for proper routing
func NewScenarioBuilder(projectRoot string, storageProvider storage.ObjectStorageProvider, coordinator *coordination.Coordinator) *ScenarioBuilder {
	if coordinator == nil {
		// TRACK: [Test utility missing prerequisite]
		panic("coordinator is required for ScenarioBuilder - all events must flow through coordinator")
	}
	return &ScenarioBuilder{
		config: &ScenarioBuilderConfig{
			DefaultCount:       100,
			DefaultDiversity:   5,
			LinkProbability:    0.3,
			TimeRange:          30 * 24 * time.Hour, // 30 days
			StartTime:          time.Now().Add(-30 * 24 * time.Hour),
			StatusDistribution: make(map[string]map[string]float64),
		},
		storage:     storageProvider,
		secCtx:      pkgctx.NewSystemSecurityContext(),
		projectRoot: projectRoot,
		logger:      nil, // Will be set by Processor pattern
		coordinator: coordinator,
		generated:   make(map[string]int),
	}
}
