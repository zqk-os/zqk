package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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
	dir := filepath.Dir(filePath)
	if err := fileutil.MkdirAll(dir, 0755); err != nil {
		return errfmt.Newf("failed to create directory %s", dir).Wrap(err)
	}
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return errfmt.Newf("failed to generate random suffix").Wrap(err)
	}
	tempFilePath := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%s", filepath.Base(filePath), hex.EncodeToString(randBytes)))
	if err := fileutil.WriteFile(tempFilePath, data, perm); err != nil {
		return errfmt.Newf("failed to write temp file %s", tempFilePath).Wrap(err)
	}
	if err := fileutil.Rename(tempFilePath, filePath); err != nil {
		_ = fileutil.Remove(tempFilePath)
		return errfmt.Newf("failed to rename temp file to target %s", filePath).Wrap(err)
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

	dir := filepath.Dir(targetPath)
	if err := fileutil.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := targetPath + fmt.Sprintf(".%d.tmp", time.Now().UnixNano())
	if err := fileutil.WriteFile(tmpPath, finalData, 0644); err != nil {
		return err
	}

	// Atomic rename
	if err := fileutil.Rename(tmpPath, targetPath); err != nil {
		_ = fileutil.Remove(tmpPath)
		return err
	}
	return nil
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

func (p *DSIAStorageProvider) Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Need a way to find kind... for now, we'll scan kinds or require it in a real implementation
	// Assuming a simplified find or require kind as an extension. We'll search in subdirs of baseDir.
	var foundPath string
	entries, err := fileutil.ReadDir(p.baseDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				potentialPath := filepath.Join(p.baseDir, entry.Name(), id+".yaml")
				if _, err := fileutil.Stat(potentialPath); err == nil {
					foundPath = potentialPath
					break
				}
			}
		}
	}

	if foundPath == "" {
		return nil, ErrObjectNotFound
	}

	data, err := fileutil.ReadFile(foundPath)
	if err != nil {
		return nil, err
	}

	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
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

	var foundPath string
	entries, err := fileutil.ReadDir(p.baseDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				potentialPath := filepath.Join(p.baseDir, entry.Name(), id+".yaml")
				if _, err := fileutil.Stat(potentialPath); err == nil {
					foundPath = potentialPath
					break
				}
			}
		}
	}

	if foundPath == "" {
		return ErrObjectNotFound
	}

	return fileutil.Remove(foundPath)
}

// Unimplemented methods to satisfy ObjectStorageProvider

func (tx *DSIATransaction) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.active {
		return fmt.Errorf("transaction inactive")
	}
	id, ok := obj[objects.FieldKeyID].(string)
	if !ok {
		return fmt.Errorf("id is required")
	}
	tx.staged[id] = obj
	delete(tx.deleted, id)
	return nil
}

func (tx *DSIATransaction) Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.active {
		return nil, fmt.Errorf("transaction inactive")
	}
	if tx.deleted[id] {
		return nil, ErrObjectNotFound
	}
	if obj, exists := tx.staged[id]; exists {
		return obj, nil
	}
	return tx.provider.Read(ctx, secCtx, id)
}

func (tx *DSIATransaction) Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.active {
		return fmt.Errorf("transaction inactive")
	}
	tx.staged[id] = updates
	delete(tx.deleted, id)
	return nil
}

func (tx *DSIATransaction) Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.active {
		return fmt.Errorf("transaction inactive")
	}
	tx.deleted[id] = true
	delete(tx.staged, id)
	return nil
}

func (tx *DSIATransaction) Commit(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.active {
		return fmt.Errorf("transaction inactive")
	}
	for id := range tx.deleted {
		_ = tx.provider.Delete(ctx, nil, id, false)
	}
	for _, obj := range tx.staged {
		id, _ := obj[objects.FieldKeyID].(string)
		if exists, _ := tx.provider.Exists(ctx, nil, id); exists {
			_ = tx.provider.Update(ctx, nil, id, obj)
		} else {
			_ = tx.provider.Create(ctx, nil, obj)
		}
	}
	tx.active = false
	return nil
}

func (tx *DSIATransaction) Rollback(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.active {
		return fmt.Errorf("transaction inactive")
	}
	tx.staged = nil
	tx.deleted = nil
	tx.active = false
	return nil
}

func (p *DSIAStorageProvider) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return &DSIATransaction{
		provider: p,
		staged:   make(map[string]map[string]any),
		deleted:  make(map[string]bool),
		active:   true,
	}, nil
}

// TRACK: [REDACTED-ID] — param renamed objs so FieldKey package objects is not shadowed after AST -write.

func (p *DSIAStorageProvider) Exists(ctx context.Context, secCtx *SecurityContext, id string) (bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var foundPath string
	entries, err := fileutil.ReadDir(p.baseDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				potentialPath := filepath.Join(p.baseDir, entry.Name(), id+".yaml")
				if _, err := fileutil.Stat(potentialPath); err == nil {
					foundPath = potentialPath
					break
				}
			}
		}
	}

	return foundPath != "", nil
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
