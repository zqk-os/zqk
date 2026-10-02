package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type DSIAStorageProvider struct {
	baseDir string
	mu      sync.RWMutex
}

// NewDSIAStorageProvider creates a new Deterministic Stable-Identity Storage provider
func NewDSIAStorageProvider(baseDir ...string) *DSIAStorageProvider {
	dir := paths.ProcessDir
	if len(baseDir) > 0 && baseDir[0] != "" {
		dir = baseDir[0]
	}
	return &DSIAStorageProvider{baseDir: dir}
}

// AtomicWriteFile writes data to a temporary file in the same directory as the target file
// and then renames it atomically to the target file path.
func (p *DSIAStorageProvider) AtomicWriteFile(filePath string, data []byte, perm fileutil.FileMode) error {
	if err := fileutil.WriteDurableFile(filePath, data, perm); err != nil {
		return errfmt.Newf("failed durable atomic write to %s", filePath).Wrap(err)
	}
	if _, statErr := fileutil.Stat(filePath); statErr != nil {
		return fmt.Errorf("AtomicWriteFile: Rename succeeded but Stat failed! path=%s, err=%w", filePath, statErr)
	}
	return nil
}

func (p *DSIAStorageProvider) getPath(kind, objectID string) string {
	return filepath.Join(p.baseDir, kind+"s", objectID+".yaml")
}

func (p *DSIAStorageProvider) writeWithChecksumAndRename(targetPath string, obj map[string]any) error {
	// First calculate checksum without the checksum field
	delete(obj, "sha256_checksum")

	// Ensure map is consistently ordered for hashing by marshaling to YAML
	data, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])

	// Embed checksum
	obj["sha256_checksum"] = checksum

	// Marshal again with checksum
	finalData, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}

	return p.AtomicWriteFile(targetPath, finalData, fileutil.StandardFilePerm)
}

func (p *DSIAStorageProvider) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok {
		return fmt.Errorf("kind is required")
	}
	objectID, ok := obj[objects.FieldKeyID].(string)
	if !ok {
		return fmt.Errorf("id is required")
	}

	targetPath := p.getPath(kind, objectID)

	// Check if already exists
	if _, err := fileutil.Stat(targetPath); err == nil {
		return fmt.Errorf("object already exists")
	}

	return p.writeWithChecksumAndRename(targetPath, obj)
}

func (p *DSIAStorageProvider) findObjectPath(id string) string {
	entries, err := fileutil.ReadDir(p.baseDir)
	if err != nil {
		return ""
	}
	targetFile := id + ".yaml"
	for _, entry := range entries {
		if entry.IsDir() {
			potentialPath := filepath.Join(p.baseDir, entry.Name(), targetFile)
			if _, err := fileutil.Stat(potentialPath); err == nil {
				return potentialPath
			}
		}
	}
	return ""
}

func (p *DSIAStorageProvider) Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	foundPath := p.findObjectPath(id)
	if foundPath == "" {
		return nil, ErrObjectNotFound
	}

	data, err := fileutil.ReadFile(foundPath)
	if err != nil {
		return nil, err
	}

	return ParseStreamBackedCurrentState(data)
}

func (p *DSIAStorageProvider) Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	kind, ok := updates[objects.FieldKeyKind].(string)
	if !ok {
		return fmt.Errorf("kind is required")
	}

	targetPath := p.getPath(kind, id)

	if _, err := fileutil.Stat(targetPath); fileutil.IsNotExist(err) {
		return fmt.Errorf("object not found")
	}

	return p.writeWithChecksumAndRename(targetPath, updates)
}

func (p *DSIAStorageProvider) Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	foundPath := p.findObjectPath(id)
	if foundPath == "" {
		return ErrObjectNotFound
	}

	return fileutil.Remove(foundPath)
}

// Unimplemented methods to satisfy ObjectStorageProvider

func (tx *DSIATransaction) ensureActive() error {
	if !tx.active {
		return fmt.Errorf("transaction inactive")
	}
	return nil
}

func (tx *DSIATransaction) withActiveLock(fn func() error) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if err := tx.ensureActive(); err != nil {
		return err
	}
	return fn()
}

