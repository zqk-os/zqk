package migration

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/storage"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func (m *DSIAMigrationUtility) MigrateObjectToDSIA(ctx context.Context, secCtx *pkgctx.SecurityContext, kind, hash, filePath string, removeOldFile bool) error {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return err
	}
	dsia := storage.NewDSIAStorageProvider(m.facade.GetProcessDir())

	// Create might fail if object exists, but we can try Update as fallback
	if err := dsia.Create(ctx, secCtx, obj); err != nil {
		if err.Error() == "object already exists" {
			// Get ID
			id, _ := obj[objects.FieldKeyID].(string)
			if id != "" {
				if updateErr := dsia.Update(ctx, secCtx, id, obj); updateErr != nil {
					return updateErr
				}
			}
		} else {
			return err
		}
	}

	if removeOldFile {
		_ = fileutil.Remove(filePath)
	}
	return nil
}

func (m *DSIAMigrationUtility) MigrateKindToDSIA(ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, removeOldFiles bool) (int, []error) {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		return 0, nil
	}
	kindDir := filepath.Join(m.facade.GetProcessDir(), dirName)

	hashFiles := m.scanHashBasedFilesRecursive(kindDir, kind)
	migrated := 0
	var errs []error
	for hash, filePath := range hashFiles {
		err := m.MigrateObjectToDSIA(ctx, secCtx, kind, hash, filePath, removeOldFiles)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to migrate hash %s: %w", hash, err))
		} else {
			migrated++
		}
	}
	return migrated, errs
}

func (m *DSIAMigrationUtility) scanHashBasedFilesRecursive(kindDir, kind string) map[string]string {
	hashToPath := make(map[string]string)
	hashPattern := regexp.MustCompile(`^[a-f0-9]{64}\.yaml$`)

	_ = filepath.Walk(kindDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if hashPattern.MatchString(filepath.Base(path)) {
			hash := strings.TrimSuffix(filepath.Base(path), ".yaml")
			hashToPath[hash] = path
		}
		return nil
	})
	return hashToPath
}

func (m *DSIAMigrationUtility) MigrateAllToDSIA(ctx context.Context, secCtx *pkgctx.SecurityContext, removeOldFiles bool) (map[string]int, map[string][]error) {
	kinds := m.facade.DiscoverObjectKinds()
	migratedByKind := make(map[string]int)
	errorsByKind := make(map[string][]error)

	for _, kind := range kinds {
		migrated, errs := m.MigrateKindToDSIA(ctx, secCtx, kind, removeOldFiles)
		if migrated > 0 {
			migratedByKind[kind] = migrated
		}
		if len(errs) > 0 {
			errorsByKind[kind] = errs
		}
	}
	return migratedByKind, errorsByKind
}
