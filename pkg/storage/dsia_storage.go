package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"gopkg.in/yaml.v3"
)

type DSIAStorageProvider struct {
	baseDir string
	mu      sync.RWMutex
}

// NewDSIAStorageProvider creates a new Deterministic Stable-Identity Storage provider
func NewDSIAStorageProvider(baseDir ...string) *DSIAStorageProvider {
	dir := "docs/process"
	if len(baseDir) > 0 && baseDir[0] != "" {
		dir = baseDir[0]
	}
	return &DSIAStorageProvider{baseDir: dir}
}

// AtomicWriteFile writes data to a temporary file in the same directory as the target file
// and then renames it atomically to the target file path.
func (p *DSIAStorageProvider) AtomicWriteFile(filePath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errfmt.Newf("failed to create directory %s", dir).Wrap(err)
	}
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return errfmt.Newf("failed to generate random suffix").Wrap(err)
	}
	tempFilePath := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%s", filepath.Base(filePath), hex.EncodeToString(randBytes)))
	if err := os.WriteFile(tempFilePath, data, perm); err != nil {
		return errfmt.Newf("failed to write temp file %s", tempFilePath).Wrap(err)
	}
	if err := os.Rename(tempFilePath, filePath); err != nil {
		_ = os.Remove(tempFilePath)
		return errfmt.Newf("failed to rename temp file to target %s", filePath).Wrap(err)
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := targetPath + fmt.Sprintf(".%d.tmp", time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, finalData, 0644); err != nil {
		return err
	}

	// Atomic rename
	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

func (p *DSIAStorageProvider) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	kind, ok := obj["kind"].(string)
	if !ok {
		return fmt.Errorf("kind is required")
	}
	objectID, ok := obj["id"].(string)
	if !ok {
		return fmt.Errorf("id is required")
	}

	targetPath := p.getPath(kind, objectID)

	// Check if already exists
	if _, err := os.Stat(targetPath); err == nil {
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
	entries, err := os.ReadDir(p.baseDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				potentialPath := filepath.Join(p.baseDir, entry.Name(), id+".yaml")
				if _, err := os.Stat(potentialPath); err == nil {
					foundPath = potentialPath
					break
				}
			}
		}
	}

	if foundPath == "" {
		return nil, ErrObjectNotFound
	}

	data, err := os.ReadFile(foundPath)
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

	kind, ok := updates["kind"].(string)
	if !ok {
		return fmt.Errorf("kind is required")
	}

	targetPath := p.getPath(kind, id)

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return fmt.Errorf("object not found")
	}

	return p.writeWithChecksumAndRename(targetPath, updates)
}

func (p *DSIAStorageProvider) Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var foundPath string
	entries, err := os.ReadDir(p.baseDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				potentialPath := filepath.Join(p.baseDir, entry.Name(), id+".yaml")
				if _, err := os.Stat(potentialPath); err == nil {
					foundPath = potentialPath
					break
				}
			}
		}
	}

	if foundPath == "" {
		return ErrObjectNotFound
	}

	return os.Remove(foundPath)
}

// Unimplemented methods to satisfy ObjectStorageProvider

func (p *DSIAStorageProvider) List(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var objects []map[string]any

	var targetDirs []string
	if filter.Kind != "" {
		targetDirs = append(targetDirs, filepath.Join(p.baseDir, filter.Kind+"s"))
	} else {
		entries, err := os.ReadDir(p.baseDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					targetDirs = append(targetDirs, filepath.Join(p.baseDir, entry.Name()))
				}
			}
		}
	}

	for _, dir := range targetDirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".yaml" {
				continue
			}
			filePath := filepath.Join(dir, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				continue
			}
			objects = append(objects, obj)
		}
	}

	return &QueryResult{
		Objects: objects,
		Meta: map[string]any{
			"total": len(objects),
		},
	}, nil
}

func (p *DSIAStorageProvider) Query(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query Query) (*QueryResult, error) {
	filter := ListFilter{
		Kind: query.Kind,
	}
	return p.List(ctx, secCtx, storageCtx, filter)
}

func (p *DSIAStorageProvider) Search(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query SearchQuery) (*SearchResult, error) {
	filter := ListFilter{}
	if len(query.Kinds) == 1 {
		filter.Kind = query.Kinds[0]
	}
	res, err := p.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}

	searchQueryLower := strings.ToLower(query.Query)
	var matches []SearchMatch

	for _, obj := range res.Objects {
		var matchedFields []string
		var score float64

		for k, v := range obj {
			strVal, ok := v.(string)
			if !ok {
				continue
			}
			strValLower := strings.ToLower(strVal)
			if strings.Contains(strValLower, searchQueryLower) {
				matchedFields = append(matchedFields, k)
				if strValLower == searchQueryLower {
					score += 1.0
				} else {
					score += 0.5
				}
			}
		}

		if len(matchedFields) > 0 {
			matches = append(matches, SearchMatch{
				Object:        obj,
				Score:         score,
				MatchedFields: matchedFields,
			})
		}
	}

	return &SearchResult{
		Objects:    matches,
		TotalCount: len(matches),
		Meta: map[string]any{
			"total": len(matches),
		},
	}, nil
}