func (tx *DSIATransaction) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	return tx.withActiveLock(func() error {
		id, ok := obj[objects.FieldKeyID].(string)
		if !ok {
			return fmt.Errorf("id is required")
		}
		tx.staged[id] = obj
		delete(tx.deleted, id)
		return nil
	})
}

func (tx *DSIATransaction) Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error) {
	var obj map[string]any
	var fromProvider bool
	err := tx.withActiveLock(func() error {
		if tx.deleted[id] {
			return ErrObjectNotFound
		}
		if staged, exists := tx.staged[id]; exists {
			obj = staged
			return nil
		}
		fromProvider = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fromProvider {
		return tx.provider.Read(ctx, secCtx, id)
	}
	return obj, nil
}

func (tx *DSIATransaction) Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error {
	return tx.withActiveLock(func() error {
		tx.staged[id] = updates
		delete(tx.deleted, id)
		return nil
	})
}

func (tx *DSIATransaction) Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error {
	return tx.withActiveLock(func() error {
		tx.deleted[id] = true
		delete(tx.staged, id)
		return nil
	})
}

func (tx *DSIATransaction) Commit(ctx context.Context) error {
	return tx.withActiveLock(func() error {
		// A failed commit may already have applied an earlier operation. Make the
		// transaction terminal so callers cannot accidentally replay a partial
		// commit and compound the inconsistency.
		tx.active = false
		for id := range tx.deleted {
			if err := tx.provider.Delete(ctx, nil, id, false); err != nil {
				return errfmt.Newf("commit delete %s", id).Wrap(err)
			}
		}
		for _, obj := range tx.staged {
			id, ok := obj[objects.FieldKeyID].(string)
			if !ok || id == "" {
				return fmt.Errorf("commit staged object: id is required")
			}
			exists, err := tx.provider.Exists(ctx, nil, id)
			if err != nil {
				return errfmt.Newf("commit check existence %s", id).Wrap(err)
			}
			if exists {
				if err := tx.provider.Update(ctx, nil, id, obj); err != nil {
					return errfmt.Newf("commit update %s", id).Wrap(err)
				}
				continue
			}
			if err := tx.provider.Create(ctx, nil, obj); err != nil {
				return errfmt.Newf("commit create %s", id).Wrap(err)
			}
		}
		return nil
	})
}

func (tx *DSIATransaction) Rollback(ctx context.Context) error {
	return tx.withActiveLock(func() error {
		tx.staged = nil
		tx.deleted = nil
		tx.active = false
		return nil
	})
}

func (p *DSIAStorageProvider) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return &DSIATransaction{
		provider: p,
		staged:   make(map[string]map[string]any),
		deleted:  make(map[string]bool),
		active:   true,
	}, nil
}

func (p *DSIAStorageProvider) Exists(ctx context.Context, secCtx *SecurityContext, id string) (bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.findObjectPath(id) != "", nil
}

func (p *DSIAStorageProvider) Count(ctx context.Context, secCtx *SecurityContext, filter ListFilter) (int, error) {
	res, err := p.List(ctx, secCtx, nil, filter)
	if err != nil {
		return 0, err
	}
	return len(res.Objects), nil
}

func (p *DSIAStorageProvider) Move(ctx context.Context, secCtx *SecurityContext, id string, newKind string, updateReferences bool) error {
	obj, err := p.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}
	if err := p.Delete(ctx, secCtx, id, false); err != nil {
		return err
	}
	obj[objects.FieldKeyKind] = newKind
	return p.Create(ctx, secCtx, obj)
}

func (p *DSIAStorageProvider) Rename(ctx context.Context, secCtx *SecurityContext, oldID, newID string, updateReferences bool) error {
	obj, err := p.Read(ctx, secCtx, oldID)
	if err != nil {
		return err
	}
	if err := p.Delete(ctx, secCtx, oldID, false); err != nil {
		return err
	}
	obj[objects.FieldKeyID] = newID
	return p.Create(ctx, secCtx, obj)
}

func (p *DSIAStorageProvider) Shutdown(ctx context.Context) error {
	return nil
}
