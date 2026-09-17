package observer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// Label for code entity nodes in the knowledge kernel (BLI-OBS-002).
const (
	LabelCodeEntity = "CodeEntity"
	LabelSourceFile = "SourceFile"
	LabelPackage    = "Package"
	EdgeContains    = "CONTAINS"   // SourceFile -> CodeEntity
	EdgeMethodOf    = "METHOD_OF"  // Method CodeEntity -> Type CodeEntity
	EdgeCalls       = "CALLS"      // CodeEntity -> CodeEntity
	EdgeDependsOn   = "DEPENDS_ON" // CodeEntity -> CodeEntity
	EdgeImports     = "IMPORTS"    // SourceFile -> Package
	EdgeImplements  = "IMPLEMENTS" // CodeEntity -> CodeEntity
)

// PopulateResult summarizes graph population (nodes/edges created, errors).
type PopulateResult struct {
	NodesCreated int
	EdgesCreated int
	Errors       []string
}

// EntityID returns a stable, unique id for an entity (file:line:kind:name).
func EntityID(e *Entity) string {
	name := e.Name
	if e.Receiver != emptyValue {
		name = e.Receiver + "." + name
	}
	return fmt.Sprintf("code_entity:%s:%d:%s:%s", e.File, e.Line, e.Kind, name)
}

// FileID returns a stable id for a source file node.
func FileID(filePath string) string {
	return "source_file:" + filePath
}

// PackageID returns a stable id for a package node.
func PackageID(pkgPath string) string {
	return "package:" + pkgPath
}

// EntityToNode converts an observer Entity to a graph Node for the knowledge kernel.
func EntityToNode(e *Entity) provider.Node {
	props := map[string]any{
		objects.FieldKeyName:      e.Name,
		"file":                    e.File,
		"line":                    e.Line,
		objects.FieldKeySignature: e.Signature,
		"language":                e.Language,
		objects.FieldKeyKind:      e.Kind,
	}
	if e.Receiver != emptyValue {
		props["receiver"] = e.Receiver
	}
	if e.Intent != emptyValue {
		props[objects.FieldKeyIntent] = e.Intent
	}
	if len(e.Embedding) > 0 {
		props["embedding"] = e.Embedding
	}
	if len(e.Metadata) > 0 {
		meta := make(map[string]any, len(e.Metadata))
		for k, v := range e.Metadata {
			meta[k] = v
		}
		props[objects.FieldKeyMetadata] = meta
	}
	labels := []string{LabelCodeEntity, e.Kind}
	return provider.Node{
		ID:         EntityID(e),
		Labels:     labels,
		Properties: props,
	}
}

// SourceFileNode creates a graph node for a source file (document layer).
func SourceFileNode(filePath string) provider.Node {
	return provider.Node{
		ID:     FileID(filePath),
		Labels: []string{LabelSourceFile, "Document"},
		Properties: map[string]any{
			objects.FieldKeyPath: filePath,
		},
	}
}

// PackageNode creates a graph node for a package.
func PackageNode(pkgPath string) provider.Node {
	return provider.Node{
		ID:     PackageID(pkgPath),
		Labels: []string{LabelPackage},
		Properties: map[string]any{
			objects.FieldKeyName: pkgPath,
		},
	}
}

// GraphWriter is the minimal interface needed to populate the knowledge kernel.
// *provider.GraphConnection and provider.GraphTransaction implement it.
type GraphWriter interface {
	CreateNode(ctx context.Context, node provider.Node) error
	CreateEdge(ctx context.Context, edge provider.Edge) error
}

