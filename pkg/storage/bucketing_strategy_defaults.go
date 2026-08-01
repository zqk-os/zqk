package storage

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/when"
)

// bucketStrategyRegistryContextKey is a private type for context key
// This prevents collisions with other context values
type bucketStrategyRegistryContextKey struct{}

// WithBucketStrategyRegistryInitialization marks the context as being used
// during bucket strategy registry initialization. This prevents circular
// dependencies when the registry needs to list bucketing_strategy objects.
func WithBucketStrategyRegistryInitialization(ctx context.Context) context.Context {
	return context.WithValue(ctx, bucketStrategyRegistryContextKey{}, true)
}

// IsBucketStrategyRegistryInitializing checks if we're currently initializing
// the bucket strategy registry. This allows GetStrategyForKind to skip
// strategy lookup during initialization to avoid circular dependencies.
func IsBucketStrategyRegistryInitializing(ctx context.Context) bool {
	if val, ok := ctx.Value(bucketStrategyRegistryContextKey{}).(bool); ok {
		return val
	}
	return false
}

// DefaultBucketStrategyRegistry ensures all object kinds have bucketing strategies
// Base strategy is kind-based, with sub-kind support (e.g., Decision -> ADR)
type DefaultBucketStrategyRegistry struct {
	loader        *BucketStrategyLoader
	kindMapper    *objects.DynamicKindMapper
	specLoader    *objects.SpecLoader
	defaultsCache map[string]BucketStrategy // kind -> default strategy
	baseKindCache map[string]string         // kind -> base kind (avoids repeated getBaseKind work; OBJECT_OPERATIONS_PERFORMANCE.md)
	mu            sync.RWMutex
}

// NewDefaultBucketStrategyRegistry creates a registry that ensures all kinds have strategies.
// It creates a new BucketStrategyLoader (and thus may create NewStorageFactory -> NewFileObjectStorage).
// Do not call this from inside FileObjectStorage's write-behind apply path; use
// NewDefaultBucketStrategyRegistryWithProvider with the current storage instead.
func NewDefaultBucketStrategyRegistry(ctx context.Context, projectRoot string) (*DefaultBucketStrategyRegistry, error) {
	loader, err := NewBucketStrategyLoader(ctx, projectRoot)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgCreateStrategyLoader).Wrap(err)
	}
	return newDefaultBucketStrategyRegistryFromLoader(ctx, loader)
}

// NewDefaultBucketStrategyRegistryWithProvider creates a registry using the given storage provider
// for loading bucketing strategies. Use this when initializing the registry from inside
// FileObjectStorage (e.g. in getBucketStrategyRegistry) to avoid recursion: pass
// NewExistingStorageBucketStrategyProvider(currentStorage) so no second FileObjectStorage is created.
func NewDefaultBucketStrategyRegistryWithProvider(ctx context.Context, provider BucketStrategyStorageProvider) (*DefaultBucketStrategyRegistry, error) {
	loader := NewBucketStrategyLoaderWithProvider(provider)
	return newDefaultBucketStrategyRegistryFromLoader(ctx, loader)
}

func newDefaultBucketStrategyRegistryFromLoader(ctx context.Context, loader *BucketStrategyLoader) (*DefaultBucketStrategyRegistry, error) {
	// Mark context as being used during initialization to prevent circular dependencies
	initCtx := WithBucketStrategyRegistryInitialization(ctx)
	if err := loader.Initialize(initCtx); err != nil {
		return nil, errfmt.Newf(ErrMsgInitStrategyLoader).Wrap(err)
	}
	return &DefaultBucketStrategyRegistry{
		loader:        loader,
		kindMapper:    objects.GetGlobalKindMapper(),
		specLoader:    objects.GetGlobalSpecLoader(),
		defaultsCache: make(map[string]BucketStrategy),
		baseKindCache: make(map[string]string),
	}, nil
}

