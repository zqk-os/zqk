package docman

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage"
	"gopkg.in/yaml.v3"
)

const emptyValue = ""

// Registry manages doc_entry object creation and registration
type Registry struct {
	storageProvider storage.ObjectStorageProvider
	projectRoot     string
}

// NewRegistry creates a new doc_entry registry
func NewRegistry(storageProvider storage.ObjectStorageProvider, projectRoot string) *Registry {
	return &Registry{
		storageProvider: storageProvider,
		projectRoot:     projectRoot,
	}
}

// GetExistingEntries returns a map of existing doc_entry paths to their IDs
func (r *Registry) GetExistingEntries(ctx context.Context, profile string) (existing map[string]string, maxID int, err error) {
	existing = make(map[string]string)
	maxID = 0

	// Create security context
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// List all existing doc_entry objects
	filter := storage.ListFilter{
		Kind: objects.KindDocEntry,
	}
	results, err := r.storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return existing, maxID, errfmt.Newf("failed to list existing doc_entries").Wrap(err)
	}

	for _, result := range results.Objects {
		if path, ok := result[objects.FieldKeyPath].(string); ok {
			if id, ok := result[objects.FieldKeyID].(string); ok {
				key := paths.NormalizeDocEntryPathForKey(path)
				existing[key] = id
				// Extract max ID
				if strings.HasPrefix(id, "DOC-") {
					var idNum int
					_, _ = fmt.Sscanf(id, "DOC-%d", &idNum)
					if idNum > maxID {
						maxID = idNum
					}
				}
			}
		}
	}

	return existing, maxID, nil
}

// CreateDocEntry creates a doc_entry object for a markdown file
func (r *Registry) CreateDocEntry(ctx context.Context, profile string, file *MarkdownFile, metadata *DocumentMetadata, docID string) error {
	logger := logging.GetLoggerFromProfile(profile)

	// Build doc_entry object (path uses path-cache alias for programmatic resolution; see PATH_CACHE_MIGRATION_SCAN.md)
	entry := map[string]any{
		objects.FieldKeyID:                docID,
		objects.FieldKeyKind:              objects.KindDocEntry,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:             metadata.Title,
		objects.FieldKeyStatus:            metadata.Status,
		objects.FieldKeyPath:              paths.PathRefFromRelPath(file.RelPath),
		objects.FieldKeySummary:           metadata.Summary,
		objects.FieldKeyGroup:             metadata.Group,
		objects.FieldKeyContentSearchable: true,
		// Initialize optional reference fields with empty lists (per spec defaults)
		objects.FieldKeyGoalRefs:        []string{},
		objects.FieldKeyWorkstreamRefs:  []string{},
		objects.FieldKeyMilestoneRefs:   []string{},
		objects.FieldKeyRequirementRefs: []string{},
	}

	// Cryptographic sealing: compute SHA-256 and content size
	targetPath := file.Path
	if targetPath == "" {
		targetPath = filepath.Join(r.projectRoot, file.RelPath)
	}
	if hash, size, err := storage.ComputeFileIntegrity(targetPath); err == nil {
		entry["content_hash"] = hash
		entry["content_size"] = size
	}

	if metadata.Category != emptyValue {
		entry[objects.FieldKeyCategory] = metadata.Category
	}

	// Create security context
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create the object using storage provider (draft-first: create parks at lifecycle origin).
	err := r.storageProvider.Create(ctx, secCtx, entry)
	if err != nil {
		// Check if it's a duplicate error
		if strings.Contains(err.Error(), "already exists") {
			logging.Fluent(logger).Debug("Doc entry already exists, skipping").
				ObjectID(docID).
				Path(file.RelPath).
				Log()
			return nil // Not an error, just skip
		}
		return errfmt.Errorf("failed to create doc_entry %s: %w", docID, err)
	}

	// Promote off draft plane so List/registry discovery see the entry (origin is preliminary).
	// Prefer review (shovel_ready) over terminal active when metadata asked for active.
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	leaveStatus := objects.GetString(entry, objects.FieldKeyStatus)
	if leaveStatus == "" || leaveStatus == objects.ObjectStatusDraft || leaveStatus == objects.ObjectStatusActive {
		leaveStatus = "review"
	}
	if err := r.storageProvider.Update(ctx, secCtx, docID, map[string]any{objects.FieldKeyStatus: leaveStatus}); err != nil {
		return errfmt.Errorf("failed to promote doc_entry %s off draft plane: %w", docID, err)
	}
	entry[objects.FieldKeyStatus] = leaveStatus

	logging.Fluent(logger).Info("Created doc_entry").
		ObjectID(docID).
		Path(file.RelPath).
		String("title", metadata.Title).
		Log()

	// Incremental update: add this path to the path alias cache so prefix:file.RelPath resolves immediately
	paths.AddPathAlias(r.projectRoot, file.RelPath, file.RelPath)
	process.TouchMeaningfulActivity()

	return nil
}

// RegisterAll discovers and registers all documentation files
func (r *Registry) RegisterAll(ctx context.Context, profile string, dryRun bool) (registered, skipped int, err error) {
	return r.RegisterSubtrees(ctx, profile, nil, dryRun)
}

