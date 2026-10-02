package migration

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	objectSpecEnum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/object_spec"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// BundledObjectSpecMigrationStats summarizes EnsureBundledObjectSpecsMigrated (REQ-035 / CRIT-9035).
type BundledObjectSpecMigrationStats struct {
	Created int
	Updated int
	Skipped int
	Errors  int
}

// EnsureBundledObjectSpecsMigrated loads YAML files under .zqk/specs/objects and
// creates or updates corresponding object_spec rows in the active storage backend (file or graph).
// Idempotent: skips when storage already matches bundled file content for ontology/title/file_path/schema_version/visibility.
// Failures increment stats.Errors; the first error is returned unless all failures are logged-only (none — return last error).
func EnsureBundledObjectSpecsMigrated(ctx context.Context, projectRoot string, logger logging.Logger) (BundledObjectSpecMigrationStats, error) {
	var stats BundledObjectSpecMigrationStats
	if projectRoot == "" {
		return stats, errfmt.Errorf(storage.ConstMiscProjectRootIsEmpty)
	}
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := fileutil.Stat(specsDir); err != nil {
		if fileutil.IsNotExist(err) {
			return stats, nil
		}
		return stats, errfmt.Newf(storage.ConstMiscObjectSpecsDirectory).Wrap(err)
	}

	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)
	if err != nil {
		return stats, errfmt.Newf(storage.ConstMiscStorageFactory).Wrap(err)
	}
	if store == nil {
		return stats, errfmt.Errorf(storage.ConstMiscStorageProviderIsNil)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	migrateCtx := pkgctx.WithPromoteOnCreate(storage.WithSyncCreateForKind(ctx, objects.KindObjectSpec))

	var firstErr error

	walkErr := filepath.WalkDir(specsDir, func(path string, d fileutil.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if appledouble.SkipDirOrSidecar(d.IsDir(), path) {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		if strings.HasPrefix(name, "_") {
			return nil
		}

		data, err := fileutil.ReadFile(path)
		if err != nil {
			stats.Errors++
			if firstErr == nil {
				firstErr = errfmt.Errorf("read %s: %w", path, err)
			}
			if logger != nil {
				storage.StorageLog(logger).Warn(storage.LogEventStorageBundledSpecReadFailed).Path(path).WithError(err).Log()
			}
			return nil
		}

		want, err := buildObjectSpecObjectFromBundledFile(projectRoot, path, data)

		if err != nil {
			stats.Errors++
			if firstErr == nil {
				firstErr = errfmt.Errorf("%s: %w", path, err)
			}
			if logger != nil {
				storage.StorageLog(logger).Warn(storage.LogEventStorageBundledSpecBuildFailed).Path(path).WithError(err).Log()
			}
			return nil
		}
		if want == nil {
			return nil
		}
		id, _ := want[objects.FieldKeyID].(string)

		existing, rerr := store.Read(ctx, secCtx, id)
		if rerr != nil {
			if !errors.Is(rerr, storage.ErrObjectNotFound) && !isNotFoundErr(rerr) {
				stats.Errors++
				if firstErr == nil {
					firstErr = errfmt.Errorf("read %s: %w", id, rerr)
				}
				if logger != nil {
					storage.StorageLog(logger).Warn(storage.LogEventStorageBundledSpecReadStorageFailed).ObjectID(id).WithError(rerr).Log()
				}

				return nil
			}
			// Create mutates want's status to preliminary origin unless WithPromoteOnCreate is set.
			leaveStatus := objects.GetString(want, objects.FieldKeyStatus)
			if cerr := store.Create(migrateCtx, secCtx, want); cerr != nil {
				stats.Errors++
				if firstErr == nil {
					firstErr = errfmt.Errorf("create %s: %w", id, cerr)
				}
				if logger != nil {
					storage.StorageLog(logger).Warn(storage.LogEventStorageBundledSpecCreateFailed).ObjectID(id).WithError(cerr).Log()
				}

				return nil
			}
			createdStatus := objects.GetString(want, objects.FieldKeyStatus)
			if leaveStatus != "" && createdStatus != leaveStatus {
				// Create landed on preliminary origin; walk legal base_object hops
				// to the seeded leave status.
				if uerr := promoteBundledObjectSpecLeaveStatus(migrateCtx, store, secCtx, id, leaveStatus); uerr != nil {
					stats.Errors++
					if firstErr == nil {
						firstErr = errfmt.Errorf("promote %s → %s: %w", id, leaveStatus, uerr)
					}
					if logger != nil {
						storage.StorageLog(logger).Warn(storage.LogEventStorageBundledSpecUpdateFailed).ObjectID(id).WithError(uerr).Log()
					}
					return nil
				}
				want[objects.FieldKeyStatus] = leaveStatus
			}
			stats.Created++

			return nil
		}

		if bundledObjectSpecMatches(existing, want) {
			stats.Skipped++

			return nil
		}

		updates := map[string]any{
			objects.FieldKeyTitle:         want[objects.FieldKeyTitle],
			objects.FieldKeyOntology:      want[objects.FieldKeyOntology],
			objects.FieldKeyFilePath:      want[objects.FieldKeyFilePath],
			objects.FieldKeySchemaVersion: want[objects.FieldKeySchemaVersion],
			objects.FieldKeyStatus:        want[objects.FieldKeyStatus],
			objects.FieldKeySourceType:    want[objects.FieldKeySourceType],
		}
		if v := objects.GetString(want, objects.FieldKeyVisibility); v != "" {
			updates[objects.FieldKeyVisibility] = v
		}
		if uerr := store.Update(migrateCtx, secCtx, id, updates); uerr != nil {
			stats.Errors++
			if firstErr == nil {
				firstErr = errfmt.Errorf("update %s: %w", id, uerr)
			}
			if logger != nil {
				storage.StorageLog(logger).Warn(storage.LogEventStorageBundledSpecUpdateFailed).ObjectID(id).WithError(uerr).Log()
			}

			return nil
		}
		stats.Updated++

		return nil
	})

	if walkErr != nil {
		return stats, walkErr
	}
	if firstErr != nil && stats.Created == 0 && stats.Updated == 0 && stats.Skipped == 0 {
		return stats, firstErr
	}
	if logger != nil && (stats.Created > 0 || stats.Updated > 0) {
		storage.StorageLog(logger).Info(storage.LogEventStorageBundledSpecFinishedInfo).
			Int("created", stats.Created).
			Int("updated", stats.Updated).
			Int("skipped", stats.Skipped).
			Int("errors", stats.Errors).
			Log()
	}
	return stats, nil
}

// promoteBundledObjectSpecLeaveStatus walks proposed → approved → in_progress → implemented
// (base_object; object_spec has no kind lifecycle). One-hop Update is required.
func promoteBundledObjectSpecLeaveStatus(ctx context.Context, store storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, id, leave string) error {
	if leave == "" {
		return nil
	}
	hops := []string{
		objects.ObjectStatusApproved,
		objects.ObjectStatusInProgress,
		objects.ObjectStatusImplemented,
	}
	current, err := store.Read(ctx, secCtx, id)
	curStatus := ""
	if err == nil {
		curStatus = objects.GetString(current, objects.FieldKeyStatus)
		if curStatus == leave {
			return nil
		}
	}
	started := curStatus == "" || curStatus == objects.ObjectStatusProposed
	for _, hop := range hops {
		if !started {
			if hop == curStatus {
				started = true
			}
			continue
		}
		if err := store.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyStatus: hop}); err != nil {
			return err
		}
		if hop == leave {
			return nil
		}
	}
	return nil
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "not found") || strings.Contains(s, "ID not found")
}