// Populate writes extracted entities and derived relationships to the graph (BLI-OBS-002).
// It creates CodeEntity and SourceFile nodes and CONTAINS / METHOD_OF edges.
// Versioning: pass extractID (e.g. run id or timestamp) to tag this batch; stored in node properties.
func Populate(ctx context.Context, conn GraphWriter, result *ExtractResult, extractID string) (*PopulateResult, error) {
	out := &PopulateResult{}
	if conn == nil {
		return out, errfmt.Errorf("graph connection is nil")
	}
	if result == nil {
		return out, nil
	}

	// Index type-like entities by (file, name) for METHOD_OF edges
	typeKey := func(file, name string) string { return file + ":" + name }
	typesByKey := make(map[string]*Entity)

	// Global name index for cross-file resolution (basic)
	entitiesByName := make(map[string][]*Entity)

	for i := range result.Entities {
		e := &result.Entities[i]
		switch e.Kind {
		case "type", "struct", "interface":
			typesByKey[typeKey(e.File, e.Name)] = e
		}

		entitiesByName[e.Name] = append(entitiesByName[e.Name], e)
		if e.Receiver != emptyValue {
			fullName := e.Receiver + "." + e.Name
			entitiesByName[fullName] = append(entitiesByName[fullName], e)
		}
	}

	// Ensure we have one file node per file and collect entity nodes + CONTAINS edges
	fileNodes := make(map[string]provider.Node)
	packageNodes := make(map[string]provider.Node)
	nodes := make([]provider.Node, 0, len(result.Entities))

	var edges []provider.Edge

	for i := range result.Entities {
		e := &result.Entities[i]
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		node := EntityToNode(e)
		if extractID != emptyValue {
			if node.Properties == nil {
				node.Properties = make(map[string]any)
			}
			node.Properties["extract_id"] = extractID
		}
		nodes = append(nodes, node)

		fid := FileID(e.File)
		if _, ok := fileNodes[e.File]; !ok {
			fileNodes[e.File] = SourceFileNode(e.File)
		}
		for _, imp := range e.Imports {
			if _, ok := packageNodes[imp]; !ok {
				packageNodes[imp] = PackageNode(imp)
			}
		}

		edges = append(edges, provider.Edge{
			FromID:     fid,
			ToID:       node.ID,
			Type:       EdgeContains,
			Properties: map[string]any{},
		})

		for _, imp := range e.Imports {
			edges = append(edges, provider.Edge{
				FromID:     fid,
				ToID:       PackageID(imp),
				Type:       EdgeImports,
				Properties: map[string]any{},
			})
		}

		if e.Kind == "method" && e.Receiver != emptyValue {
			if typ, ok := typesByKey[typeKey(e.File, e.Receiver)]; ok {
				edges = append(edges, provider.Edge{
					FromID:     node.ID,
					ToID:       EntityID(typ),
					Type:       EdgeMethodOf,
					Properties: map[string]any{},
				})
			}
		}

		for _, call := range e.Calls {
			if targets, ok := entitiesByName[call]; ok && len(targets) > 0 {
				edges = append(edges, provider.Edge{
					FromID:     node.ID,
					ToID:       EntityID(targets[0]),
					Type:       EdgeCalls,
					Properties: map[string]any{},
				})
			}
		}

		for _, dep := range e.DependsOn {
			if targets, ok := entitiesByName[dep]; ok && len(targets) > 0 {
				edges = append(edges, provider.Edge{
					FromID:     node.ID,
					ToID:       EntityID(targets[0]),
					Type:       EdgeDependsOn,
					Properties: map[string]any{},
				})
			}
		}
	}

	// Create package nodes
	for _, n := range packageNodes {
		if err := conn.CreateNode(ctx, n); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("create package node %s: %v", n.ID, err))
			continue
		}
		out.NodesCreated++
	}
	// Create file nodes
	for _, n := range fileNodes {
		if err := conn.CreateNode(ctx, n); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("create file node %s: %v", n.ID, err))
			continue
		}
		out.NodesCreated++
	}
	// Create entity nodes
	for _, n := range nodes {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if err := conn.CreateNode(ctx, n); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("create entity node %s: %v", n.ID, err))
			continue
		}
		out.NodesCreated++
	}
	// Create edges
	for _, ed := range edges {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if err := conn.CreateEdge(ctx, ed); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("create edge %s %s->%s: %v", ed.Type, ed.FromID, ed.ToID, err))
			continue
		}
		out.EdgesCreated++
	}

	return out, nil
}