// GetStrategyForKind returns the bucketing strategy for an object kind
// Returns composite strategy if multiple strategies exist, single strategy if one exists,
// or default kind-based strategy if none exist
// Supports sub-kinds (e.g., Decision -> ADR)
func (r *DefaultBucketStrategyRegistry) GetStrategyForKind(ctx context.Context, kind string) (BucketStrategy, error) {
	// Skip strategy lookup during registry initialization to avoid circular dependency
	// When initializing, we're listing bucketing_strategy objects, which don't need bucketing
	if IsBucketStrategyRegistryInitializing(ctx) {
		return nil, nil
	}

	// Check for explicit strategies first
	strategies, err := r.loader.GetStrategiesForKind(ctx, kind)
	if err != nil {
		return nil, errfmt.Errorf(ErrMsgLoadStrategiesKind, kind, err)
	}

	// Filter to only enabled strategies
	enabledStrategies := make([]map[string]any, 0)
	for _, strategyData := range strategies {
		enabled, ok := strategyData[objects.FieldKeyEnabled].(bool)
		if ok && enabled {
			enabledStrategies = append(enabledStrategies, strategyData)
		}
	}

	// If multiple strategies exist, create a composite
	if len(enabledStrategies) > 1 {
		bucketStrategies := make([]BucketStrategy, 0, len(enabledStrategies))
		for _, strategyData := range enabledStrategies {
			bs, err := r.convertStrategyToBucketStrategy(strategyData)
			if err != nil {
				return nil, errfmt.Newf(ErrMsgConvertStrategy).Wrap(err)
			}
			bucketStrategies = append(bucketStrategies, bs)
		}
		return &CompositeBucketStrategy{
			Strategies: bucketStrategies,
			Separator:  BucketSeparator,
		}, nil
	}

	// If one strategy exists, return it
	if len(enabledStrategies) == 1 {
		return r.convertStrategyToBucketStrategy(enabledStrategies[0])
	}

	// No explicit strategy - use default kind-based strategy
	return r.getDefaultStrategyForKind(kind)
}

// getDefaultStrategyForKind returns the default bucketing strategy for a kind
// Default is path-based (no bucketing) unless the kind is known to be high-volume
func (r *DefaultBucketStrategyRegistry) getDefaultStrategyForKind(kind string) (BucketStrategy, error) {
	var strategy BucketStrategy
	var exists bool
	if err := concurrency.RunInRLockWithLogger(&r.mu, locknames.LockNameBucketStrategyGetDefaultCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var ok bool
		strategy, ok = r.defaultsCache[kind]
		exists = ok
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockDefaultStrategyCheck, err).Log()
	}

	if exists {
		return strategy, nil
	}

	// Check if this is a sub-kind (e.g., ADR is a sub-kind of Decision)
	baseKind := r.getBaseKind(kind)
	if baseKind != kind {
		// Use base kind's strategy
		return r.getDefaultStrategyForKind(baseKind)
	}

	// Determine default strategy based on kind characteristics
	strategy = r.determineDefaultStrategy(kind)

	// Cache it
	if err := concurrency.RunInLockWithLogger(&r.mu, locknames.LockNameBucketStrategyGetDefaultCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		r.defaultsCache[kind] = strategy
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockDefaultStrategyCache, err).Log()
	}

	return strategy, nil
}

// getBaseKind returns the base kind for a sub-kind
// For example, "adr" or "arch_decision_record" -> "decision"
// Uses kind_synonym objects to resolve sub-kinds to base kinds.
// Results are cached to avoid repeated Initialize/ResolveKind on hot path (OBJECT_OPERATIONS_PERFORMANCE.md).
func (r *DefaultBucketStrategyRegistry) getBaseKind(kind string) string {
	var result string
	var found bool
	if err := concurrency.RunInRLockWithLogger(&r.mu, locknames.LockNameBucketStrategyGetBaseCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		result, found = r.baseKindCache[kind]
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockBaseKindCheck, err).Log()
	}
	if found {
		return result
	}
	result = r.computeBaseKind(kind)
	if err := concurrency.RunInLockWithLogger(&r.mu, locknames.LockNameBucketStrategyGetBaseCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		r.baseKindCache[kind] = result
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockBaseKindCache, err).Log()
	}
	return result
}