type DSIATransaction struct {
	provider *DSIAStorageProvider
	staged   map[string]map[string]any
	deleted  map[string]bool
	active   bool
	mu       sync.Mutex
}

func (tx *DSIATransaction) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.active {
		return fmt.Errorf("transaction inactive")
	}
	id, ok := obj["id"].(string)
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
		id, _ := obj["id"].(string)
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

func (p *DSIAStorageProvider) BulkCreate(ctx context.Context, secCtx *SecurityContext, objects []map[string]any) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(objects),
		Results:    make([]map[string]any, 0, len(objects)),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, obj := range objects {
		id, _ := obj["id"].(string)
		err := p.Create(ctx, secCtx, obj)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, obj)
		}
	}

	return result, nil
}

func (p *DSIAStorageProvider) BulkUpdate(ctx context.Context, secCtx *SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(updates),
		Results:    make([]map[string]any, 0, len(updates)),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, updateItem := range updates {
		err := p.Update(ctx, secCtx, updateItem.ID, updateItem.Updates)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      updateItem.ID,
				Index:   i,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, updateItem.Updates)
		}
	}

	return result, nil
}

func (p *DSIAStorageProvider) BulkGet(ctx context.Context, secCtx *SecurityContext, ids []string) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0, len(ids)),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, id := range ids {
		obj, err := p.Read(ctx, secCtx, id)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, obj)
		}
	}

	return result, nil
}

func (p *DSIAStorageProvider) BulkDelete(ctx context.Context, secCtx *SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	for i, id := range ids {
		err := p.Delete(ctx, secCtx, id, cascade)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			result.SuccessCount++
			result.Results = append(result.Results, map[string]any{"id": id, "deleted": true})
		}
	}

	return result, nil
}

func (p *DSIAStorageProvider) Exists(ctx context.Context, secCtx *SecurityContext, id string) (bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var foundPath string
	entries, err := os.ReadDir(p.baseDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				potentialPath := filepath.Join(p.baseDir, entry.Name(), id+".yaml")
				if _, err := os.Stat(potentialPath); err == nil {
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

func (p *DSIAStorageProvider) Aggregate(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	res, err := p.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}

	resultMap := make(map[string]any)

	for _, agg := range aggregations {
		alias := agg.Alias
		if alias == "" {
			alias = string(agg.Function)
			if agg.Field != "" {
				alias += "_" + agg.Field
			}
		}

		switch agg.Function {
		case AggregationCount:
			resultMap[alias] = len(res.Objects)
		default:
			resultMap[alias] = len(res.Objects)
		}
	}

	return &AggregateResult{
		Aggregations: resultMap,
		Meta: map[string]any{
			"total": len(res.Objects),
		},
	}, nil
}

func (p *DSIAStorageProvider) GetRelated(ctx context.Context, secCtx *SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	obj, err := p.Read(ctx, secCtx, id)
	if err != nil {
		return nil, err
	}

	var related []map[string]any
	for k, v := range obj {
		if relationshipType != "" && k != relationshipType && !strings.HasSuffix(k, "_ref") && !strings.HasSuffix(k, "_refs") {
			continue
		}
		if refID, ok := v.(string); ok {
			if target, err := p.Read(ctx, secCtx, refID); err == nil {
				related = append(related, target)
			}
		} else if refIDs, ok := v.([]any); ok {
			for _, item := range refIDs {
				if rID, ok := item.(string); ok {
					if target, err := p.Read(ctx, secCtx, rID); err == nil {
						related = append(related, target)
					}
				}
			}
		}
	}
	return related, nil
}

func (p *DSIAStorageProvider) GetPath(ctx context.Context, secCtx *SecurityContext, fromID, toID string) ([]map[string]any, error) {
	fromObj, err := p.Read(ctx, secCtx, fromID)
	if err != nil {
		return nil, err
	}
	toObj, err := p.Read(ctx, secCtx, toID)
	if err != nil {
		return nil, err
	}
	return []map[string]any{fromObj, toObj}, nil
}

func (p *DSIAStorageProvider) GetNeighbors(ctx context.Context, secCtx *SecurityContext, id string, direction string) ([]map[string]any, error) {
	return p.GetRelated(ctx, secCtx, id, "", 1)
}

func (p *DSIAStorageProvider) Move(ctx context.Context, secCtx *SecurityContext, id string, newKind string, updateReferences bool) error {
	obj, err := p.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}
	if err := p.Delete(ctx, secCtx, id, false); err != nil {
		return err
	}
	obj["kind"] = newKind
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
	obj["id"] = newID
	return p.Create(ctx, secCtx, obj)
}

func (p *DSIAStorageProvider) Shutdown(ctx context.Context) error {
	return nil
}
