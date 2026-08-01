package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	objectSpecEnum "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/object_spec"
	bldr_instance_v1 "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"gopkg.in/yaml.v3"
)

// BundledObjectSpecMigrationStats summarizes EnsureBundledObjectSpecsMigrated (REQ-035 / CRIT-9035).
type BundledObjectSpecMigrationStats struct {
	Created int
	Updated int
	Skipped int
	Errors  int
}

// EnsureBundledObjectSpecsMigrated loads YAML files under docs/architecture/_internal/object_specs and
// creates or updates corresponding object_spec rows in the active storage backend (file or graph).
// Idempotent: skips when storage already matches bundled file content for ontology/title/file_path/schema_version/visibility.
// Failures increment stats.Errors; the first error is returned unless all failures are logged-only (none — return last error).
func EnsureBundledObjectSpecsMigrated(ctx context.Context, projectRoot string, logger logging.Logger) (BundledObjectSpecMigrationStats, error) {
	var stats BundledObjectSpecMigrationStats
	if projectRoot == emptyValue {
		return stats, errfmt.Errorf(ConstMiscProjectRootIsEmpty)
	}
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := os.Stat(specsDir); err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return stats, errfmt.Newf(ConstMiscObjectSpecsDirectory).Wrap(err)
	}

	store, err := GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)
	if err != nil {
		return stats, errfmt.Newf(ConstMiscStorageFactory).Wrap(err)
	}
	if store == nil {
		return stats, errfmt.Errorf(ConstMiscStorageProviderIsNil)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	migrateCtx := WithSyncCreateForKind(ctx, objects.KindObjectSpec)

	var firstErr error

	walkErr := filepath.WalkDir(specsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		if strings.HasPrefix(name, "_") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			stats.Errors++
			if firstErr == nil {
				firstErr = errfmt.Errorf("read %s: %w", path, err)
			}
			if logger != nil {
				StorageLog(logger).Warn(LogEventStorageBundledSpecReadFailed).Path(path).WithError(err).Log()
			}
			return nil
		}

		want, err := buildObjectSpecObjectFromBundledFile(path, data)

		if err != nil {
			stats.Errors++
			if firstErr == nil {
				firstErr = errfmt.Errorf("%s: %w", path, err)
			}
			if logger != nil {
				StorageLog(logger).Warn(LogEventStorageBundledSpecBuildFailed).Path(path).WithError(err).Log()
			}
			return nil
		}
		if want == nil {
			return nil
		}
		id, _ := want[objects.FieldKeyID].(string)

		existing, rerr := store.Read(ctx, secCtx, id)
		if rerr != nil {
			if !errors.Is(rerr, ErrObjectNotFound) && !isNotFoundErr(rerr) {
				stats.Errors++
				if firstErr == nil {
					firstErr = errfmt.Errorf("read %s: %w", id, rerr)
				}
				if logger != nil {
					StorageLog(logger).Warn(LogEventStorageBundledSpecReadStorageFailed).ObjectID(id).WithError(rerr).Log()
				}

				return nil
			}
			// Create
			if cerr := store.Create(migrateCtx, secCtx, want); cerr != nil {
				stats.Errors++
				if firstErr == nil {
					firstErr = errfmt.Errorf("create %s: %w", id, cerr)
				}
				if logger != nil {
					StorageLog(logger).Warn(LogEventStorageBundledSpecCreateFailed).ObjectID(id).WithError(cerr).Log()
				}

				return nil
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
		if v := objects.GetString(want, objects.FieldKeyVisibility); v != emptyValue {
			updates[objects.FieldKeyVisibility] = v
		}
		if uerr := store.Update(migrateCtx, secCtx, id, updates); uerr != nil {
			stats.Errors++
			if firstErr == nil {
				firstErr = errfmt.Errorf("update %s: %w", id, uerr)
			}
			if logger != nil {
				StorageLog(logger).Warn(LogEventStorageBundledSpecUpdateFailed).ObjectID(id).WithError(uerr).Log()
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
		StorageLog(logger).Info(LogEventStorageBundledSpecFinishedInfo).
			Int("created", stats.Created).
			Int("updated", stats.Updated).
			Int("skipped", stats.Skipped).
			Int("errors", stats.Errors).
			Log()
	}
	return stats, nil
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "not found") || strings.Contains(s, "ID not found")
}

func buildObjectSpecObjectFromBundledFile(absPath string, data []byte) (map[string]any, error) {
	var specDef map[string]any
	if err := yaml.Unmarshal(data, &specDef); err != nil {
		return nil, err
	}
	ontology, _ := specDef[objects.FieldKeyOntology].(string)
	if ontology == emptyValue {
		return nil, nil
	}

	stem := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	id := "OBJ-" + stem

	sv := objects.DefaultSchemaVersion
	if v := objects.GetString(specDef, objects.FieldKeySchemaVersion); v != emptyValue {
		sv = objects.ValidSchemaVersion(v)
	}

	absPath = filepath.Clean(absPath)
	title := titleFromBundledSpec(specDef, ontology)
	if len(title) < 5 {
		title = fmt.Sprintf(ConstMiscSSpecification, ontology)
	}
	if len(title) > 120 {
		title = title[:117] + "..."
	}

	b := bldr_instance_v1.NewObjectSpecInstanceBuilder(sv)
	b.ID(id)
	b.Status(objectSpecEnum.StatusImplemented)
	b.SetField(objects.FieldKeyTitle, title)
	b.Ontology(ontology)
	b.FilePath(absPath)
	b.SourceType(objectSpecEnum.SourceTypeInternal)
	b.SetField(objects.FieldKeyNamespaceID, "zqk:kernel")
	b.SetField(objects.FieldKeyOriginProject, "zqk")
	b.SetField(objects.FieldKeyOriginSystem, "zqk")
	if v := objects.GetString(specDef, objects.FieldKeyVisibility); v != emptyValue {
		b.SetField(objects.FieldKeyVisibility, v)
	}

	obj, err := b.Build()
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func titleFromBundledSpec(specDef map[string]any, ontology string) string {
	title := fmt.Sprintf(ConstMiscSSpecification1, ontology)
	if desc := objects.GetString(specDef, objects.FieldKeyDescription); desc != emptyValue {
		first := strings.TrimSpace(strings.Split(desc, "\n")[0])
		if first != emptyValue {
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
	keys := []string{"ontology", "title", "file_path", ConstMiscSchemaVersion, "status", "source_type"}
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