// RegisterSubtrees discovers and registers documentation files in the specified subtrees.
// If subtrees is empty or nil, it discovers all markdown files under docs/.
func (r *Registry) RegisterSubtrees(ctx context.Context, profile string, subtrees []string, dryRun bool) (registered, skipped int, err error) {
	logger := logging.GetLoggerFromProfile(profile)

	// Discover markdown files
	var discoverer *Discoverer
	if len(subtrees) > 0 {
		discoverer = NewDiscovererWithSubtrees(r.projectRoot, subtrees)
	} else {
		discoverer = NewDiscoverer(r.projectRoot)
	}

	files, err := discoverer.Discover()
	if err != nil {
		return 0, 0, errfmt.Newf("failed to discover markdown files").Wrap(err)
	}

	logging.Fluent(logger).Info("Discovered markdown files").
		Int("count", len(files)).
		Log()

	// Get existing entries
	existing, maxID, err := r.GetExistingEntries(ctx, profile)
	if err != nil {
		return 0, 0, errfmt.Newf("failed to get existing entries").Wrap(err)
	}

	logging.Fluent(logger).Info("Found existing doc_entries").
		Int("count", len(existing)).
		Int("max_id", maxID).
		Log()

	// Parse and register
	parser := NewParser()
	created := 0
	skipped = 0
	nextID := maxID + 1

	for _, file := range files {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return created, skipped, ctxErr
		}
		process.TouchMeaningfulActivity()

		// Skip if already exists
		key := paths.NormalizeDocEntryPathForKey(file.RelPath)
		if _, exists := existing[key]; exists {
			skipped++
			continue
		}
		if _, exists := existing[file.RelPath]; exists {
			skipped++
			continue
		}

		// Parse metadata
		metadata, err := parser.Parse(file.Path)
		if err != nil {
			logging.Fluent(logger).Warn("Failed to parse markdown file").
				Path(file.RelPath).
				WithError(err).
				Log()
			continue
		}

		// Generate ID
		docID := fmt.Sprintf("DOC-%03d", nextID)
		nextID++

		if dryRun {
			logging.Fluent(logger).Info("Would create doc_entry").
				ObjectID(docID).
				Path(file.RelPath).
				String("title", metadata.Title).
				Log()
			created++
			continue
		}

		// Create doc_entry
		err = r.CreateDocEntry(ctx, profile, file, metadata, docID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return created, skipped, ctxErr
			}
			logging.Fluent(logger).Warn("Failed to create doc_entry").
				ObjectID(docID).
				Path(file.RelPath).
				WithError(err).
				Log()
			continue
		}

		created++
		existing[key] = docID
		process.TouchMeaningfulActivity()
	}

	registered = created
	return registered, skipped, nil
}

// RegisterShippedDocs registers shipped documentation (architecture, best-practices, onboarding)
// during system init into the kernel doc_entry graph, skipping archive trees.
// Fails closed if any shipped markdown file in those trees fails to register or lacks a doc_entry.
func RegisterShippedDocs(ctx context.Context, projectRoot string, logger logging.Logger) (registered, skipped int, err error) {
	if projectRoot == emptyValue {
		return 0, 0, errfmt.Errorf("project root cannot be empty")
	}

	factory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return 0, 0, errfmt.Errorf("failed to initialize storage factory for docman: %w", err)
	}
	storageProvider := factory.GetStorageForKind(objects.KindDocEntry)
	registry := NewRegistry(storageProvider, projectRoot)

	profile := string(pkgctx.ProfileHuman)
	reg, skip, err := registry.RegisterSubtrees(ctx, profile, ShippedInitDocSubtrees, false)
	if err != nil {
		return reg, skip, errfmt.Errorf("failed to register shipped documentation: %w", err)
	}

	// Fail-closed verification: ensure every discovered markdown file in ShippedInitDocSubtrees has a doc_entry
	discoverer := NewDiscovererWithSubtrees(projectRoot, ShippedInitDocSubtrees)
	files, err := discoverer.Discover()
	if err != nil {
		return reg, skip, errfmt.Errorf("failed to scan shipped docs during post-init verification: %w", err)
	}
	if len(files) > 0 {
		existing, _, err := registry.GetExistingEntries(ctx, profile)
		if err != nil {
			return reg, skip, errfmt.Errorf("failed to list doc_entries during post-init verification: %w", err)
		}
		for _, f := range files {
			key := paths.NormalizeDocEntryPathForKey(f.RelPath)
			if _, ok := existing[key]; !ok {
				if _, ok2 := existing[paths.PathRefFromRelPath(f.RelPath)]; !ok2 && existing[f.RelPath] == "" {
					return reg, skip, errfmt.Errorf("fail-closed: shipped doc %q has no registered doc_entry in kernel graph", f.RelPath)
				}
			}
		}
	}

	return reg, skip, nil
}

// Helper function to validate YAML structure (for testing)
//
//nolint:unused // Test helper - reserved for future use
func validateDocEntryYAML(data []byte) error {
	var entry map[string]any
	return yaml.Unmarshal(data, &entry)
}
