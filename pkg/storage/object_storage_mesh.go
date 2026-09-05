package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/federation"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	// MeshObjectStorage aggregates data from the local kernel and all federated peers.
)

type MeshObjectStorage struct {
	local     ObjectStorageProvider
	transport federation.Transport
}

// NewMeshObjectStorage creates a new MeshObjectStorage.
func NewMeshObjectStorage(local ObjectStorageProvider, transport federation.Transport) *MeshObjectStorage {
	if transport == nil {
		transport = federation.GetDefaultTransport()
	}
	return &MeshObjectStorage{
		local:     local,
		transport: transport,
	}
}

// List aggregates objects from local and remote kernels.
func (m *MeshObjectStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	// 1. Get local results
	localResult, err := m.local.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}

	// Optimization & Recursion Guard: If we are listing remote kernels, don't federate the list.
	if filter.Kind == objects.KindRemoteKernel {
		return localResult, nil
	}

	// 2. Discover remote kernels
	peerFilter := ListFilter{Kind: objects.KindRemoteKernel, Limit: 0}
	peers, err := m.local.List(ctx, secCtx, storageCtx, peerFilter)
	if err != nil || len(peers.Objects) == 0 {
		return localResult, nil // No peers, return local only
	}

	// 3. Resolve local identity to skip self
	idManager := federation.GetIdentityManager()
	localID, err := idManager.GetKernelID()
	if err != nil {
		logging.LogSwallowedError(err)
	}

	// 4. Bounded Parallel query to peers using goroutinelabels.Pool (POL-CODE-013)

	var mu sync.Mutex
	allObjects := localResult.Objects
	poolSize := len(peers.Objects)
	if poolSize > 5 {
		poolSize = 5
	} // Tight mesh query concurrency

	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), ConstStreamMeshQueryPool, ConstStreamAggregatingFederatedProjectData, poolSize, poolSize)
	pool.Start(ctx)

	for _, peer := range peers.Objects {
		peer := peer
		id, _ := peer[objects.FieldKeyID].(string)
		endpoint, _ := peer[objects.FieldKeyEndpoint].(string)

		// Skip self and invalid endpoints
		if id == localID || strings.HasSuffix(id, localID) || endpoint == "" {
			continue
		}
		var _err_82963055 = pool.Submit(ctx, func(workerCtx context.Context) error {

			peerCtx, cancel := context.WithTimeout(workerCtx, 30*time.Second)
			defer cancel()

			args := map[string]any{
				objects.FieldKeyKind: filter.Kind,
				"filters":            filter.Filters,
				"sort_by":            filter.SortBy,
				"sort_asc":           filter.SortAsc,
				"offset":             filter.Offset,
				"limit":              filter.Limit,
				"group_by":           filter.GroupBy,
			}

			raw, err := m.transport.ExecuteTool(peerCtx, endpoint, "object_list", args)
			if err != nil {
				return nil
			}

			var remoteResult QueryResult
			if err := json.Unmarshal(raw, &remoteResult); err != nil {
				return nil
			}

			mu.Lock()
			allObjects = append(allObjects, remoteResult.Objects...)
			mu.Unlock()
			return nil
		})
		if _err_82963055 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82963055).Log()
		}
	}

	pool.Stop()

	// Re-apply sorting if needed
	if filter.SortBy != "" {
		sortObjects(allObjects, filter.SortBy, filter.SortAsc)
	}

	// Re-apply pagination to the merged result
	totalCount := len(allObjects)
	effectiveLimit := filter.Limit
	if effectiveLimit > 0 || filter.Offset > 0 {
		start := filter.Offset
		if start > len(allObjects) {
			start = len(allObjects)
		}
		end := start + effectiveLimit
		if end > len(allObjects) {
			end = len(allObjects)
		}
		if effectiveLimit == 0 {
			end = len(allObjects)
		}
		allObjects = allObjects[start:end]
	}

	var grouped map[string][]map[string]any
	if filter.GroupBy != "" && storageCtx != nil && storageCtx.EnableGrouping {
		grouped = m.groupObjects(allObjects, filter.GroupBy, storageCtx.MaxGroupSize)
	}

	meta := map[string]any{
		"total_count": totalCount,
	}
	if grouped != nil {
		meta["total_groups"] = len(grouped)
	}

	return &QueryResult{
		Objects: allObjects,
		Groups:  grouped,
		Meta:    meta,
	}, nil
}

