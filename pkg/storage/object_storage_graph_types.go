package storage

import (
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

// GraphObjectStorage implements ObjectStorageProvider for graph-based storage.
// Spec YAML is still read from the project tree via SpecLoader + FileSpecStorageProvider (REQ-035 / BLI-149);
// the graph holds object instance data, not spec definitions.
type GraphObjectStorage struct {
	conn            provider.GraphConnection
	validator       validation.Validator
	idValidator     *validation.IDValidator
	specLoader      *objects.SpecLoader
	lifecycleLoader *objects.LifecycleLoader
	projectRoot     string
	bucketRegistry  *BucketStrategyRegistry
}

// NewGraphObjectStorage creates a new graph-based object storage.
// projectRoot is the workspace root used to resolve .zqk/specs/objects (may be "" for tests).
func NewGraphObjectStorage(conn provider.GraphConnection, projectRoot string) (*GraphObjectStorage, error) {
	if conn == nil {
		return nil, errfmt.Errorf(ConstStreamGraphConnectionIsRequired)
	}

	// Initialize loaders (same spec path resolution as file backend when projectRoot is set)
	specLoader := objects.NewSpecLoader(projectRoot)
	lifecycleLoader := objects.NewLifecycleLoader("")

	// Initialize bucket registry
	bucketRegistry := NewBucketStrategyRegistry()
	if projectRoot != emptyValue {
		// Just register some defaults for graph
		bucketRegistry.RegisterStrategy("graph_node", NewMonthlyChronoStrategy("created_at"))
	}

	// Get validator from registry
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("go") // Use Go validator
	if validator == nil {
		return nil, errfmt.Errorf(ConstStreamValidatorNotAvailable)
	}

	return &GraphObjectStorage{
		conn:            conn,
		validator:       validator,
		idValidator:     validation.GetIDValidator(),
		specLoader:      specLoader,
		lifecycleLoader: lifecycleLoader,
		projectRoot:     projectRoot,
		bucketRegistry:  bucketRegistry,
	}, nil
}
