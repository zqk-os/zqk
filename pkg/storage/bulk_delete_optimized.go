package storage

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"context"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/when"
)

// DependencyGraph represents the dependency relationships between objects
type DependencyGraph struct {
	nodes     map[string]*DependencyNode
	mu        sync.RWMutex
	leafKinds map[string]bool // Kinds that are known to be leaf nodes
}

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	ID         string
	Kind       string
	Dependents []string // IDs of objects that depend on this one
	DependsOn  []string // IDs of objects this one depends on
	Visited    bool
	Deleted    bool
}

// NewDependencyGraph creates a new dependency graph
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes: make(map[string]*DependencyNode),
		leafKinds: map[string]bool{
			objects.KindAuditEvent:             true,
			objects.KindAuditAggregationMetric: true,
			objects.KindBaseMetric:             true,
			objects.KindCommandMetric:          true,
			objects.KindChangeJournalEntry:     true,
			objects.KindSchedulerJob:           true, // No process objects reference scheduler_job
			objects.KindMcpSession:             true, // Safe leaf node
			objects.KindDocEntry:               true, // Safe leaf node
			objects.KindAgentInstruction:       true, // Transient agent instructions (leaf node)
		},
	}
}

// AddNode adds a node to the graph
func (g *DependencyGraph) AddNode(id, kind string) error {
	return concurrency.RunInLockWithLogger(&g.mu, locknames.LockNameDependencyGraphAddNode, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if _, exists := g.nodes[id]; !exists {
			g.nodes[id] = &DependencyNode{
				ID:         id,
				Kind:       kind,
				Dependents: make([]string, 0),
				DependsOn:  make([]string, 0),
			}
		}
		return nil
	})
}

// AddDependency adds a dependency relationship
func (g *DependencyGraph) AddDependency(dependentID, dependsOnID string) error {
	return concurrency.RunInLockWithLogger(&g.mu, locknames.LockNameDependencyGraphAddDependency, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		dependent, exists := g.nodes[dependentID]
		if !exists {
			return nil
		}
		dependsOn, exists := g.nodes[dependsOnID]
		if !exists {
			return nil
		}
		dependent.DependsOn = append(dependent.DependsOn, dependsOnID)
		dependsOn.Dependents = append(dependsOn.Dependents, dependentID)
		return nil
	})
}

// GetDeletionOrder returns the order in which objects should be deleted
// (leaf nodes first, then objects that depend on them)
func (g *DependencyGraph) GetDeletionOrder(ids []string) ([]string, error) {
	var deletionOrder []string
	err := concurrency.RunInRLockWithLogger(&g.mu, locknames.LockNameDependencyGraphGetDeletionOrder, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		for _, node := range g.nodes {
			node.Visited = false
		}
		deletionOrder = make([]string, 0, len(ids))
		visited := make(map[string]bool)
		var visit func(id string)
		visit = func(id string) {
			if visited[id] {
				return
			}
			visited[id] = true
			node, exists := g.nodes[id]
			if !exists {
				return
			}
			for _, dependentID := range node.Dependents {
				visit(dependentID)
			}
			deletionOrder = append(deletionOrder, id)
		}
		for _, id := range ids {
			_, exists := g.nodes[id]
			when.When(func() bool { return exists }).Then(func() {
				visit(id)
			}).OrElse(func() {
				deletionOrder = append(deletionOrder, id)
			}).Run()
		}
		return nil
	})
	return deletionOrder, err
}

// IsLeafNode checks if a kind is a known leaf node (nothing references it)
func (g *DependencyGraph) IsLeafNode(kind string) bool {
	var isLeaf bool
	err := concurrency.RunInRLockWithLogger(&g.mu, locknames.LockNameDependencyGraphIsLeafNode, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		isLeaf = g.leafKinds[kind]
		return nil
	})
	if err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockCheckLeafNode, err).Log()
	}
	return isLeaf
}

// resolveBulkOrphansOrRefuse decides what to do about batch members whose referrers are staying.
//
// It mirrors deleteImpl's sequence deliberately — filter to blocking dependents, honor an explicit
// unlink request, re-check, then refuse — so the two entry points give the same answer for the same
// graph. Where they differ is only the membership test: bulk excludes dependents that are themselves
// in the batch, because those are not survivors.
func (f *FileObjectStorage) resolveBulkOrphansOrRefuse(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	orphaned map[string][]string,
	deleting map[string]bool,
) error {
	blocking := make(map[string][]string, len(orphaned))
	for id, deps := range orphaned {
		if filtered := filterBlockingDependents(deps); len(filtered) > 0 {
			blocking[id] = filtered
		}
	}
	if len(blocking) == 0 {
		return nil
	}

	if UnlinkReferencesBeforeDelete(ctx) {
		for id, deps := range blocking {
			if err := UnlinkReferencesFromDependents(ctx, secCtx, f, id, deps); err != nil {
				return errfmt.Newf(ErrMsgUnlinkRefsFail).Wrap(err)
			}
		}
		if f.projectRoot != emptyValue {
			_ = caspkg.FlushAllListingIndexesForProjectRootWithTimeout(f.projectRoot, IndexFlushAfterCreateTimeout)
		}
		remaining := make(map[string][]string, len(blocking))
		for id := range blocking {
			// findDependents ignores its kind argument; the lookup is by id through the reverse index.
			deps, err := f.findDependents(ctx, id, emptyValue)
			if err != nil {
				return errfmt.Newf(ErrMsgCheckDepsAfterUnlink).Wrap(err)
			}
			survivors := make([]string, 0, len(deps))
			for _, dep := range deps {
				if !deleting[dep] {
					survivors = append(survivors, dep)
				}
			}
			if filtered := filterBlockingDependents(survivors); len(filtered) > 0 {
				remaining[id] = filtered
			}
		}
		blocking = remaining
		if len(blocking) == 0 {
			return nil
		}
	}

	// Sorted so the message is stable across runs; map iteration order is not.
	ids := make([]string, 0, len(blocking))
	total := 0
	for id, deps := range blocking {
		ids = append(ids, id)
		total += len(deps)
	}
	sort.Strings(ids)
	sample := ids[0]
	sampleDeps := append([]string(nil), blocking[sample]...)
	sort.Strings(sampleDeps)

	return errfmt.Errorf(
		"bulk delete refused: %d object(s) in this batch are still referenced by %d object(s) outside it, and erasing them would leave dangling references; e.g. %s is referenced by %v. Pass --unlink-references to strip the inbound references first, or --cascade to delete the referrers too",
		len(blocking), total, sample, sampleDeps,
	)
}

// BulkDeleteOptimized performs optimized bulk deletion with dependency graph and parallel processing
