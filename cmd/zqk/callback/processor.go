package callback

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/outputtypes"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Processor handles callback processing with queue and sorting
type Processor struct {
	queue         *Queue
	logFile       string
	appendMode    bool
	projectRoot   string
	outputFormat  string // canonical outputtypes ID (e.g. text, jsonl, json)
	processorCtx  context.Context
	processorStop context.CancelFunc
	mu            sync.Mutex
	logger        logging.Logger
}

var (
	globalProcessor     *Processor
	globalProcessorMu   sync.Mutex
	globalProcessorInit sync.Once
)

// GetProcessor returns the global callback processor instance (production use).
func GetProcessor() *Processor {
	globalProcessorInit.Do(func() {
		globalProcessor = &Processor{
			logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		}
	})
	return globalProcessor
}

// NewProcessorForTest returns a new Processor instance for tests. Each test should use
// its own processor and call Shutdown() in defer/t.Cleanup to avoid shared state and
// lock contention. Do not use in production; use GetProcessor() instead.
func NewProcessorForTest() *Processor {
	return &Processor{
		logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Initialize initializes the processor with configuration
func (p *Processor) Initialize(projectRoot, logFile string, appendMode bool, queueSize int, sorter Sorter, format string) error {
	return concurrency.WithLockTimeout(
		&p.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(p.logger),
		LockNameCallbackProcessorInitialize,
		func() error {
			p.projectRoot = projectRoot
			p.logFile = logFile
			p.appendMode = appendMode
			p.outputFormat = outputtypes.Normalize(format)

			// Create or update queue
			if p.queue == nil {
				p.queue = NewQueue(queueSize, sorter, p.logger)
			} else {
				// Update queue configuration if needed
				// For now, queue config is set at creation
			}

			// Stop existing processor if running
			if p.processorCtx != nil && p.processorStop != nil {
				p.processorStop()
			}

			// Start new processor
			p.processorCtx, p.processorStop = context.WithCancel(pkgctx.NewSystemContext()) //nolint:gosec // G118: cancel stored; invoked from Shutdown
			p.queue.StartProcessing(p.processorCtx, p.processEntry)

			return nil
		},
	)
}

// Shutdown shuts down the processor
func (p *Processor) Shutdown() {
	_ = concurrency.WithLockTimeout(
		&p.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(p.logger),
		LockNameCallbackProcessorShutdown,
		func() error {
			if p.queue != nil {
				p.queue.StopProcessing()
			}

			if p.processorStop != nil {
				p.processorStop()
				p.processorStop = nil
			}

			p.processorCtx = nil
			return nil
		},
	)
}

// Enqueue adds a callback entry to the queue
func (p *Processor) Enqueue(payload map[string]any) error {
	return concurrency.WithLockTimeout(
		&p.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(p.logger),
		LockNameCallbackProcessorEnqueue,
		func() error {
			if p.queue == nil {
				return errfmt.Errorf("processor not initialized")
			}

			entry := &CallbackEntry{
				Payload:   payload,
				Timestamp: time.Now().UTC(),
			}

			return p.queue.Enqueue(entry)
		},
	)
}

// ProcessDirect processes a callback directly (bypasses queue)
// This is used when queue is disabled (queue-size=0)
func (p *Processor) ProcessDirect(projectRoot, logFile string, appendMode bool, payload map[string]any, format string) error {
	return concurrency.WithLockTimeout(
		&p.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(p.logger),
		LockNameCallbackProcessorProcessDirect,
		func() error {
			// Determine log file if not provided
			if logFile == emptyValue {
				accountID := callbackSystemAccountID
				logsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CallbackDir)
				if err := fileutil.MkdirAll(logsDir, paths.DirPerm755); err != nil {
					return errfmt.Newf("failed to create logs directory").Wrap(err)
				}
				logFile = filepath.Join(logsDir, fmt.Sprintf("%s.log", accountID))
			}

			// Temporarily set log file, append mode, format, and project root for this call
			oldLogFile := p.logFile
			oldAppendMode := p.appendMode
			oldProjectRoot := p.projectRoot
			oldFormat := p.outputFormat
			p.logFile = logFile
			p.appendMode = appendMode
			p.projectRoot = projectRoot
			p.outputFormat = outputtypes.Normalize(format)

			defer func() {
				p.logFile = oldLogFile
				p.appendMode = oldAppendMode
				p.projectRoot = oldProjectRoot
				p.outputFormat = oldFormat
			}()

			return p.processEntry(&CallbackEntry{
				Payload:   payload,
				Timestamp: time.Now().UTC(),
			})
		},
	)
}

// processEntry processes a single callback entry
func (p *Processor) processEntry(entry *CallbackEntry) error {
	// Determine log file
	logFile := p.logFile
	if logFile == emptyValue {
		accountID := callbackSystemAccountID
		logsDir := filepath.Join(p.projectRoot, paths.ProjectDataDir, paths.CallbackDir)
		if err := fileutil.MkdirAll(logsDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create logs directory").Wrap(err)
		}
		logFile = filepath.Join(logsDir, fmt.Sprintf("%s.log", accountID))
	}

	// Ensure parent directory exists for custom log file paths
	// This allows callbacks to write to custom paths specified by users
	// Permission errors from os.MkdirAll will bubble up naturally if the
	// system user (scheduler context) doesn't have permission to create directories
	dirPath := filepath.Dir(logFile)
	if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		return errfmt.Errorf("failed to create log directory %s: %w", dirPath, err)
	}

	// Format log entry based on output format
	logEntry, err := p.formatLogEntry(entry)
	if err != nil {
		return errfmt.Newf("failed to format log entry").Wrap(err)
	}

	// Open log file
	flags := fileutil.O_WRONLY | fileutil.O_CREATE
	if p.appendMode {
		flags |= fileutil.O_APPEND
	} else {
		flags |= fileutil.O_TRUNC
	}

	file, err := fileutil.OpenFile(logFile, flags, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf("failed to open log file").Wrap(err)
	}
	defer file.Close()

	// Write log entry (as bytes for JSON formats, string for text)
	if _, err := file.Write(logEntry); err != nil {
		return errfmt.Newf("failed to write log entry").Wrap(err)
	}

	return nil
}

