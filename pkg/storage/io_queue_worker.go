// Extracted from io_queue.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func (q *ioQueue) startWorker() {
	defer func() {
		q.workerRunning.Store(0)
		// Note: wg.Done() is called by goroutinelabels.WithWaitGroup() on goroutine exit
	}()

	batch := make([]*IOOperation, 0, q.batchSize)
	batchTicker := time.NewTicker(q.batchTimeout)
	defer batchTicker.Stop()

	idleTicker := time.NewTicker(q.idleTimeout)
	defer idleTicker.Stop()

	lastWorkTime := time.Now()

	for {
		select {
		case <-q.ctx.Done():
			// Process any remaining operations before shutdown
			if len(batch) > 0 {
				q.processBatch(batch)
			}
			return

		case op := <-q.operations:
			// New work arrived - reset idle timer
			q.queueDepth.Add(-1)
			lastWorkTime = time.Now()
			idleTicker.Stop()
			idleTicker.Reset(q.idleTimeout)

			batch = append(batch, op)
			if len(batch) >= q.batchSize {
				// Batch is full - process immediately
				q.processBatch(batch)
				batch = batch[:0]
				batchTicker.Stop()
				batchTicker.Reset(q.batchTimeout)
			}

		case <-batchTicker.C:
			// Batch timeout reached - process current batch
			// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting
			if q.ctx.Err() != nil {
				// Context cancelled - process any pending batch and exit
				if len(batch) > 0 {
					q.processBatch(batch)
				}
				return
			}
			if len(batch) > 0 {
				lastWorkTime = time.Now()
				q.processBatch(batch)
				batch = batch[:0]
			}

		case <-idleTicker.C:
			// Idle timeout reached - check if we should shut down
			// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting
			if q.ctx.Err() != nil {
				// Context cancelled - process any pending batch and exit
				if len(batch) > 0 {
					q.processBatch(batch)
				}
				return
			}
			queueSize := len(q.operations)
			if queueSize == 0 && len(batch) == 0 {
				idleDuration := time.Since(lastWorkTime)
				if idleDuration >= q.idleTimeout {
					// Been idle long enough - shut down worker (on-demand pattern)
					return
				}
				// Not idle long enough yet - reset timer
				idleTicker.Stop()
				idleTicker.Reset(q.idleTimeout - idleDuration)
			} else {
				// There's work - reset idle timer
				lastWorkTime = time.Now()
				idleTicker.Stop()
				idleTicker.Reset(q.idleTimeout)
			}
		}
	}
}

// processBatch processes a batch of I/O operations
func (q *ioQueue) processBatch(batch []*IOOperation) {
	for _, op := range batch {
		result := q.processOperation(op)
		// Send result (non-blocking)
		select {
		case op.Result <- result:
		default:
			// Result channel is full or closed - operation will timeout
		}
	}
}

// processOperation processes a single I/O operation
func (q *ioQueue) processOperation(op *IOOperation) IOResult {
	switch op.Type {
	case IOOperationRead:
		return q.processRead(op)
	case IOOperationWrite:
		return q.processWrite(op)
	default:
		return IOResult{Err: errfmt.Errorf("unknown I/O operation type: %s", op.Type)}
	}
}

// processRead processes a read operation
func (q *ioQueue) processRead(op *IOOperation) IOResult {
	// Perform read operation directly (bypass queue to avoid circular call)
	// Read file and parse YAML
	data, err := fileutil.ReadFile(op.FilePath)
	if fileutil.IsNotExist(err) {
		return IOResult{Err: errfmt.Errorf(ConstMiscFileNotFoundS, op.FilePath)}
	}
	if err != nil {
		return IOResult{Err: errfmt.Errorf(ConstMiscFailedToReadFileSW, op.FilePath, err)}
	}

	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return IOResult{Err: errfmt.Errorf(ConstMiscFailedToParseYamlFromSW, op.FilePath, err)}
	}

	// Return result with object data
	return IOResult{
		Obj: obj,
		Err: nil,
	}
}

// processWrite processes a write operation
func (q *ioQueue) processWrite(op *IOOperation) IOResult {
	// Validate write operation has required data
	if len(op.Data) == 0 {
		return IOResult{Err: errfmt.Errorf(ConstMiscWriteOperationRequiresDataFileS, op.FilePath)}
	}

	// Determine file permissions (default to 0600 if not specified)
	perm := op.Perm
	if perm == 0 {
		perm = 0600
	}

	// Perform write operation directly (bypass queue to avoid circular call)
	// Ensure directory exists
	dirPath := filepath.Dir(op.FilePath)
	if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		return IOResult{Err: errfmt.Errorf(ConstMiscFailedToCreateDirectoryForSW, op.FilePath, err)}
	}

	// Write file atomically using DSIA Storage Provider
	dsiaProvider := NewDSIAStorageProvider()
	if err := dsiaProvider.AtomicWriteFile(op.FilePath, op.Data, perm); err != nil {
		return IOResult{Err: errfmt.Errorf(ConstMiscFailedToWriteFileSW, op.FilePath, err)}
	}

	// Return success result
	return IOResult{
		Err: nil,
	}
}

// InitiateShutdown implements QueueShutdownHandler
// Stops accepting new operations
