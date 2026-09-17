package storage

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/when"
)

// BucketStrategyStorageProvider defines the interface for storing and retrieving
// bucketing strategy configurations across different backends (file, graph, RDF, etc.)
type BucketStrategyStorageProvider interface {
	// LoadStrategy loads a bucketing strategy by ID
	LoadStrategy(ctx context.Context, strategyID string) (map[string]any, error)

	// LoadAllStrategies loads all bucketing strategies
	LoadAllStrategies(ctx context.Context) ([]map[string]any, error)

	// LoadStrategiesForKind loads all strategies that apply to a specific object kind
	LoadStrategiesForKind(ctx context.Context, kind string) ([]map[string]any, error)

	// SaveStrategy saves a bucketing strategy
	SaveStrategy(ctx context.Context, strategy map[string]any) error

	// DeleteStrategy deletes a bucketing strategy by ID
	DeleteStrategy(ctx context.Context, strategyID string) error

	// GetBackendType returns the type of backend (file, graph, rdf, etc.)
	GetBackendType() string
}

// FileBucketStrategyStorage implements BucketStrategyStorageProvider for file-based storage
// Reads strategies from YAML files in .zqk/process/bucketing_strategies/
type FileBucketStrategyStorage struct {
	projectRoot string
}

// NewFileBucketStrategyStorage creates a new file-based strategy storage
func NewFileBucketStrategyStorage(projectRoot string) *FileBucketStrategyStorage {
	return &FileBucketStrategyStorage{
		projectRoot: projectRoot,
	}
}

func (f *FileBucketStrategyStorage) GetBackendType() string {
	return "file"
}

func (f *FileBucketStrategyStorage) LoadStrategy(ctx context.Context, strategyID string) (map[string]any, error) {
	// Load from YAML file using ObjectStorageProvider
	// This reuses the existing file storage infrastructure
	storageFactory, err := NewStorageFactory(ctx, f.projectRoot)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgCreateStorageFactory).Wrap(err)
	}

	storage := storageFactory.GetStorage()
	obj, err := storage.Read(ctx, nil, strategyID)
	if err != nil {
		return nil, errfmt.Errorf(ErrMsgLoadStrategyFmt, strategyID, err)
	}

	return obj, nil
}

func (f *FileBucketStrategyStorage) LoadAllStrategies(ctx context.Context) ([]map[string]any, error) {
	loadStart := time.Now()
	// List all bucketing_strategy objects
	storageFactory, err := NewStorageFactory(ctx, f.projectRoot)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgCreateStorageFactory).Wrap(err)
	}

	storage := storageFactory.GetStorage()
	filter := ListFilter{
		Kind: objects.KindBucketingStrategy,
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	listStart := time.Now()
	results, err := storage.List(ctx, secCtx, storageCtx, filter)
	listDuration := time.Since(listStart)
	if listDuration > 100*time.Millisecond {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(LogEventStorageBucketingStorageListSlowDebug).
			String("operation", OpNameLoadAllStrategies).
			String("step", "storage_list").
			String("duration", listDuration.String()).
			WithFields(logging.Field{Key: "metadata", Value: map[string]any{"duration_ms": float64(listDuration.Nanoseconds()) / 1e6, "result_count": len(results.Objects)}}).
			Log()
	}
	if err != nil {
		return nil, errfmt.Newf(ErrMsgListStrategies).Wrap(err)
	}

	strategies := append(make([]map[string]any, 0, len(results.Objects)), results.Objects...)

	totalDuration := time.Since(loadStart)
	if totalDuration > 100*time.Millisecond {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(LogEventStorageBucketingStorageListTotalDurationDebug).
			String("operation", OpNameLoadAllStrategies).
			String("step", "complete").
			String("duration", totalDuration.String()).
			WithFields(logging.Field{Key: "metadata", Value: map[string]any{"duration_ms": float64(totalDuration.Nanoseconds()) / 1e6, FieldKeyStrategyCount: len(strategies)}}).
			Log()
	}

	return strategies, nil
}

func (f *FileBucketStrategyStorage) LoadStrategiesForKind(ctx context.Context, kind string) ([]map[string]any, error) {
	// Load all strategies and filter by applies_to
	allStrategies, err := f.LoadAllStrategies(ctx)
	if err != nil {
		return nil, err
	}

	var matchingStrategies []map[string]any
	for _, strategy := range allStrategies {
		appliesTo, ok := strategy[objects.FieldKeyAppliesTo].([]any)
		if !ok {
			continue
		}

		// Check if kind is in applies_to
		for _, appliedKind := range appliesTo {
			if appliedKindStr, ok := appliedKind.(string); ok && appliedKindStr == kind {
				enabled, _ := strategy[objects.FieldKeyEnabled].(bool)
				if enabled {
					matchingStrategies = append(matchingStrategies, strategy)
				}
				break
			}
		}
	}

	return matchingStrategies, nil
}