func (r *DefaultBucketStrategyRegistry) computeBaseKind(kind string) string {
	// Initialize kind mapper if needed
	if err := r.kindMapper.Initialize(); err != nil {
		return kind // Fallback to original kind
	}

	// Use KindSynonymResolver to resolve sub-kinds to base kinds
	synonymResolver := objects.GetGlobalSynonymResolver()
	if synonymResolver != nil {
		if err := synonymResolver.Initialize(); err == nil {
			resolvedKind := synonymResolver.ResolveKind(kind)
			if resolvedKind != kind {
				if dir := r.kindMapper.GetDirectoryFromKind(resolvedKind); dir != emptyValue {
					return resolvedKind
				}
			}
		}
	}

	subKindMap := map[string]string{
		objects.KindAdr:                   objects.KindDecision,
		objects.KindArchDecisionRecord:    objects.KindDecision,
		objects.KindArchitecturalDecision: objects.KindDecision,
	}
	if baseKind, ok := subKindMap[kind]; ok {
		if dir := r.kindMapper.GetDirectoryFromKind(baseKind); dir != emptyValue {
			return baseKind
		}
	}
	return kind
}

// determineDefaultStrategy determines the appropriate default strategy for a kind
// High-volume kinds get chronological bucketing; others path-based (no bucketing).
// glossary_term uses first-letter bucketing via an explicit bucketing_strategy object (strategy_type: first_letter).
func (r *DefaultBucketStrategyRegistry) determineDefaultStrategy(kind string) BucketStrategy {
	// High-volume kinds that benefit from chronological bucketing (see high_volume_kinds.yaml)
	highVolumeKinds := map[string]bool{
		objects.KindAuditEvent:             true,
		objects.KindChangeJournalEntry:     true,
		objects.KindBaseMetric:             true,
		objects.KindCommandMetric:          true,
		objects.KindSchedulerHealthMetric:  true,
		objects.KindFileLockMetric:         true,
		objects.KindAuditAggregationMetric: true,
		objects.KindSchedulerJob:           true,
		objects.KindZqkSession:             true,
	}

	if highVolumeKinds[kind] {
		// Use monthly chronological bucketing by default
		// Can be overridden with explicit strategy for more granular bucketing
		return &ChronoBucketStrategy{
			Field:       objects.FieldKeyCreatedAt,
			Granularity: GranularityMonthly, // Use predefined granularity
			ParseFunc: func(s string) (time.Time, error) {
				return time.Parse(time.RFC3339, s)
			},
		}
	}

	// Default: path-based (no bucketing)
	return &PathBasedBucketStrategy{}
}

// convertStrategyToBucketStrategy converts a loaded strategy (map[string]any) to BucketStrategy
func (r *DefaultBucketStrategyRegistry) convertStrategyToBucketStrategy(strategyData map[string]any) (BucketStrategy, error) {
	strategyType, ok := strategyData[objects.FieldKeyStrategyType].(string)
	if !ok {
		return nil, errfmt.Errorf(ErrMsgStrategyMissingType)
	}

	field, ok := strategyData[objects.FieldKeyField].(string)
	if !ok {
		field = ""
	}
	format, ok := strategyData[objects.FieldKeyFormat].(string)
	if !ok {
		format = ""
	}
	granularity, ok := strategyData[FieldKeyGranularity].(string)
	if !ok {
		granularity = ""
	}

	// If format is a predefined granularity string, use it as granularity
	if format != emptyValue {
		predefinedGranularities := map[string]bool{
			GranularityMonthly:    true,
			GranularityWeekly:     true,
			GranularityDaily:      true,
			GranularityHourly:     true,
			GranularityHalfHourly: true,
			GranularityQtrHourly:  true,
			GranularityTenths:     true,
		}
		if predefinedGranularities[format] {
			granularity = format
			format = "" // Clear format since we're using granularity
		}
	}

	switch strategyType {
	case StrategyTypeChronological:
		if format == emptyValue && granularity == emptyValue {
			return nil, errfmt.Errorf(ErrMsgChronoNeedsFormat)
		}
		return &ChronoBucketStrategy{
			Field:       field,
			Format:      format,
			Granularity: granularity,
			ParseFunc: func(s string) (time.Time, error) {
				return time.Parse(time.RFC3339, s)
			},
		}, nil

	case StrategyTypeState:
		return &StateBucketStrategy{
			Field: field,
		}, nil

	case StrategyTypeSize:
		return &SizeBucketStrategy{
			Field: field,
		}, nil

	case StrategyTypeFirstLetter:
		if field == emptyValue {
			field = objects.FieldKeyTitle
		}
		return &FirstLetterBucketStrategy{
			Field: field,
		}, nil

	case StrategyTypeComposite:
		// Composite strategies would need more complex parsing
		// For now, return path-based as fallback
		return &PathBasedBucketStrategy{}, nil

	default:
		return &PathBasedBucketStrategy{}, nil
	}
}

