package system

import (
	stdcontext "context"
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/concurrency"
)

// projectCacheBgState tracks in-flight background cache work per project root so tests (and any
// caller) can block on completion via [WaitProjectCacheBackgroundWork] instead of polling timeouts.
// Counters are incremented before spawning goroutines and decremented when the goroutine exits.
type projectCacheBgState struct {
	mu          sync.Mutex
	cond        *sync.Cond
	nObjectID   int
	nReverseRef int
}

var projectCacheBg sync.Map // string (projectRoot) -> *projectCacheBgState

func getOrCreateProjectCacheBgState(projectRoot string) *projectCacheBgState {
	if v, ok := projectCacheBg.Load(projectRoot); ok {
		return v.(*projectCacheBgState)
	}
	s := &projectCacheBgState{}
	s.cond = sync.NewCond(&s.mu)
	if v, loaded := projectCacheBg.LoadOrStore(projectRoot, s); loaded {
		return v.(*projectCacheBgState)
	}
	return s
}

func projectCacheBgIncObjectID(projectRoot string) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	s := getOrCreateProjectCacheBgState(projectRoot)
	s.mu.Lock()
	s.nObjectID++
	s.mu.Unlock()
}

func projectCacheBgDecObjectID(projectRoot string) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	v, ok := projectCacheBg.Load(projectRoot)
	if !ok {
		return
	}
	s := v.(*projectCacheBgState)
	s.mu.Lock()
	beforeIdle := s.nObjectID == 0 && s.nReverseRef == 0
	s.nObjectID--
	if s.nObjectID < 0 {
		s.nObjectID = 0
	}
	afterIdle := s.nObjectID == 0 && s.nReverseRef == 0
	becameIdle := !beforeIdle && afterIdle
	if afterIdle {
		s.cond.Broadcast()
	}
	s.mu.Unlock()
	if becameIdle {
		notifyCacheSidecarsIdle(projectRoot)
	}
}

func projectCacheBgIncReverseRef(projectRoot string) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	s := getOrCreateProjectCacheBgState(projectRoot)
	s.mu.Lock()
	s.nReverseRef++
	s.mu.Unlock()
}

func projectCacheBgDecReverseRef(projectRoot string) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	v, ok := projectCacheBg.Load(projectRoot)
	if !ok {
		return
	}
	s := v.(*projectCacheBgState)
	s.mu.Lock()
	beforeIdle := s.nObjectID == 0 && s.nReverseRef == 0
	s.nReverseRef--
	if s.nReverseRef < 0 {
		s.nReverseRef = 0
	}
	afterIdle := s.nObjectID == 0 && s.nReverseRef == 0
	becameIdle := !beforeIdle && afterIdle
	if afterIdle {
		s.cond.Broadcast()
	}
	s.mu.Unlock()
	if becameIdle {
		notifyCacheSidecarsIdle(projectRoot)
	}
}

var (
	cacheSidecarsIdleCallbackMu sync.Mutex
	cacheSidecarsIdleCallbacks  = map[uint64]func(projectRoot string){}
	cacheSidecarsIdleNextID     uint64
)

// RegisterCacheSidecarsIdleCallback registers fn to run synchronously (outside the cache-sidecar
// mutex) each time background object-id or reverse-reference index work for projectRoot reaches
// zero. Use for orchestration that must run after sidecars finish but without waiting on the
// coordinator async path; keep fn short. [WaitProjectCacheBackgroundWork] is the barrier for
// background counters reaching zero; idle callbacks may run after that wait returns (see
// [WaitProjectCacheBackgroundWork] doc).
//
// Do not synchronously start new sidecar work for the same projectRoot from fn (e.g. calling
// TriggerBackgroundObjectIDCacheBuild for that root): that risks re-entrancy, stacked idle edges,
// and orchestration cycles. Defer follow-up to another goroutine or queue with a generation/one-shot
// guard. See docs/process/architecture/concurrency-patterns-v1.0.md § Cache sidecar idle coordination.
//
// Returns remove to unregister.
func RegisterCacheSidecarsIdleCallback(fn func(projectRoot string)) (remove func()) {
	if fn == nil {
		return func() {}
	}
	id := atomic.AddUint64(&cacheSidecarsIdleNextID, 1)
	cacheSidecarsIdleCallbackMu.Lock()
	cacheSidecarsIdleCallbacks[id] = fn
	cacheSidecarsIdleCallbackMu.Unlock()
	return func() {
		cacheSidecarsIdleCallbackMu.Lock()
		delete(cacheSidecarsIdleCallbacks, id)
		cacheSidecarsIdleCallbackMu.Unlock()
	}
}

func notifyCacheSidecarsIdle(projectRoot string) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	cacheSidecarsIdleCallbackMu.Lock()
	cbs := make([]func(string), 0, len(cacheSidecarsIdleCallbacks))
	for _, fn := range cacheSidecarsIdleCallbacks {
		cbs = append(cbs, fn)
	}
	cacheSidecarsIdleCallbackMu.Unlock()
	for _, fn := range cbs {
		fn(projectRoot)
	}
	emitCacheSidecarsIdleViaCoordinator(stdcontext.Background(), projectRoot)
}

// WaitProjectCacheBackgroundWork blocks until all background work started for projectRoot by
// [TriggerBackgroundObjectIDCacheBuild], [TriggerBackgroundObjectIDCacheForceRebuild], or the
// async path of reverse-reference index build has finished. Completion is signaled by condition
// variables (not wall-clock polling): slow machines stay correct.
//
// Waiting uses [concurrency.WaitCondContext]: the caller goroutine holds the cond's mutex around
// cond.Wait, and ctx cancellation triggers a Broadcast via context.AfterFunc (see pkg/concurrency
// README § sync.Cond + context).
//
// When counters reach zero, [notifyCacheSidecarsIdle] runs [RegisterCacheSidecarsIdleCallback]
// hooks (synchronous, in-process orchestration) and [emitCacheSidecarsIdleViaCoordinator] (async
// observability for subscribers/metrics). Those are not substitutes for this wait primitive.
// Idle hooks are invoked after the sidecar mutex is released; a waiter may therefore return from
// this function before those callbacks run (ordering is not guaranteed vs. wait completion).
//
// If ctx is cancelled before work finishes, returns ctx.Err(); background goroutines may still run.
// Typical test cleanup: pass [stdcontext.Background]; rely on go test's overall -timeout as the
// hang guard.
func WaitProjectCacheBackgroundWork(ctx stdcontext.Context, projectRoot string) error {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return nil
	}
	v, ok := projectCacheBg.Load(projectRoot)
	if !ok {
		return nil
	}
	s := v.(*projectCacheBgState)
	return concurrency.WaitCondContext(ctx, s.cond, &s.mu, func() bool {
		return s.nObjectID == 0 && s.nReverseRef == 0
	})
}
