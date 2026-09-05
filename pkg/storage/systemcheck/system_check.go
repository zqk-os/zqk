package systemcheck

import (
	"context"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/pipeline"
)

// HandCASSystemCheckResult captures results from a hand-CAS + duplicate-ID scan.
type HandCASSystemCheckResult struct {
	Kind        string
	KindsDir    string
	Duplicates  []DuplicateIDEntry // IDs appearing in >1 path
	OrphanPaths []string           // Paths that lack an ID field or can't be parsed
	Violations  int                // Total violations (duplicates+orphans)
}

// DuplicateIDEntry represents a detected duplicate-ID scenario.
type DuplicateIDEntry struct {
	ObjectID string   // The duplicated object ID
	Paths    []string // All paths pointing to this ID
}

// HandCASSystemCheckError wraps structured errors reported by hand-CAS checks.
type HandCASSystemCheckError struct {
	Kind        string             `json:"kind"`
	KindsDir    string             `json:"kinds_dir"`
	Duplicates  []DuplicateIDEntry `json:"duplicates"`
	OrphanPaths []string           `json:"orphan_paths"`
}

func (e *HandCASSystemCheckError) Error() string {
	return errfmt.Errorf("hand-CAS check found violations in kind %q: %d duplicate IDs, %d orphan paths", e.Kind, len(e.Duplicates), len(e.OrphanPaths)).Error()
}

// handCASScanPipelineKind is the pipeline kind for the read-only hand-CAS scan
// (observable through the standard pipeline stage metrics).
const handCASScanPipelineKind = "storage.hand_cas_dup_id_scan"

// handCASScanPayload carries scan state between pipeline stages.
type handCASScanPayload struct {
	kind        string
	kindDir     string
	casFiles    []string            // CAS-hash-named files found in kindDir
	idToPaths   map[string][]string // embedded object ID -> file paths carrying it
	orphanPaths []string            // CAS files with no parseable embedded ID
}

// RunSystemCheckForHandCASAndDupIDs scans kindDir for CAS YAML files and detects:
//   - Hand-CAS edits (multiple file paths embedding the same object ID)
//   - Duplicate-orphan IDs (same ID pointing to different content)
//
// The scan runs through [pipeline.Builder] (INGEST list CAS files → NORMALIZE peek embedded
// IDs → FINALIZE collect duplicates) so each phase is individually observable.
// It does NOT modify any state — it is purely read-only diagnostic.
func RunSystemCheckForHandCASAndDupIDs(ctx context.Context, kindDir string, kind string) (*HandCASSystemCheckResult, error) {
	if kindDir == "" || kind == "" {
		return nil, errfmt.Errorf("kindDir and kind must be non-empty")
	}
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	out, err := pipeline.NewBuilder(handCASScanPipelineKind, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, handCASScanStageListCASFiles).
		AddStage(pipeline.StageNormalize, handCASScanStagePeekObjectIDs).
		AddStage(pipeline.StageFinalize, handCASScanStageCollectViolations).
		Build().
		Run(&pipeline.Context{Ctx: ctx}, &handCASScanPayload{kind: kind, kindDir: kindDir})
	if err != nil {
		return nil, err
	}

	result, ok := out.(*HandCASSystemCheckResult)
	if !ok {
		return nil, errfmt.Errorf("hand-CAS scan pipeline returned unexpected payload type %T", out)
	}
	return result, nil
}

// handCASScanStageListCASFiles lists the CAS-hash-named files in the kind directory.
func handCASScanStageListCASFiles(_ *pipeline.Context, payload any) (any, error) {
	scan, err := handCASScanPayloadFrom(payload)
	if err != nil {
		return nil, err
	}
	entries, err := fileutil.ReadDir(scan.kindDir)
	if err != nil {
		return nil, errfmt.Errorf("failed to read kind dir %q: %w", scan.kindDir, err)
	}
	for _, entry := range entries {
		if name := entry.Name(); filecas.CasHashFilenameRe.MatchString(name) {
			scan.casFiles = append(scan.casFiles, filepath.Join(scan.kindDir, name))
		}
	}
	return scan, nil
}