// EnsureAllKindsHaveStrategies ensures that all discovered object kinds have strategies
// This is called during system initialization
// Returns a map of kind -> strategy name for logging/reporting
func (r *DefaultBucketStrategyRegistry) EnsureAllKindsHaveStrategies(ctx context.Context) (map[string]string, error) {
	// Initialize kind mapper
	if err := r.kindMapper.Initialize(); err != nil {
		return nil, errfmt.Newf(ErrMsgInitKindMapper).Wrap(err)
	}

	// Get all discovered kinds
	allKinds := r.getAllSystemKinds()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	strategyMap := make(map[string]string)

	// Ensure each kind has a strategy (either explicit or default)
	for _, kind := range allKinds {
		strategy, err := r.GetStrategyForKind(ctx, kind)
		when.When(func() bool { return err != nil }).Then(func() {
			StorageLog(logger).Warn(LogEventStorageBucketingGetStrategyForKindFailedWarn).
				Kind(kind).
				WithError(err).
				Log()
			strategyMap[kind] = DefaultStrategyError
		}).OrElse(func() {
			strategyMap[kind] = strategy.Name()
		}).Run()
	}

	StorageLog(logger).Info(LogEventStorageBucketingEnsuredAllKindsInfo).
		Int("kind_count", len(allKinds)).
		Int(FieldKeyStrategyCount, len(strategyMap)).
		Log()

	return strategyMap, nil
}

// getAllSystemKinds returns all object kinds discovered in the system
func (r *DefaultBucketStrategyRegistry) getAllSystemKinds() []string {
	// Initialize kind mapper to ensure it has discovered all kinds
	if err := r.kindMapper.Initialize(); err != nil {
		// Fallback to common kinds if initialization fails
		return r.getCommonSystemKinds()
	}

	// Get all kinds from the kind mapper
	allKinds := r.kindMapper.GetAllKinds()
	if len(allKinds) > 0 {
		return allKinds
	}

	// Fallback: use common system kinds
	return r.getCommonSystemKinds()
}

// getCommonSystemKinds returns a list of common system object kinds
func (r *DefaultBucketStrategyRegistry) getCommonSystemKinds() []string {
	return []string{
		// Core objects
		objects.KindGoal, objects.KindMilestone, objects.KindWorkstream, objects.KindPriorityPlan, objects.KindBacklogItem,
		objects.KindRequirement, objects.KindCriteria, objects.KindTestCase,
		objects.KindPolicy, objects.KindDecision, objects.KindQuestion,
		objects.KindMission, objects.KindVision, objects.KindRoadmap,
		objects.KindAccount, objects.KindRole, objects.KindComponent,

		// High-volume objects (need bucketing)
		objects.KindAuditEvent, objects.KindChangeJournalEntry,
		objects.KindBaseMetric, objects.KindCommandMetric, objects.KindSchedulerHealthMetric,
		objects.KindFileLockMetric, objects.KindAuditAggregationMetric,
		objects.KindVerificationMatrix,

		// System objects
		objects.KindCodeReference, objects.KindDocEntry,
		objects.KindPersona, objects.KindResolver, objects.KindRule, objects.KindScenario,
		objects.KindSchedulerJob, objects.KindTemplate,
		objects.KindContextRefreshSchedule, objects.KindIntegrityManifest,
		objects.KindMetadataPackage, objects.KindRiskBlocker, objects.KindRollbackReport,
		objects.KindDisplay, objects.KindNamespace, objects.KindNamespaceRegistry,
		objects.KindDomainRegistry, objects.KindImportTracking, objects.KindSynonym,
		objects.KindGlossaryTerm,
		objects.KindGlossaryTermRelation,
		objects.KindVocabularyScheme,
		objects.KindConvergenceSession,

		// Organizational objects
		objects.KindOrganization, objects.KindDivision, objects.KindDepartment, objects.KindTeam, objects.KindPartnership,

		// Sub-kinds
		objects.KindAdr, // Architectural Decision Record (sub-kind of decision)
	}
}