// formatLogEntry formats a callback entry based on the output format
// Returns bytes for JSON formats, or string bytes for text format
func (p *Processor) formatLogEntry(entry *CallbackEntry) ([]byte, error) {
	format := outputtypes.Normalize(p.outputFormat)
	if format == emptyValue {
		format = outputtypes.IDText // Default
	}

	switch format {
	case outputtypes.IDJSONL:
		return p.formatJSONL(entry)
	case outputtypes.IDJSON:
		return p.formatJSON(entry)
	case outputtypes.IDText:
		return []byte(p.formatText(entry)), nil
	default:
		return []byte(p.formatText(entry)), nil
	}
}

func (p *Processor) callbackEventMap(entry *CallbackEntry) map[string]any {
	event := make(map[string]any, len(entry.Payload)+1)
	event["timestamp"] = entry.Timestamp.Format(time.RFC3339)
	for k, v := range entry.Payload {
		event[k] = v
	}
	return event
}

// formatJSONL formats entry as JSONL (one JSON object per line, no trailing newline in object)
func (p *Processor) formatJSONL(entry *CallbackEntry) ([]byte, error) {
	jsonData, err := json.Marshal(p.callbackEventMap(entry))
	if err != nil {
		return nil, errfmt.Newf("failed to marshal JSON").Wrap(err)
	}
	return append(jsonData, '\n'), nil
}

// formatJSON formats entry as pretty-printed JSON
func (p *Processor) formatJSON(entry *CallbackEntry) ([]byte, error) {
	jsonData, err := json.MarshalIndent(p.callbackEventMap(entry), "", "  ")
	if err != nil {
		return nil, errfmt.Newf("failed to marshal JSON").Wrap(err)
	}
	return append(jsonData, '\n'), nil
}

// formatText formats entry as human-readable text (original format)
func (p *Processor) formatText(entry *CallbackEntry) string {
	payload := entry.Payload
	timestamp := entry.Timestamp.Format(time.RFC3339)

	jobID, _ := payload["job_id"].(string)
	callbackType, _ := payload[objects.FieldKeyCallbackType].(string)
	status := callbackStatusUnknown
	if success, ok := payload["success"].(bool); ok {
		if success {
			status = callbackStatusSuccess
		} else {
			status = callbackStatusError
		}
	} else if errStr, ok := payload["error"].(string); ok && errStr != emptyValue {
		status = callbackStatusError
	}

	var logEntry string
	if callbackType == callbackTypeCompletion || callbackType == callbackTypeError {
		duration, _ := payload["duration"].(float64)
		command, _ := payload[objects.FieldKeyCommand].(string)
		logEntry = fmt.Sprintf("[%s] Job: %s | Status: %s | Duration: %.2fs | Command: %s",
			timestamp, jobID, status, duration, command)

		if stdout, ok := payload["stdout"].(string); ok && stdout != emptyValue {
			logEntry += fmt.Sprintf("\n  Stdout: %s", stdout)
		}
		if stderr, ok := payload["stderr"].(string); ok && stderr != emptyValue {
			logEntry += fmt.Sprintf("\n  Stderr: %s", stderr)
		}
		if errStr, ok := payload["error"].(string); ok && errStr != emptyValue {
			logEntry += fmt.Sprintf("\n  Error: %s", errStr)
		}
	} else {
		// Status callback
		logEntry = fmt.Sprintf("[%s] Job: %s | Type: %s | Status: %s",
			timestamp, jobID, callbackType, status)
	}

	// Ensure single newline at end (no blank lines between entries)
	return logEntry + "\n"
}

// GetQueueSize returns the current queue size
func (p *Processor) GetQueueSize() int {
	var size int
	_ = concurrency.WithLockTimeout(
		&p.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(p.logger),
		LockNameCallbackProcessorGetQueueSize,
		func() error {
			if p.queue == nil {
				size = 0
				return nil
			}
			size = p.queue.Size()
			return nil
		},
	)
	return size
}
