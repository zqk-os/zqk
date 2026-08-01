package scheduler

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// Event is a JSON-Lines friendly job coordination event.
type Event struct {
	Type           string         `json:"type"`
	Timestamp      time.Time      `json:"timestamp"`
	JobID          string         `json:"job_id"`
	ExecutionID    string         `json:"execution_id,omitempty"`
	ProcessID      int            `json:"process_id,omitempty"`
	PolicyDecision string         `json:"policy_decision,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// CoordinationChannel provides a minimal file-based publish/watch mechanism.
type CoordinationChannel struct {
	eventLog string

	mu          sync.RWMutex
	subscribers []chan Event
}

const (
	pipelineKindCoordinationChannelWatch = "coordination_channel_watch"

	// coordinationEventLogLockTimeout bounds wait when another process holds the events.jsonl flock.
	coordinationEventLogLockTimeout = 5 * time.Second
)

// NewCoordinationChannel constructs a channel rooted at:
//
//	{projectRoot}/.zqk/scheduler/events/coordination-bus.jsonl
//
// NewCoordinationChannel creates a new coordination channel
func NewCoordinationChannel(projectRoot string) CoordinationChannelInterface {
	eventsDir := filepath.Join(projectRoot, paths.ProjectDataDir, "scheduler", "events")
	eventLog := filepath.Join(eventsDir, "coordination-bus.jsonl")
	legacyEventLog := filepath.Join(eventsDir, "events.log")
	if err := migrateLegacyEventLog(legacyEventLog, eventLog); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to migrate legacy event log").WithError(err).Log()
	}
	return &CoordinationChannel{
		eventLog:    eventLog,
		subscribers: make([]chan Event, 0, 4),
	}
}

// migrateLegacyEventLog promotes the old events.log filename to events.jsonl.
// Best-effort: if migration fails, writers/readers continue using the new filename.
func migrateLegacyEventLog(legacyPath, newPath string) error {
	if legacyPath == emptyValue || newPath == emptyValue || legacyPath == newPath {
		return nil
	}
	if info, err := os.Stat(legacyPath); err != nil {
		_ = info // Acknowledged
		return nil
	}
	if info, err := os.Stat(newPath); err == nil {
		_ = info // Acknowledged
		return nil
	}
	return os.Rename(legacyPath, newPath)
}

// Subscribe registers an in-memory subscriber channel.
func (cc *CoordinationChannel) Subscribe() <-chan Event {
	if cc == nil {
		ch := make(chan Event)
		close(ch)
		return ch
	}
	ch := make(chan Event, 8)
	cc.mu.Lock()
	cc.subscribers = append(cc.subscribers, ch)
	cc.mu.Unlock()
	return ch
}

// PublishEvent appends a JSON object as a single JSONL line to the shared log.
func (cc *CoordinationChannel) PublishEvent(ev Event) error {
	if cc == nil {
		return errfmt.Errorf("coordination channel required")
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}

	if err := os.MkdirAll(filepath.Dir(cc.eventLog), paths.DirPerm755); err != nil {
		return errfmt.Newf("create events dir").Wrap(err)
	}

	line, err := json.Marshal(ev)
	if err != nil {
		return errfmt.Newf("marshal event").Wrap(err)
	}
	payload := append(line, '\n')

	lock, err := storagepkg.NewFileLock(cc.eventLog + paths.LockFileSuffix)
	if err != nil {
		return errfmt.Newf("new event log lock").Wrap(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to close event log lock").WithError(err).Log()
		}
	}()

	return lock.WithLockTimeout(coordinationEventLogLockTimeout, func() error {
		f, err := os.OpenFile(cc.eventLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, paths.FilePerm644)
		if err != nil {
			return errfmt.Newf("open event log").Wrap(err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				SLog(logger).Debug("Failed to close event log file").WithError(err).Log()
			}
		}()
		if _, err := f.Write(payload); err != nil {
			return errfmt.Newf("append event").Wrap(err)
		}
		return nil
	})
}

// buildWatchEventsPipeline constructs the pipeline for event fan-out.
func (cc *CoordinationChannel) buildWatchEventsPipeline(logger logging.Logger) *pipeline.Pipeline {
	return pipeline.NewBuilder(pipelineKindCoordinationChannelWatch, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			line, ok := payload.([]byte)
			if !ok || len(line) == 0 {
				return nil, nil
			}

			var ev Event
			if err := json.Unmarshal(line, &ev); err != nil {
				return nil, nil
			}
			return ev, nil
		}).
		AddStage(pipeline.StageNormalize, func(pctx *pipeline.Context, payload any) (any, error) {
			if payload == nil {
				return nil, nil
			}
			ev, ok := payload.(Event)
			if !ok {
				return nil, nil
			}
			if ev.Timestamp.IsZero() {
				ev.Timestamp = time.Now().UTC()
			}
			return ev, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			if payload == nil {
				return nil, nil
			}
			ev, ok := payload.(Event)
			if !ok {
				return nil, nil
			}

			cc.mu.RLock()
			for _, sub := range cc.subscribers {
				select {
				case sub <- ev:
				default:
				}
			}
			cc.mu.RUnlock()
			return payload, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return payload, nil
		}).
		Build()
}

// WatchEvents tails the event log and fan-outs new events to subscribers.
func (cc *CoordinationChannel) WatchEvents(ctx context.Context) error {
	if cc == nil {
		return errfmt.Errorf("coordination channel required")
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	pl := cc.buildWatchEventsPipeline(logger)

	if err := os.MkdirAll(filepath.Dir(cc.eventLog), paths.DirPerm755); err != nil {
		return errfmt.Newf("create events dir").Wrap(err)
	}

	f, err := os.OpenFile(cc.eventLog, os.O_CREATE|os.O_RDONLY, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf("open event log").Wrap(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			SLog(logger).Debug("Failed to close event log for watching").WithError(err).Log()
		}
	}()

	st, err := f.Stat()
	if err != nil {
		return errfmt.Newf("stat event log").Wrap(err)
	}
	if _, err := f.Seek(st.Size(), io.SeekStart); err != nil {
		return errfmt.Newf("seek event log").Wrap(err)
	}

	reader := bufio.NewReader(f)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return errfmt.Newf("read event log").Wrap(err)
		}
		if len(line) == 0 {
			continue
		}
		_, runErr := pl.Run(&pipeline.Context{Ctx: ctx}, line)
		// Stages are best-effort and should not return errors for malformed lines.
		if runErr != nil {
			return runErr
		}
	}
}