// PopulateBatch uses ExecuteBatch when the connection supports it, for better performance.
// Falls back to Populate when batch is not supported or fails. Requires a full GraphConnection.
func PopulateBatch(ctx context.Context, conn interface {
	GraphWriter
	ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error)
}, result *ExtractResult, extractID string) (*PopulateResult, error) {
	if result == nil || conn == nil {
		return &PopulateResult{}, nil
	}
	typeKey := func(file, name string) string { return file + ":" + name }
	typesByKey := make(map[string]*Entity)

	// Global name index for cross-file resolution (basic)
	entitiesByName := make(map[string][]*Entity)

	for i := range result.Entities {
		e := &result.Entities[i]
		switch e.Kind {
		case "type", "struct", "interface":
			typesByKey[typeKey(e.File, e.Name)] = e
		}

		entitiesByName[e.Name] = append(entitiesByName[e.Name], e)
		if e.Receiver != emptyValue {
			fullName := e.Receiver + "." + e.Name
			entitiesByName[fullName] = append(entitiesByName[fullName], e)
		}
	}

	fileNodes := make(map[string]provider.Node)
	packageNodes := make(map[string]provider.Node)
	for _, e := range result.Entities {
		if _, ok := fileNodes[e.File]; !ok {
			fileNodes[e.File] = SourceFileNode(e.File)
		}
		for _, imp := range e.Imports {
			if _, ok := packageNodes[imp]; !ok {
				packageNodes[imp] = PackageNode(imp)
			}
		}
	}
	var ops []provider.Operation

	// 1) Create package nodes
	for _, n := range packageNodes {
		ops = append(ops, provider.Operation{Type: "create_node", Data: n})
	}

	// 2) Create file nodes
	for _, n := range fileNodes {
		ops = append(ops, provider.Operation{Type: "create_node", Data: n})
	}

	// 3) Create entity nodes
	for i := range result.Entities {
		e := &result.Entities[i]
		node := EntityToNode(e)
		if extractID != emptyValue {
			if node.Properties == nil {
				node.Properties = make(map[string]any)
			}
			node.Properties["extract_id"] = extractID
		}
		ops = append(ops, provider.Operation{Type: "create_node", Data: node})
	}

	// 4) Create edges
	edgesCreated := 0
	for i := range result.Entities {
		e := &result.Entities[i]

		// CONTAINS (File -> Entity)
		ops = append(ops, provider.Operation{
			Type: "create_edge",
			Data: provider.Edge{
				FromID: FileID(e.File), ToID: EntityID(e), Type: EdgeContains, Properties: map[string]any{},
			},
		})
		edgesCreated++

		// IMPORTS (File -> Package)
		for _, imp := range e.Imports {
			ops = append(ops, provider.Operation{
				Type: "create_edge",
				Data: provider.Edge{
					FromID: FileID(e.File), ToID: PackageID(imp), Type: EdgeImports, Properties: map[string]any{},
				},
			})
			edgesCreated++
		}

		// METHOD_OF
		if e.Kind == "method" && e.Receiver != emptyValue {
			if typ, ok := typesByKey[typeKey(e.File, e.Receiver)]; ok {
				ops = append(ops, provider.Operation{
					Type: "create_edge",
					Data: provider.Edge{
						FromID: EntityID(e), ToID: EntityID(typ), Type: EdgeMethodOf, Properties: map[string]any{},
					},
				})
				edgesCreated++
			}
		}

		// CALLS
		for _, call := range e.Calls {
			if targets, ok := entitiesByName[call]; ok && len(targets) > 0 {
				ops = append(ops, provider.Operation{
					Type: "create_edge",
					Data: provider.Edge{
						FromID: EntityID(e), ToID: EntityID(targets[0]), Type: EdgeCalls, Properties: map[string]any{},
					},
				})
				edgesCreated++
			}
		}

		// DEPENDS_ON
		for _, dep := range e.DependsOn {
			if targets, ok := entitiesByName[dep]; ok && len(targets) > 0 {
				ops = append(ops, provider.Operation{
					Type: "create_edge",
					Data: provider.Edge{
						FromID: EntityID(e), ToID: EntityID(targets[0]), Type: EdgeDependsOn, Properties: map[string]any{},
					},
				})
				edgesCreated++
			}
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // Limit concurrent MemGraph batch executions
	batchSize := 2500
	numChunks := (len(ops) + batchSize - 1) / batchSize
	totalOps := int32(len(ops)) //nolint:gosec

	var processed int32
	var fallbackNeeded int32

	// Lock-free slice for parallel batch error collection
	chunkErrors := make([][]string, numChunks)

	for chunkIdx := 0; chunkIdx < numChunks; chunkIdx++ {
		i := chunkIdx * batchSize
		end := i + batchSize
		if end > len(ops) {
			end = len(ops)
		}
		chunk := ops[i:end]
		chunkIdx := chunkIdx

		sem <- struct{}{}

		goroutinelabels.NewGoroutine("graph_populator", "batch insert").
			WithContext(ctx).
			WithWaitGroup(&wg).
			WithPreCleanup(func() {
				<-sem
			}).
			WithErrorHandler(func(err error) {
				if err != nil {
					atomic.StoreInt32(&fallbackNeeded, 1)
				}
			}).
			WithPanicHandler(func(r interface{}) {
				atomic.StoreInt32(&fallbackNeeded, 1)
			}).
			StartWithContext(ctx, func(ctx context.Context) error {
				batchRes, batchErr := conn.ExecuteBatch(ctx, chunk)
				if batchErr != nil {
					return batchErr
				}

				var errs []string
				for _, e := range batchRes.Errors {
					errs = append(errs, e.Error())
				}
				chunkErrors[chunkIdx] = errs

				p := atomic.AddInt32(&processed, int32(len(chunk))) //nolint:gosec
				logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("Populating Graph: %d/%d operations", p, totalOps)).Log()
				return nil
			})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("wait_group", "waiting for graph batch insert").
		WithCleanup(func() { close(waitDone) }).
		WithPanicHandler(func(r interface{}) {}).
		StartSimple(func() { wg.Wait() })

	select {
	case <-waitDone:
	case <-time.After(15 * time.Minute):
		atomic.StoreInt32(&fallbackNeeded, 1)
		logging.FluentEvent(logging.GetLogger()).Warn("Graph batch population timed out").Log()
	case <-ctx.Done():
		atomic.StoreInt32(&fallbackNeeded, 1)
	}

	if atomic.LoadInt32(&fallbackNeeded) == 1 {
		logging.FluentEvent(logging.GetLogger()).Warn("Populate batch failed, falling back...").Log()
		return Populate(ctx, conn, result, extractID)
	}

	out := &PopulateResult{}
	for _, errs := range chunkErrors {
		out.Errors = append(out.Errors, errs...)
	}

	logging.FluentEvent(logging.GetLogger()).Info("Graph population complete.").Log()
	out.NodesCreated = len(packageNodes) + len(fileNodes) + len(result.Entities)
	out.EdgesCreated = edgesCreated
	return out, nil
}