var baseObjectIDRegex = regexp.MustCompile(`^[A-Z]+-\d{3,}$`)

// ObjectSpecIDForStem computes a deterministic, spec-compliant ID (^[A-Z]+-\d{3,}$) for a bundled spec file stem.
func ObjectSpecIDForStem(stem string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(stem))
	num := (h.Sum32() % 900000) + 100000
	return fmt.Sprintf("OBJ-%06d", num)
}

// ValidateObjectSpecInstance verifies that an object_spec meets base_object and object_spec schema rules.
func ValidateObjectSpecInstance(obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	if !baseObjectIDRegex.MatchString(id) {
		return errfmt.Errorf("object_spec ID %q does not match ^[A-Z]+-\\d{3,}$", id)
	}
	desc, _ := obj[objects.FieldKeyDescription].(string)
	if len(strings.TrimSpace(desc)) < 10 {
		return errfmt.Errorf("object_spec description %q violates min_length 10", desc)
	}
	filePath, _ := obj[objects.FieldKeyFilePath].(string)
	if filePath == "" {
		return errfmt.Errorf("object_spec file_path is required")
	}
	ontology, _ := obj[objects.FieldKeyOntology].(string)
	if ontology == "" {
		return errfmt.Errorf("object_spec ontology is required")
	}
	sourceType, _ := obj[objects.FieldKeySourceType].(string)
	if sourceType != "internal" && sourceType != "external" && sourceType != "imported" {
		return errfmt.Errorf("object_spec source_type %q violates base_object enum", sourceType)
	}
	return nil
}