// handCASScanStagePeekObjectIDs groups CAS files by their embedded object ID. A missing or
// unparseable ID means the path is an orphan / hand-CAS artifact (CAS blobs always embed id).
func handCASScanStagePeekObjectIDs(_ *pipeline.Context, payload any) (any, error) {
	scan, err := handCASScanPayloadFrom(payload)
	if err != nil {
		return nil, err
	}
	scan.idToPaths = make(map[string][]string, len(scan.casFiles))
	for _, path := range scan.casFiles {
		id := filecas.CasHashFilePeekObjectID(path)
		if id == "" {
			scan.orphanPaths = append(scan.orphanPaths, path)
			continue
		}
		scan.idToPaths[id] = append(scan.idToPaths[id], path)
	}
	return scan, nil
}

// handCASScanStageCollectViolations turns grouped IDs into the caller-facing result: every ID
// that appears in more than one path is a hand-CAS violation.
func handCASScanStageCollectViolations(_ *pipeline.Context, payload any) (any, error) {
	scan, err := handCASScanPayloadFrom(payload)
	if err != nil {
		return nil, err
	}
	var duplicates []DuplicateIDEntry
	for id, dupPaths := range scan.idToPaths {
		if len(dupPaths) > 1 {
			duplicates = append(duplicates, DuplicateIDEntry{ObjectID: id, Paths: dupPaths})
		}
	}
	return &HandCASSystemCheckResult{
		Kind:        scan.kind,
		KindsDir:    scan.kindDir,
		Duplicates:  duplicates,
		OrphanPaths: scan.orphanPaths,
		Violations:  len(duplicates) + len(scan.orphanPaths),
	}, nil
}

func handCASScanPayloadFrom(payload any) (*handCASScanPayload, error) {
	scan, ok := payload.(*handCASScanPayload)
	if !ok || scan == nil {
		return nil, errfmt.Errorf("hand-CAS scan stage received unexpected payload type %T", payload)
	}
	return scan, nil
}

// RunSystemCheckForAllKinds scans ALL registered kinds' CAS directories for each.
// BatchRunSystemCheckForHandCASAndDupIDs runs a parallel system check across many CAS
// directories at once. Fan-out and the channel closer both run through
// [goroutinelabels.NewGoroutine] so they are labeled for profiling and panic-safe.
func BatchRunSystemCheckForHandCASAndDupIDs(ctx context.Context, kinds []string, kindDirResolver func(string) string) (<-chan *HandCASSystemCheckResult, <-chan error) {
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}
	results := make(chan *HandCASSystemCheckResult, len(kinds))
	errors := make(chan error, len(kinds))

	var wg sync.WaitGroup
	for _, kind := range kinds {
		k := kind
		goroutinelabels.NewGoroutine(handCASScanGoroutineName, handCASScanGoroutinePurpose).
			WithWaitGroup(&wg).
			WithErrorHandler(func(err error) { errors <- err }).
			StartWithContext(ctx, func(scanCtx context.Context) error {
				kindDir := ""
				if kindDirResolver != nil {
					kindDir = kindDirResolver(k)
				}
				res, err := RunSystemCheckForHandCASAndDupIDs(scanCtx, kindDir, k)
				if err != nil {
					return errfmt.Errorf("check for kind %q failed: %w", k, err)
				}
				results <- res
				return nil
			})
	}

	goroutinelabels.NewGoroutine(handCASScanCloserName, handCASScanCloserPurpose).
		WithCleanup(func() {
			close(results)
			close(errors)
		}).
		StartSimple(wg.Wait)

	return results, errors
}

const (
	handCASScanGoroutineName    = "hand_cas_scan_kind"
	handCASScanGoroutinePurpose = "scanning one CAS kind directory for hand-CAS duplicate IDs"
	handCASScanCloserName       = "hand_cas_scan_closer"
	handCASScanCloserPurpose    = "closing hand-CAS scan result/error channels once all kinds finish"
)
