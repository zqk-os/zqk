package storage

import (
	"context"
	"maps"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// GetRelated finds objects related to the given object via stored graph edges.
// Walks outbound refs and inbound reverse-index dependents so parent-start and
// child-start traversals are the same graph. relationshipType may be empty (all
// roles), an edge role (membership|composition|associate), or a field name.
func (f *FileObjectStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	// Read the starting object
	startObj, err := f.Read(ctx, secCtx, id)
	if err != nil {
		return nil, errfmt.Errorf(ConstStreamFailedToReadObjectStrErr, id, err)
	}

	// Track visited objects to avoid cycles
	visited := make(map[string]bool)
	visited[id] = true

	// Result set
	related := make([]map[string]any, 0)

	// Queue for BFS traversal: (objectID, currentDepth)
	type queueItem struct {
		id    string
		depth int
	}
	queue := []queueItem{{id: id, depth: 0}}

	// BFS traversal
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Skip if we've exceeded max depth
		if current.depth >= depth {
			continue
		}

		// Read current object (skip if already read start object)
		var currentObj map[string]any
		if current.id == id {
			currentObj = startObj
		} else {
			var readErr error
			currentObj, readErr = f.Read(ctx, secCtx, current.id)
			if readErr != nil {
				// Skip objects we can't read (permissions, not found, etc.)
				continue
			}
		}

		for _, hop := range f.collectGraphHops(ctx, secCtx, current.id, currentObj, relationshipType) {
			if visited[hop.NeighborID] {
				continue
			}
			refObj, readErr := f.Read(ctx, secCtx, hop.NeighborID)
			if readErr != nil {
				continue
			}
			related = append(related, refObj)
			visited[hop.NeighborID] = true
			if current.depth+1 < depth {
				queue = append(queue, queueItem{id: hop.NeighborID, depth: current.depth + 1})
			}
		}
	}

	return related, nil
}

