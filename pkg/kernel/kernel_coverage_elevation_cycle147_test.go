package kernel_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/dna"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/kernel"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

type cycle147ClosableStorage struct {
	storage.ObjectStorageProvider
	shutdownErr error
	shutdownRan bool
}

func (s *cycle147ClosableStorage) Shutdown(ctx context.Context) error {
	s.shutdownRan = true
	return s.shutdownErr
}

type cycle147MockExtension struct {
	name        string
	initErr     error
	shutdownErr error
	initCalled  bool
	shutCalled  bool
}

func (e *cycle147MockExtension) Name() string {
	return e.name
}

func (e *cycle147MockExtension) Init(ctx context.Context, k kernel.KnowledgeKernel) error {
	e.initCalled = true
	return e.initErr
}

func (e *cycle147MockExtension) Shutdown(ctx context.Context) error {
	e.shutCalled = true
	return e.shutdownErr
}

type cycle147MockMCP struct {
	mcp.MCPServer
	callToolErr    error
	callToolRes    *mcp.ToolCallResult
	listToolsErr   error
	listToolsRes   *mcp.ToolsListResult
	receivedParams *mcp.ToolCallParams
}

func (m *cycle147MockMCP) CallTool(ctx context.Context, params *mcp.ToolCallParams) (*mcp.ToolCallResult, error) {
	m.receivedParams = params
	if m.callToolErr != nil {
		return nil, m.callToolErr
	}
	return m.callToolRes, nil
}

func (m *cycle147MockMCP) ListTools(ctx context.Context) (*mcp.ToolsListResult, error) {
	if m.listToolsErr != nil {
		return nil, m.listToolsErr
	}
	return m.listToolsRes, nil
}

type cycle147Envelope struct {
	baseObj dna.BaseObject
}

func (e cycle147Envelope) GetBaseObject() dna.BaseObject {
	return e.baseObj
}

func makeCycle147URN(t *testing.T, prefix, entity, id string) dna.URN {
	t.Helper()
	u, err := dna.NewURN(prefix, entity, id)
	require.NoError(t, err)
	return u
}

func TestCycle147_KnowledgeKernel_LifecycleAndProjectRoot(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	mockServer := &cycle147MockMCP{}
	storageProvider := &cycle147ClosableStorage{}

	// Test default constructor with empty project root
	defaultKernel := kernel.NewKernel(storageProvider, mockServer)
	require.NotNil(t, defaultKernel)
	assert.Empty(t, defaultKernel.ProjectRoot())

	// Test constructor with explicit project root
	customKernel := kernel.NewKernelWithOptions(storageProvider, mockServer, proj.Root)
	require.NotNil(t, customKernel)
	assert.Equal(t, proj.Root, customKernel.ProjectRoot())
}