func (f *FileBucketStrategyStorage) SaveStrategy(ctx context.Context, strategy map[string]any) error {
	// Save using ObjectStorageProvider
	storageFactory, err := NewStorageFactory(ctx, f.projectRoot)
	if err != nil {
		return errfmt.Newf(ErrMsgCreateStorageFactory).Wrap(err)
	}

	storage := storageFactory.GetStorage()
	strategyID, ok := strategy[objects.FieldKeyID].(string)
	if !ok {
		return errfmt.Errorf(ErrMsgStrategyNoID)
	}

	// Use Update if exists, Create if new
	_, readErr := storage.Read(ctx, nil, strategyID)
	var saveErr error
	when.When(func() bool { return readErr != nil }).Then(func() {
		saveErr = storage.Create(ctx, nil, strategy)
	}).OrElse(func() {
		saveErr = storage.Update(ctx, nil, strategyID, strategy)
	}).Run()
	if saveErr != nil {
		return errfmt.Newf(ErrMsgSaveStrategy).Wrap(saveErr)
	}

	return nil
}

func (f *FileBucketStrategyStorage) DeleteStrategy(ctx context.Context, strategyID string) error {
	storageFactory, err := NewStorageFactory(ctx, f.projectRoot)
	if err != nil {
		return errfmt.Newf(ErrMsgCreateStorageFactory).Wrap(err)
	}

	storage := storageFactory.GetStorage()
	err = storage.Delete(ctx, nil, strategyID, false)
	if err != nil {
		return errfmt.Newf(ErrMsgDeleteStrategy).Wrap(err)
	}

	return nil
}

// GraphBucketStrategyStorage implements BucketStrategyStorageProvider for graph-based storage
// Stores strategies as nodes in the graph database (Memgraph, Neo4j, etc.)
type GraphBucketStrategyStorage struct {
	storage ObjectStorageProvider
}

// NewGraphBucketStrategyStorage creates a new graph-based strategy storage
func NewGraphBucketStrategyStorage(storage ObjectStorageProvider) *GraphBucketStrategyStorage {
	return &GraphBucketStrategyStorage{
		storage: storage,
	}
}

func (g *GraphBucketStrategyStorage) GetBackendType() string {
	return "graph"
}

func (g *GraphBucketStrategyStorage) LoadStrategy(ctx context.Context, strategyID string) (map[string]any, error) {
	obj, err := g.storage.Read(ctx, nil, strategyID)
	if err != nil {
		return nil, errfmt.Errorf(ErrMsgLoadStrategyFmt, strategyID, err)
	}
	return obj, nil
}

func (g *GraphBucketStrategyStorage) LoadAllStrategies(ctx context.Context) ([]map[string]any, error) {
	filter := ListFilter{
		Kind: objects.KindBucketingStrategy,
	}
	results, err := g.storage.List(ctx, nil, nil, filter)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgListStrategies).Wrap(err)
	}

	strategies := append(make([]map[string]any, 0, len(results.Objects)), results.Objects...)

	return strategies, nil
}

func (g *GraphBucketStrategyStorage) LoadStrategiesForKind(ctx context.Context, kind string) ([]map[string]any, error) {
	// Use graph query to find strategies that apply to this kind
	// This is more efficient than loading all and filtering
	allStrategies, err := g.LoadAllStrategies(ctx)
	if err != nil {
		return nil, err
	}

	var matchingStrategies []map[string]any
	for _, strategy := range allStrategies {
		appliesTo, ok := strategy[objects.FieldKeyAppliesTo].([]any)
		if !ok {
			continue
		}

		for _, appliedKind := range appliesTo {
			if appliedKindStr, ok := appliedKind.(string); ok && appliedKindStr == kind {
				enabled, _ := strategy[objects.FieldKeyEnabled].(bool)
				if enabled {
					matchingStrategies = append(matchingStrategies, strategy)
				}
				break
			}
		}
	}

	return matchingStrategies, nil
}

func (g *GraphBucketStrategyStorage) SaveStrategy(ctx context.Context, strategy map[string]any) error {
	strategyID, ok := strategy[objects.FieldKeyID].(string)
	if !ok {
		return errfmt.Errorf(ErrMsgStrategyNoID)
	}

	// Use Update if exists, Create if new
	_, readErr := g.storage.Read(ctx, nil, strategyID)
	var saveErr error
	when.When(func() bool { return readErr != nil }).Then(func() {
		saveErr = g.storage.Create(ctx, nil, strategy)
	}).OrElse(func() {
		saveErr = g.storage.Update(ctx, nil, strategyID, strategy)
	}).Run()
	if saveErr != nil {
		return errfmt.Newf(ErrMsgSaveStrategy).Wrap(saveErr)
	}
	return nil
}