func buildObjectSpecObjectFromBundledFile(projectRoot, absPath string, data []byte) (map[string]any, error) {
	var specDef map[string]any
	if err := yaml.Unmarshal(data, &specDef); err != nil {
		return nil, err
	}
	ontology, _ := specDef[objects.FieldKeyOntology].(string)
	if ontology == "" {
		return nil, nil
	}

	stem := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	id := ObjectSpecIDForStem(stem)

	sv := objects.DefaultSchemaVersion
	if v := objects.GetString(specDef, objects.FieldKeySchemaVersion); v != "" {
		sv = objects.ValidSchemaVersion(v)
	}

	absPath = filepath.Clean(absPath)
	relPath := absPath
	if projectRoot != "" {
		if r, err := filepath.Rel(projectRoot, absPath); err == nil && !strings.HasPrefix(r, "..") {
			relPath = r
		}
	}
	if filepath.IsAbs(relPath) {
		if idx := strings.Index(absPath, paths.ProcessInternalObjectSpecsDir); idx >= 0 {
			relPath = absPath[idx:]
		}
	}

	title := titleFromBundledSpec(specDef, ontology)
	if len(title) < 5 {
		title = fmt.Sprintf(storage.ConstMiscSSpecification, ontology)
	}

	desc := objects.GetString(specDef, objects.FieldKeyDescription)
	if desc == "" {
		desc = fmt.Sprintf("Object specification definition for %s.", ontology)
	} else if len(strings.TrimSpace(desc)) < 10 {
		desc = desc + " specification"
	}

	b := instance_builders.NewForKind(objects.KindObjectSpec, sv)
	b.SetID(id)
	b.SetStatus(string(objectSpecEnum.StatusImplemented))
	b.SetField(objects.FieldKeyTitle, title)
	b.SetField(objects.FieldKeyDescription, desc)
	b.SetField(objects.FieldKeyOntology, ontology)
	b.SetField(objects.FieldKeyFilePath, relPath)
	b.SetField(objects.FieldKeySourceType, string(objectSpecEnum.SourceTypeInternal))
	b.SetField(objects.FieldKeyNamespaceID, "zqk:kernel")
	b.SetField(objects.FieldKeyOriginProject, "zqk")
	b.SetField(objects.FieldKeyOriginSystem, "zqk")
	if v := objects.GetString(specDef, objects.FieldKeyVisibility); v != "" {
		b.SetField(objects.FieldKeyVisibility, v)
	}

	obj, err := b.Build()
	if err != nil {
		return nil, err
	}

	if err := ValidateObjectSpecInstance(obj); err != nil {
		return nil, err
	}

	return obj, nil
}

func titleFromBundledSpec(specDef map[string]any, ontology string) string {
	title := fmt.Sprintf(storage.ConstMiscSSpecification1, ontology)
	if desc := objects.GetString(specDef, objects.FieldKeyDescription); desc != "" {
		first := strings.TrimSpace(strings.Split(desc, "\n")[0])
		if first != "" {
			if len(first) > 80 {
				title = first[:77] + "..."
			} else {
				title = first
			}
		}
	}
	return title
}

func bundledObjectSpecMatches(existing, want map[string]any) bool {
	keys := []string{"ontology", "title", "file_path", storage.ConstMiscSchemaVersion, "status", "source_type"}
	for _, k := range keys {
		if fmt.Sprint(existing[k]) != fmt.Sprint(want[k]) {
			return false
		}
	}
	ev, eOk := existing[objects.FieldKeyVisibility].(string)
	wv, wOk := want[objects.FieldKeyVisibility].(string)
	if eOk != wOk || ev != wv {
		return false
	}
	return true
}