func TestCycle147_Kernel_Extensions_RegistrationAndValidation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	k := kernel.NewKernelWithOptions(&cycle147ClosableStorage{}, &cycle147MockMCP{}, proj.Root)

	// 1. Nil extension registration
	err := k.RegisterExtension(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot register nil extension")

	// 2. Empty extension name registration
	emptyExt := &cycle147MockExtension{name: ""}
	err = k.RegisterExtension(emptyExt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extension name cannot be empty")

	// 3. Extension initialization failure
	failExt := &cycle147MockExtension{name: "failing-plugin", initErr: errors.New("init boom")}
	err = k.RegisterExtension(failExt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize extension")
	assert.True(t, failExt.initCalled)

	// 4. Valid registration & duplicate check
	validExt := &cycle147MockExtension{name: "analytics-plugin"}
	err = k.RegisterExtension(validExt)
	require.NoError(t, err)
	assert.True(t, validExt.initCalled)

	dupErr := k.RegisterExtension(validExt)
	require.Error(t, dupErr)
	assert.Contains(t, dupErr.Error(), "already registered")

	// 5. Lookup and listing
	foundExt, ok := k.GetExtension("analytics-plugin")
	require.True(t, ok)
	assert.Equal(t, validExt, foundExt)

	_, ok = k.GetExtension("non-existent")
	assert.False(t, ok)

	secondExt := &cycle147MockExtension{name: "auth-plugin"}
	err = k.RegisterExtension(secondExt)
	require.NoError(t, err)

	names := k.ListExtensions()
	assert.Equal(t, []string{"analytics-plugin", "auth-plugin"}, names)
}

func TestCycle147_Kernel_Shutdown_ErrorScenarios(t *testing.T) {
	ctx := context.Background()

	// 1. Clean shutdown without closer or extensions
	kClean := kernel.NewKernel(nil, &cycle147MockMCP{})
	err := kClean.Shutdown(ctx)
	require.NoError(t, err)

	// 2. Shutdown with extension error
	kExtErr := kernel.NewKernel(nil, &cycle147MockMCP{})
	errExt := &cycle147MockExtension{name: "failing-ext", shutdownErr: errors.New("ext teardown fail")}
	require.NoError(t, kExtErr.RegisterExtension(errExt))

	err = kExtErr.Shutdown(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error shutting down extension failing-ext")
	assert.True(t, errExt.shutCalled)

	// 3. Shutdown with storage provider closer error
	storeErr := errors.New("storage sync error")
	storageWithErr := &cycle147ClosableStorage{shutdownErr: storeErr}
	kStoreErr := kernel.NewKernel(storageWithErr, &cycle147MockMCP{})

	err = kStoreErr.Shutdown(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, storeErr))
	assert.True(t, storageWithErr.shutdownRan)

	// 4. Combined shutdown errors joined together
	storeErrBoth := errors.New("storage combined fail")
	storageBoth := &cycle147ClosableStorage{shutdownErr: storeErrBoth}
	kBoth := kernel.NewKernel(storageBoth, &cycle147MockMCP{})

	extBoth := &cycle147MockExtension{name: "both-fail-ext", shutdownErr: errors.New("ext combined fail")}
	require.NoError(t, kBoth.RegisterExtension(extBoth))

	err = kBoth.Shutdown(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error shutting down extension both-fail-ext")
	assert.Contains(t, err.Error(), "storage combined fail")
}

func TestCycle147_Kernel_MCP_ExecutionAndListing(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Success tool execution
	expectedToolRes := &mcp.ToolCallResult{
		Content: []mcp.Content{{Type: "text", Text: "tool execution successful"}},
	}
	mockServer := &cycle147MockMCP{callToolRes: expectedToolRes}
	k := kernel.NewKernel(nil, mockServer)

	res, err := k.ExecuteMCPTool(ctx, secCtx, "custom_run", map[string]any{"key": "value"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, expectedToolRes, res)
	assert.Equal(t, "custom_run", mockServer.receivedParams.Name)
	assert.Equal(t, "value", mockServer.receivedParams.Arguments["key"])

	// 2. Failed tool execution
	mockServer.callToolErr = errors.New("mcp protocol failure")
	_, err = k.ExecuteMCPTool(ctx, secCtx, "custom_run", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp protocol failure")

	// 3. Tool listing success
	expectedListRes := &mcp.ToolsListResult{
		Tools: []mcp.Tool{{Name: "custom_run", Description: "runs custom task"}},
	}
	mockServer.listToolsRes = expectedListRes
	mockServer.listToolsErr = nil

	listRes, err := k.ListMCPTools(ctx, secCtx)
	require.NoError(t, err)
	require.NotNil(t, listRes)
	assert.Equal(t, expectedListRes, listRes)

	// 4. Tool listing failure
	mockServer.listToolsErr = errors.New("cannot enumerate tools")
	_, err = k.ListMCPTools(ctx, secCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot enumerate tools")
}

func TestCycle147_Adjacency_PlaneResolver_ResolutionAndErrors(t *testing.T) {
	ctx := context.Background()
	resolver := kernel.NewPlaneMapResolver()
	require.NotNil(t, resolver)

	uUnknown := makeCycle147URN(t, "kernel", "item", "unregistered-urn")
	uRegistered := makeCycle147URN(t, "kernel", "item", "registered-urn")

	// Resolution on missing URN
	plane, err := resolver.ResolvePlane(ctx, uUnknown)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plane not registered for urn")
	assert.Equal(t, dna.PlaneDraft, plane)

	// Register plane and verify lookup
	resolver.SetPlane(uRegistered, dna.PlaneStaged)
	plane, err = resolver.ResolvePlane(ctx, uRegistered)
	require.NoError(t, err)
	assert.Equal(t, dna.PlaneStaged, plane)

	// Mutate registered plane to promoted
	resolver.SetPlane(uRegistered, dna.PlanePromoted)
	plane, err = resolver.ResolvePlane(ctx, uRegistered)
	require.NoError(t, err)
	assert.Equal(t, dna.PlanePromoted, plane)
}

func TestCycle147_PlaneBoundaryEnforcer_ResolutionFailures(t *testing.T) {
	ctx := context.Background()
	resolver := kernel.NewPlaneMapResolver()
	enforcer := kernel.NewPlaneBoundaryEnforcer(resolver)

	uSrc := makeCycle147URN(t, "cell", "node", "src-1")
	uTgt := makeCycle147URN(t, "cell", "node", "tgt-1")

	// Both unregistered: source resolution fails first
	edge := kernel.Edge{Source: uSrc, Target: uTgt, Kind: "causal"}
	err := enforcer.ValidateEdge(ctx, edge)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source plane resolution failed")

	// Source registered, target unregistered: target resolution fails
	resolver.SetPlane(uSrc, dna.PlaneDraft)
	err = enforcer.ValidateEdge(ctx, edge)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target plane resolution failed")

	// Nil resolver bypasses validation cleanly
	nilEnforcer := kernel.NewPlaneBoundaryEnforcer(nil)
	err = nilEnforcer.ValidateEdge(ctx, edge)
	require.NoError(t, err)
}

func TestCycle147_PlaneBoundaryEnforcer_ApoptoticAndCausalRules(t *testing.T) {
	ctx := context.Background()
	resolver := kernel.NewPlaneMapResolver()
	enforcer := kernel.NewPlaneBoundaryEnforcer(resolver)

	uDraft := makeCycle147URN(t, "cell", "node", "draft")
	uStaged := makeCycle147URN(t, "cell", "node", "staged")
	uPromoted := makeCycle147URN(t, "cell", "node", "promoted")
	uApoptotic := makeCycle147URN(t, "cell", "node", "apoptotic")

	resolver.SetPlane(uDraft, dna.PlaneDraft)
	resolver.SetPlane(uStaged, dna.PlaneStaged)
	resolver.SetPlane(uPromoted, dna.PlanePromoted)
	resolver.SetPlane(uApoptotic, dna.PlaneApoptotic)

	// Source apoptotic rejection
	err := enforcer.ValidateEdge(ctx, kernel.Edge{Source: uApoptotic, Target: uDraft, Kind: "relates_to"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apoptotic objects cannot participate in active edges")

	// Promoted -> Staged causal dependency rejection
	err = enforcer.ValidateEdge(ctx, kernel.Edge{Source: uPromoted, Target: uStaged, Kind: "causal"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "promoted object")
	assert.Contains(t, err.Error(), "cannot depend on unpromoted object")

	// Promoted -> Staged depends_on dependency rejection
	err = enforcer.ValidateEdge(ctx, kernel.Edge{Source: uPromoted, Target: uStaged, Kind: "depends_on"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "promoted object")

	// Draft -> Promoted depends_on dependency rejection
	err = enforcer.ValidateEdge(ctx, kernel.Edge{Source: uDraft, Target: uPromoted, Kind: "depends_on"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "draft object")
	assert.Contains(t, err.Error(), "cannot form causal dependency to promoted object")

	// Promoted -> Promoted causal dependency allowed
	uPromoted2 := makeCycle147URN(t, "cell", "node", "promoted-2")
	resolver.SetPlane(uPromoted2, dna.PlanePromoted)
	err = enforcer.ValidateEdge(ctx, kernel.Edge{Source: uPromoted, Target: uPromoted2, Kind: "causal"})
	require.NoError(t, err)
}

func TestCycle147_AdjacencyEngine_EdgeOperationsAndValidation(t *testing.T) {
	ctx := context.Background()
	engine := kernel.NewMemoryAdjacencyEngine(nil)

	uSrc := makeCycle147URN(t, "cell", "node", "src")
	uTgt := makeCycle147URN(t, "cell", "node", "tgt")
	emptyURN := dna.URN{}

	// Empty source or target URN rejection
	err := engine.AddEdge(ctx, kernel.Edge{Source: emptyURN, Target: uTgt, Kind: "causal"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source and target URNs must be non-empty")

	err = engine.AddEdge(ctx, kernel.Edge{Source: uSrc, Target: emptyURN, Kind: "causal"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source and target URNs must be non-empty")

	// Empty graph queries return empty slices without panic
	uMissing := makeCycle147URN(t, "cell", "node", "missing")
	outMissing, err := engine.GetOutbound(ctx, uMissing)
	require.NoError(t, err)
	assert.Empty(t, outMissing)

	inMissing, err := engine.GetInbound(ctx, uMissing)
	require.NoError(t, err)
	assert.Empty(t, inMissing)

	// Add valid edge with weights and provenance
	prov := dna.Provenance{ParentHash: "hash-root-123", AgentID: "agent-1"}
	edge1 := kernel.Edge{Source: uSrc, Target: uTgt, Kind: "causal", Weight: 4.5, Provenance: prov}
	err = engine.AddEdge(ctx, edge1)
	require.NoError(t, err)

	// Idempotent duplicate addition
	err = engine.AddEdge(ctx, edge1)
	require.NoError(t, err)

	outEdges, err := engine.GetOutbound(ctx, uSrc)
	require.NoError(t, err)
	require.Len(t, outEdges, 1)
	assert.Equal(t, 4.5, outEdges[0].Weight)
	assert.Equal(t, prov, outEdges[0].Provenance)

	// Add second edge with different kind
	edge2 := kernel.Edge{Source: uSrc, Target: uTgt, Kind: "references", Weight: 1.0}
	err = engine.AddEdge(ctx, edge2)
	require.NoError(t, err)

	outEdges, err = engine.GetOutbound(ctx, uSrc)
	require.NoError(t, err)
	assert.Len(t, outEdges, 2)

	// RemoveEdge with specific kind removes only that kind
	err = engine.RemoveEdge(ctx, uSrc, uTgt, "causal")
	require.NoError(t, err)

	outEdges, err = engine.GetOutbound(ctx, uSrc)
	require.NoError(t, err)
	require.Len(t, outEdges, 1)
	assert.Equal(t, "references", outEdges[0].Kind)

	// RemoveEdge with empty kind removes all edges between source and target
	err = engine.RemoveEdge(ctx, uSrc, uTgt, "")
	require.NoError(t, err)

	outEdges, err = engine.GetOutbound(ctx, uSrc)
	require.NoError(t, err)
	assert.Empty(t, outEdges)

	inEdges, err := engine.GetInbound(ctx, uTgt)
	require.NoError(t, err)
	assert.Empty(t, inEdges)
}

func TestCycle147_AdjacencyEngine_CycleDetectionTopologies(t *testing.T) {
	ctx := context.Background()
	engine := kernel.NewMemoryAdjacencyEngine(nil)

	// 1. Empty graph has no cycles
	hasCycle, err := engine.HasCycle(ctx)
	require.NoError(t, err)
	assert.False(t, hasCycle)

	cycles, err := engine.DetectCycles(ctx)
	require.NoError(t, err)
	assert.Empty(t, cycles)

	// 2. Diamond DAG: A -> B, A -> C, B -> D, C -> D (Acyclic)
	uA := makeCycle147URN(t, "graph", "node", "A")
	uB := makeCycle147URN(t, "graph", "node", "B")
	uC := makeCycle147URN(t, "graph", "node", "C")
	uD := makeCycle147URN(t, "graph", "node", "D")

	require.NoError(t, engine.AddEdge(ctx, kernel.Edge{Source: uA, Target: uB, Kind: "flow"}))
	require.NoError(t, engine.AddEdge(ctx, kernel.Edge{Source: uA, Target: uC, Kind: "flow"}))
	require.NoError(t, engine.AddEdge(ctx, kernel.Edge{Source: uB, Target: uD, Kind: "flow"}))
	require.NoError(t, engine.AddEdge(ctx, kernel.Edge{Source: uC, Target: uD, Kind: "flow"}))

	hasCycle, err = engine.HasCycle(ctx)
	require.NoError(t, err)
	assert.False(t, hasCycle)

	// 3. Close the loop D -> A to make it cyclic
	require.NoError(t, engine.AddEdge(ctx, kernel.Edge{Source: uD, Target: uA, Kind: "loop"}))
	hasCycle, err = engine.HasCycle(ctx)
	require.NoError(t, err)
	assert.True(t, hasCycle)

	detectedCycles, err := engine.DetectCycles(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, detectedCycles)

	// 4. Self-loop verification
	selfEngine := kernel.NewMemoryAdjacencyEngine(nil)
	uSelf := makeCycle147URN(t, "graph", "node", "self")
	require.NoError(t, selfEngine.AddEdge(ctx, kernel.Edge{Source: uSelf, Target: uSelf, Kind: "loop"}))

	hasSelfCycle, err := selfEngine.HasCycle(ctx)
	require.NoError(t, err)
	assert.True(t, hasSelfCycle)
}

func TestCycle147_AdjacencyEngine_ConcurrencyAndEnvelope(t *testing.T) {
	ctx := context.Background()
	engine := kernel.NewMemoryAdjacencyEngine(nil)

	const workerCount = 8
	const opsPerWorker = 30
	var wg sync.WaitGroup
	wg.Add(workerCount)

	for w := 0; w < workerCount; w++ {
		workerID := w
		goroutinelabels.NewGoroutine("cycle147_adj_worker", "concurrent graph stress").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				uSrc, errSrc := dna.NewURN("cell", "worker", "src")
				require.NoError(t, errSrc)
				uTgt, errTgt := dna.NewURN("cell", "worker", "tgt")
				require.NoError(t, errTgt)

				addEdgeErr := engine.AddEdge(ctx, kernel.Edge{
					Source: uSrc,
					Target: uTgt,
					Kind:   "stress",
					Weight: float64(workerID + i),
				})
				require.NoError(t, addEdgeErr)

				out, outErr := engine.GetOutbound(ctx, uSrc)
				require.NoError(t, outErr)
				assert.NotEmpty(t, out)

				in, inErr := engine.GetInbound(ctx, uTgt)
				require.NoError(t, inErr)
				assert.NotEmpty(t, in)

				_, hasCycleErr := engine.HasCycle(ctx)
				require.NoError(t, hasCycleErr)
			}
		})
	}
	wg.Wait()

	// Test ObjectEnvelope implementation
	env := cycle147Envelope{
		baseObj: dna.BaseObject{
			Version: 1,
			Plane:   dna.PlanePromoted,
			URN:     makeCycle147URN(t, "cell", "envelope", "item-1"),
		},
	}
	base := env.GetBaseObject()
	assert.Equal(t, uint64(1), base.Version)
	assert.Equal(t, dna.PlanePromoted, base.Plane)
	assert.Equal(t, "urn:zqk:cell:envelope:item-1", base.URN.String())
}
