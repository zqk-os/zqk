package agentclaim

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type checkinWriteRequest struct {
	projectRoot string
	timer       *CheckinTimer
}

type CheckinWriteQueue struct {
	mu         sync.Mutex
	items      map[string]checkinWriteRequest
	shutdownCh chan chan struct{}
	doneCh     chan struct{}
}

var (
	globalQueue *CheckinWriteQueue
	queueOnce   sync.Once
)

func GetGlobalCheckinWriteQueue() *CheckinWriteQueue {
	queueOnce.Do(func() {
		globalQueue = &CheckinWriteQueue{
			items:      make(map[string]checkinWriteRequest),
			shutdownCh: make(chan chan struct{}),
			doneCh:     make(chan struct{}),
		}
		goroutinelabels.NewGoroutine("checkin_write_queue", "batch agent checkin disk writes").StartSimple(globalQueue.worker)
	})
	return globalQueue
}

func (q *CheckinWriteQueue) Enqueue(projectRoot string, timer *CheckinTimer) {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Reducer: deduplicate by path. Only the latest state is written.
	path := CheckinTimerPath(projectRoot, timer.TaskID)
	q.items[path] = checkinWriteRequest{projectRoot: projectRoot, timer: timer}
}

func (q *CheckinWriteQueue) FlushWait() {
	done := make(chan struct{})
	q.shutdownCh <- done
	<-done
}

func (q *CheckinWriteQueue) worker() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	defer close(q.doneCh)

	for {
		select {
		case <-ticker.C:
			q.flush()
		case waitCh := <-q.shutdownCh:
			q.flush()
			if waitCh != nil {
				close(waitCh)
			} else {
				return
			}
		}
	}
}

func (q *CheckinWriteQueue) flush() {
	q.mu.Lock()
	items := q.items
	q.items = make(map[string]checkinWriteRequest)
	q.mu.Unlock()

	for _, req := range items {
		_ = q.writeDirect(req.projectRoot, req.timer)
	}
}

func (q *CheckinWriteQueue) writeDirect(projectRoot string, timer *CheckinTimer) error {
	path := CheckinTimerPath(projectRoot, timer.TaskID)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	data, err := json.Marshal(timer)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, paths.FilePerm644)
}

func (q *CheckinWriteQueue) Stop() {
	q.shutdownCh <- nil
	<-q.doneCh
}
