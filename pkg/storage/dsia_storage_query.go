// Extracted from dsia_storage.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func (p *DSIAStorageProvider) List(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var objects []map[string]any

	var targetDirs []string
	if filter.Kind != "" {
		targetDirs = append(targetDirs, filepath.Join(p.baseDir, filter.Kind+"s"))
	} else {
		entries, err := fileutil.ReadDir(p.baseDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					targetDirs = append(targetDirs, filepath.Join(p.baseDir, entry.Name()))
				}
			}
		}
	}

	for _, dir := range targetDirs {
		files, err := fileutil.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".yaml" {
				continue
			}
			filePath := filepath.Join(dir, file.Name())
			data, err := fileutil.ReadFile(filePath)
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