// GetPath finds a path between two objects via reference relationships
func (f *FileObjectStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	// BFS to find shortest path
	type pathNode struct {
		id      string
		path    []string // IDs in path from start to this node
		visited map[string]bool
	}

	// Normalize IDs (handle "kind:id" format)
	normalizeID := func(id string) string {
		if strings.Contains(id, ":") {
			parts := strings.SplitN(id, ":", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
		return id
	}

	normalizedFromID := normalizeID(fromID)
	normalizedToID := normalizeID(toID)

	// If same object, return single-item path
	if normalizedFromID == normalizedToID {
		obj, err := f.Read(ctx, secCtx, normalizedFromID)
		if err != nil {
			return nil, err
		}
		return []map[string]any{obj}, nil
	}

	// BFS queue
	queue := []pathNode{{
		id:      normalizedFromID,
		path:    []string{normalizedFromID},
		visited: map[string]bool{normalizedFromID: true},
	}}

	// Track all visited nodes globally to avoid cycles
	globalVisited := make(map[string]bool)
	globalVisited[normalizedFromID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Read current object
		currentObj, err := f.Read(ctx, secCtx, current.id)
		if err != nil {
			continue // Skip objects we can't read
		}

		for _, hop := range f.collectGraphHops(ctx, secCtx, current.id, currentObj, emptyValue) {
			normalizedRefID := hop.NeighborID

			// Check if we reached the target
			if normalizedRefID == normalizedToID {
				// Build path: current path + target
				path := make([]string, len(current.path))
				copy(path, current.path)
				path = append(path, normalizedToID)

				// Read all objects in path
				result := make([]map[string]any, 0, len(path))
				for _, pathID := range path {
					obj, err := f.Read(ctx, secCtx, pathID)
					if err != nil {
						return nil, errfmt.Errorf(ConstStreamFailedToReadObjectStrInPathErr, pathID, err)
					}
					result = append(result, obj)
				}
				return result, nil
			}

			// Skip if already visited globally
			if globalVisited[normalizedRefID] {
				continue
			}

			// Add to queue
			newPath := make([]string, len(current.path))
			copy(newPath, current.path)
			newPath = append(newPath, normalizedRefID)

			newVisited := make(map[string]bool, len(current.visited)+1)
			maps.Copy(newVisited, current.visited)
			newVisited[normalizedRefID] = true

			queue = append(queue, pathNode{
				id:      normalizedRefID,
				path:    newPath,
				visited: newVisited,
			})

			globalVisited[normalizedRefID] = true
		}
	}

	// No path found
	return []map[string]any{}, nil
}

// GetNeighbors finds immediate neighbors of an object (depth=1)
func (f *FileObjectStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	// Read the object
	obj, err := f.Read(ctx, secCtx, id)
	if err != nil {
		return nil, errfmt.Errorf(ConstStreamFailedToReadObjectStrErr, id, err)
	}

	neighbors := make([]map[string]any, 0)
	visited := make(map[string]bool)

	// Normalize ID for comparison
	normalizeID := func(refID string) string {
		if strings.Contains(refID, ":") {
			parts := strings.SplitN(refID, ":", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
		return refID
	}

	normalizedID := normalizeID(id)
	visited[normalizedID] = true

	// Handle outgoing (objects this references)
	if direction == "outgoing" || direction == "both" {
		yamlParser := parser.NewYAMLParser()
		refFields := yamlParser.ExtractReferenceFields(obj)

		for _, refValue := range refFields {
			var refIDs []string
			switch v := refValue.(type) {
			case string:
				if v != emptyValue {
					refIDs = []string{v}
				}
			case []any:
				for _, item := range v {
					if str, ok := item.(string); ok && str != emptyValue {
						refIDs = append(refIDs, str)
					}
				}
			case []string:
				refIDs = v
			}

			for _, refID := range refIDs {
				normalizedRefID := normalizeID(refID)
				if !visited[normalizedRefID] {
					refObj, readErr := f.Read(ctx, secCtx, normalizedRefID)
					if readErr == nil {
						neighbors = append(neighbors, refObj)
						visited[normalizedRefID] = true
					}
				}
			}
		}
	}

	// Handle incoming (objects that reference this)
	if direction == "incoming" || direction == "both" {
		// Find all objects that reference this object
		// We need to search all object kinds for reference fields containing this ID
		// This is expensive but necessary for file-based backend
		// Get all object kinds by scanning the process directory
		processEntries, err := fileutil.ReadDir(f.processDir)
		if err != nil {
			// If we can't read the directory, skip incoming search
			return neighbors, nil
		}

		for _, entry := range processEntries {
			if !entry.IsDir() {
				continue
			}

			kind := objects.GetKindFromDirectory(entry.Name())
			if kind == emptyValue {
				continue
			}

			kindDir := filepath.Join(f.processDir, entry.Name())
			if _, err := fileutil.Stat(kindDir); fileutil.IsNotExist(err) {
				continue
			}

			// Collect file paths
			filePaths, err := f.collectFilePaths(ctx, kindDir, nil, nil)
			if err != nil {
				continue
			}

			// Check each object
			for _, filePath := range filePaths {
				fileObj, readErr := f.readObjectFile(ctx, filePath)
				if readErr != nil {
					continue
				}

				// Extract reference fields
				yamlParser := parser.NewYAMLParser()
				refFields := yamlParser.ExtractReferenceFields(fileObj)

				// Check if any reference field contains our ID
				found := false
				for _, refValue := range refFields {
					var refIDs []string
					switch v := refValue.(type) {
					case string:
						if v != emptyValue {
							refIDs = []string{v}
						}
					case []any:
						for _, item := range v {
							if str, ok := item.(string); ok && str != emptyValue {
								refIDs = append(refIDs, str)
							}
						}
					case []string:
						refIDs = v
					}

					for _, refID := range refIDs {
						normalizedRefID := normalizeID(refID)
						if normalizedRefID == normalizedID {
							found = true
							break
						}
					}
					if found {
						break
					}
				}

				if found {
					objID, _ := fileObj[objects.FieldKeyID].(string)
					if objID != emptyValue && !visited[objID] {
						// Check permissions
						if err := f.checkPermission(secCtx, "read", kind); err == nil {
							neighbors = append(neighbors, fileObj)
							visited[objID] = true
						}
					}
				}
			}
		}
	}

	return neighbors, nil
}