func (g *GraphBucketStrategyStorage) DeleteStrategy(ctx context.Context, strategyID string) error {
	err := g.storage.Delete(ctx, nil, strategyID, false)
	if err != nil {
		return errfmt.Newf(ErrMsgDeleteStrategy).Wrap(err)
	}
	return nil
}

// existingStorageBucketStrategyProvider uses an existing ObjectStorageProvider to load/save
// bucketing strategies without creating a new StorageFactory. Used when initializing the
// bucket strategy registry from inside FileObjectStorage to avoid recursion
// (getBucketStrategyRegistry -> NewDefaultBucketStrategyRegistry -> NewBucketStrategyLoader ->
// NewBucketStrategyStorageFactory -> NewStorageFactory -> NewFileObjectStorage -> write-behind -> apply).
type existingStorageBucketStrategyProvider struct {
	storage ObjectStorageProvider
}

// NewExistingStorageBucketStrategyProvider returns a BucketStrategyStorageProvider that delegates
// to the given storage. Call this from getBucketStrategyRegistry with the current FileObjectStorage
// so registry init does not create another FileObjectStorage.
func NewExistingStorageBucketStrategyProvider(storage ObjectStorageProvider) BucketStrategyStorageProvider {
	return &existingStorageBucketStrategyProvider{storage: storage}
}

func (e *existingStorageBucketStrategyProvider) GetBackendType() string { return "file" }

func (e *existingStorageBucketStrategyProvider) LoadStrategy(ctx context.Context, strategyID string) (map[string]any, error) {
	obj, err := e.storage.Read(ctx, nil, strategyID)
	if err != nil {
		return nil, errfmt.Errorf(ErrMsgLoadStrategyFmt, strategyID, err)
	}
	return obj, nil
}

func (e *existingStorageBucketStrategyProvider) LoadAllStrategies(ctx context.Context) ([]map[string]any, error) {
	filter := ListFilter{Kind: objects.KindBucketingStrategy}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()
	results, err := e.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgListStrategies).Wrap(err)
	}
	return append(make([]map[string]any, 0, len(results.Objects)), results.Objects...), nil
}

func (e *existingStorageBucketStrategyProvider) LoadStrategiesForKind(ctx context.Context, kind string) ([]map[string]any, error) {
	all, err := e.LoadAllStrategies(ctx)
	if err != nil {
		return nil, err
	}
	var matching []map[string]any
	for _, strategy := range all {
		appliesTo, ok := strategy[objects.FieldKeyAppliesTo].([]any)
		if !ok {
			continue
		}
		for _, appliedKind := range appliesTo {
			if s, ok := appliedKind.(string); ok && s == kind {
				if enabled, _ := strategy[objects.FieldKeyEnabled].(bool); enabled {
					matching = append(matching, strategy)
				}
				break
			}
		}
	}
	return matching, nil
}

func (e *existingStorageBucketStrategyProvider) SaveStrategy(ctx context.Context, strategy map[string]any) error {
	strategyID, ok := strategy[objects.FieldKeyID].(string)
	if !ok {
		return errfmt.Errorf(ErrMsgStrategyNoID)
	}
	_, err := e.storage.Read(ctx, nil, strategyID)
	if err != nil {
		return e.storage.Create(ctx, nil, strategy)
	}
	return e.storage.Update(ctx, nil, strategyID, strategy)
}

func (e *existingStorageBucketStrategyProvider) DeleteStrategy(ctx context.Context, strategyID string) error {
	return e.storage.Delete(ctx, nil, strategyID, false)
}

// BucketStrategyStorageFactory creates the appropriate strategy storage provider
// based on the configured backend (file, graph, RDF, etc.)
type BucketStrategyStorageFactory struct {
	projectRoot string
	storage     ObjectStorageProvider
}

// NewBucketStrategyStorageFactory creates a new factory and detects the appropriate backend
func NewBucketStrategyStorageFactory(ctx context.Context, projectRoot string) (*BucketStrategyStorageFactory, error) {
	factory := &BucketStrategyStorageFactory{
		projectRoot: projectRoot,
	}

	// Use the same storage factory to detect backend
	storageFactory, err := NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgCreateStorageFactory).Wrap(err)
	}

	factory.storage = storageFactory.GetStorage()
	return factory, nil
}

// GetStorageProvider returns the appropriate BucketStrategyStorageProvider
// based on the detected backend
func (f *BucketStrategyStorageFactory) GetStorageProvider() BucketStrategyStorageProvider {
	// Check if we're using graph backend
	if graphStorage, ok := f.storage.(*GraphObjectStorage); ok {
		return NewGraphBucketStrategyStorage(graphStorage)
	}

	// Default to file-based storage
	return NewFileBucketStrategyStorage(f.projectRoot)
}

// GetBackendType returns the type of backend in use
func (f *BucketStrategyStorageFactory) GetBackendType() string {
	if _, ok := f.storage.(*GraphObjectStorage); ok {
		return "graph"
	}
	return "file"
}