// Read tries local first, then searches the mesh.
func (m *MeshObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	obj, err := m.local.Read(ctx, secCtx, id)
	if err == nil {
		return obj, nil
	}
	return nil, err
}

// Create only creates locally. Federation is read-only for now.
func (m *MeshObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return m.local.Create(ctx, secCtx, obj)
}

func (m *MeshObjectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return m.local.Update(ctx, secCtx, id, updates)
}

func (m *MeshObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	return m.local.Delete(ctx, secCtx, id, cascade)
}

func (m *MeshObjectStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	return m.local.Exists(ctx, secCtx, id)
}

func (m *MeshObjectStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	return m.local.Query(ctx, secCtx, storageCtx, query)
}

func (m *MeshObjectStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	return m.local.Search(ctx, secCtx, storageCtx, query)
}

func (m *MeshObjectStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return m.local.BeginTransaction(ctx)
}

func (m *MeshObjectStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	return m.local.BulkCreate(ctx, secCtx, objects)
}

func (m *MeshObjectStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	return m.local.BulkUpdate(ctx, secCtx, updates)
}

func (m *MeshObjectStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	return m.local.BulkGet(ctx, secCtx, ids)
}

func (m *MeshObjectStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	return m.local.BulkDelete(ctx, secCtx, ids, cascade)
}

func (m *MeshObjectStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	res, err := m.List(ctx, secCtx, nil, filter)
	if err != nil {
		return 0, err
	}
	if val, ok := res.Meta["total_count"].(int); ok {
		return val, nil
	}
	return len(res.Objects), nil
}

func (m *MeshObjectStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return m.local.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
}

func (m *MeshObjectStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	return m.local.GetRelated(ctx, secCtx, id, relationshipType, depth)
}

func (m *MeshObjectStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return m.local.GetPath(ctx, secCtx, fromID, toID)
}

func (m *MeshObjectStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	return m.local.GetNeighbors(ctx, secCtx, id, direction)
}

func (m *MeshObjectStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	return m.local.Move(ctx, secCtx, id, newKind, updateReferences)
}

func (m *MeshObjectStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	return m.local.Rename(ctx, secCtx, oldID, newID, updateReferences)
}

// Ensure MeshObjectStorage implements ObjectStorageProvider
var _ ObjectStorageProvider = (*MeshObjectStorage)(nil)

// GetLocal returns the local storage provider.
func (m *MeshObjectStorage) GetLocal() ObjectStorageProvider {
	return m.local
}

func (m *MeshObjectStorage) Shutdown(ctx context.Context) error {
	if m.local != nil {
		return m.local.Shutdown(ctx)
	}
	return nil
}

// groupObjects groups objects by a field or comma-separated fields
func (m *MeshObjectStorage) groupObjects(objList []map[string]any, groupBy string, maxGroups int) map[string][]map[string]any {
	groups := make(map[string][]map[string]any)

	groupFields := strings.Split(groupBy, ",")
	for i := range groupFields {
		groupFields[i] = strings.TrimSpace(groupFields[i])
	}

	for _, obj := range objList {
		var groupValueParts []string
		for _, field := range groupFields {
			if val, ok := obj[field]; ok && val != nil && fmt.Sprintf("%v", val) != "" {
				groupValueParts = append(groupValueParts, fmt.Sprintf("%v", val))
			} else {
				groupValueParts = append(groupValueParts, "") // Empty/null values grouped together
			}
		}
		groupValue := strings.Join(groupValueParts, ", ")

		groups[groupValue] = append(groups[groupValue], obj)
	}

	// Limit number of groups if specified
	if maxGroups > 0 && len(groups) > maxGroups {
		// Keep only the first maxGroups groups (sorted by key)
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		limited := make(map[string][]map[string]any)
		for i := 0; i < maxGroups && i < len(keys); i++ {
			limited[keys[i]] = groups[keys[i]]
		}
		return limited
	}

	return groups
}
