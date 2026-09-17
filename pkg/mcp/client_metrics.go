package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ClientSequenceEvent represents a single event in the client lifecycle sequence
type ClientSequenceEvent struct {
	EventType   string         `json:"event_type"` // "connect", "disconnect", "initialize", "tools_list", "tools_call", etc.
	ClientID    string         `json:"client_id"`
	Timestamp   time.Time      `json:"timestamp"`
	Duration    time.Duration  `json:"duration,omitempty"` // Time since previous event in sequence
	Fields      map[string]any `json:"fields,omitempty"`   // Additional context (tool_count, method, etc.)
	SequenceID  string         `json:"sequence_id"`        // Unique ID for this connection sequence
	SequencePos int            `json:"sequence_pos"`       // Position in sequence (0 = first event)
}

// ClientSequenceMetrics tracks metrics for a client connection sequence
type ClientSequenceMetrics struct {
	SequenceID      string                `json:"sequence_id"`
	ClientID        string                `json:"client_id"`
	FirstEvent      time.Time             `json:"first_event"`
	LastEvent       time.Time             `json:"last_event"`
	TotalEvents     int                   `json:"total_events"`
	Events          []ClientSequenceEvent `json:"events"`
	EventTypes      map[string]int        `json:"event_types"`         // Count of each event type
	Anomalies       []string              `json:"anomalies,omitempty"` // e.g., "tools_list_before_initialize"
	TotalDuration   time.Duration         `json:"total_duration"`
	Initialized     bool                  `json:"initialized"`
	ToolsRegistered int                   `json:"tools_registered"`
}

// bufferedEvent represents an event waiting for clientID to be set
type bufferedEvent struct {
	EventType string
	Fields    map[string]any
	Timestamp time.Time
}

// ClientMetricsStore stores and manages MCP client sequence metrics
type ClientMetricsStore struct {
	filePath              string
	metrics               map[string]*ClientSequenceMetrics // sequence_id -> metrics
	buffer                map[string][]bufferedEvent        // sequence_id -> buffered events (waiting for clientID)
	eventsRecordedTotal   atomic.Int64                      // Lifetime events recorded
	sequencesCreatedTotal atomic.Int64                      // Lifetime sequences created
	mu                    sync.RWMutex                      // Protects metrics and buffer

	// Save queue for batching saves
	saveQueue   chan struct{}   // Signals that a save is needed
	saveWorker  sync.Once       // Ensures save worker is started only once
	shutdownCtx context.Context // Shutdown context from ProcessGroupManager
}

// GetClientMetricsStats returns lifetime counters for events recorded and sequences created.
func (s *ClientMetricsStore) GetClientMetricsStats() (eventsRecorded, sequencesCreated int64) {
	if s == nil {
		return 0, 0
	}
	return s.eventsRecordedTotal.Load(), s.sequencesCreatedTotal.Load()
}

// cloneClientSequenceMetrics returns a deep copy (Events slice copied).
func cloneClientSequenceMetrics(m *ClientSequenceMetrics) *ClientSequenceMetrics {
	if m == nil {
		return nil
	}
	c := *m
	eventsCopy := make([]ClientSequenceEvent, len(m.Events))
	copy(eventsCopy, m.Events)
	c.Events = eventsCopy
	return &c
}

// cloneClientSequenceMetricsMap deep-copies the map; empty input yields a new empty map.
func cloneClientSequenceMetricsMap(src map[string]*ClientSequenceMetrics) map[string]*ClientSequenceMetrics {
	if len(src) == 0 {
		return make(map[string]*ClientSequenceMetrics)
	}
	out := make(map[string]*ClientSequenceMetrics, len(src))
	for k, v := range src {
		out[k] = cloneClientSequenceMetrics(v)
	}
	return out
}

// NewClientMetricsStore creates a new client metrics store
// shutdownCtx should be from ProcessGroupManager.GetShutdownContext() for proper shutdown handling
func NewClientMetricsStore(filePath string, shutdownCtx context.Context) (*ClientMetricsStore, error) {
	store := &ClientMetricsStore{
		filePath:    filePath,
		metrics:     make(map[string]*ClientSequenceMetrics),
		buffer:      make(map[string][]bufferedEvent),
		saveQueue:   make(chan struct{}, 100), // Buffered channel for save requests
		shutdownCtx: shutdownCtx,
	}

	// Load existing metrics
	if err := store.Load(); err != nil {
		// If file doesn't exist, that's okay - start fresh
		if !fileutil.IsNotExist(err) {
			return nil, errfmt.Newf("failed to load metrics").Wrap(err)
		}
	}

	return store, nil
}

// StartSaveWorker starts the save worker using ProcessGroupManager
// This should be called after the store is created and ProcessGroupManager is available
// The worker will be registered as CRITICAL so it completes saves during shutdown
func (s *ClientMetricsStore) StartSaveWorker(pgm *ProcessGroupManager) {
	s.saveWorker.Do(func() {
		// Spawn save worker via ProcessGroupManager
		// Mark as CRITICAL so it completes saves during shutdown
		_, _ = pgm.SpawnGoroutine(
			"metrics-save-worker",
			"Metrics Save Worker",
			"Batches and saves client metrics periodically",
			true, // CRITICAL - must complete saves during shutdown
			s.runSaveWorker,
		)
	})
}

// Load loads metrics from file
func (s *ClientMetricsStore) Load() error {
	var filePath string
	_ = concurrency.RunInLockWithLogger(
		&s.mu, LockNameClientMetricsLoadPath, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			filePath = s.filePath
			return nil
		},
	)

	// Check if file exists (I/O outside lock)
	if _, err := fileutil.Stat(filePath); err != nil {
		if fileutil.IsNotExist(err) {
			_ = concurrency.RunInLockWithLogger(
				&s.mu, LockNameClientMetricsLoadInit, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					s.metrics = make(map[string]*ClientSequenceMetrics)
					return nil
				},
			)
			return nil
		}
		return err
	}

	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			_ = concurrency.RunInLockWithLogger(
				&s.mu, LockNameClientMetricsLoadInit2, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					s.metrics = make(map[string]*ClientSequenceMetrics)
					return nil
				},
			)
			return nil
		}
		return err
	}

	if len(data) == 0 {
		_ = concurrency.RunInLockWithLogger(
			&s.mu, LockNameClientMetricsLoadInit3, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				s.metrics = make(map[string]*ClientSequenceMetrics)
				return nil
			},
		)
		return nil
	}

	var fileData struct {
		Metrics map[string]*ClientSequenceMetrics `json:"metrics"`
	}

	if err := json.Unmarshal(data, &fileData); err != nil {
		// If parsing fails, start fresh
		_ = concurrency.RunInLockWithLogger(
			&s.mu, LockNameClientMetricsLoadInit4, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				s.metrics = make(map[string]*ClientSequenceMetrics)
				return nil
			},
		)
		return nil
	}

	// Update metrics (re-acquire lock)
	_ = concurrency.RunInLockWithLogger(
		&s.mu, LockNameClientMetricsLoadUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.metrics = fileData.Metrics
			if s.metrics == nil {
				s.metrics = make(map[string]*ClientSequenceMetrics)
			}
			return nil
		},
	)

	return nil
}

// Save queues a save operation (non-blocking)
// The actual save will be performed by the save worker, which batches saves
func (s *ClientMetricsStore) Save() error {
	// Non-blocking enqueue - if queue is full, save will happen on next batch anyway
	select {
	case s.saveQueue <- struct{}{}:
		// Save queued successfully
	default:
		// Queue full - that's okay, save worker will process saves periodically
	}
	return nil
}

// saveNow performs the actual save operation (called by save worker)
func (s *ClientMetricsStore) saveNow() error {
	var metricsCopy map[string]*ClientSequenceMetrics
	_ = concurrency.RunInRLockWithLogger(
		&s.mu, LockNameClientMetricsSaveCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			metricsCopy = cloneClientSequenceMetricsMap(s.metrics)
			return nil
		},
	)

	return s.saveMetrics(metricsCopy)
}

// runSaveWorker is the save worker function that runs in ProcessGroupManager
func (s *ClientMetricsStore) runSaveWorker(ctx context.Context) {
	// Batch saves: save every 5 seconds or when queue has items
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var pendingSave bool

	for {
		select {
		case <-ctx.Done():
			// Shutdown requested - perform final save (CRITICAL)
			if pendingSave {
				_ = s.saveNow() //nolint:errcheck // Best effort on shutdown
			}
			// Also perform final save even if no pending save (ensure data is persisted)
			_ = s.saveNow() //nolint:errcheck // Best effort on shutdown
			return

		case <-s.saveQueue:
			// Save requested - mark as pending
			pendingSave = true

		case <-ticker.C:
			// Periodic save - perform if pending
			if pendingSave {
				_ = s.saveNow() //nolint:errcheck // Metrics save errors are non-critical
				pendingSave = false
			}
		}
	}
}

// Close performs a final save (called during shutdown)
// The save worker will be stopped by ProcessGroupManager, but we ensure final save here
func (s *ClientMetricsStore) Close() error {
	// Perform final save
	return s.saveNow()
}

// saveMetrics saves metrics to file (called without lock)
func (s *ClientMetricsStore) saveMetrics(metrics map[string]*ClientSequenceMetrics) error {
	// Create directory if needed
	if err := fileutil.EnsureDir(filepath.Dir(s.filePath)); err != nil {
		return errfmt.Newf("failed to create metrics directory").Wrap(err)
	}

	fileData := struct {
		Metrics map[string]*ClientSequenceMetrics `json:"metrics"`
	}{
		Metrics: metrics,
	}

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal metrics").Wrap(err)
	}

	// Write to temp file first, then rename (atomic write)
	tmpPath := s.filePath + ".tmp"
	if err := fileutil.WriteSecureFile(tmpPath, data); err != nil {
		return errfmt.Newf("failed to write metrics file").Wrap(err)
	}

	if err := fileutil.Rename(tmpPath, s.filePath); err != nil {
		return errfmt.Newf("failed to rename metrics file").Wrap(err)
	}

	return nil
}

// RecordEvent records a client sequence event
// If clientID is empty, the event is buffered until clientID is set via UpdateClientID
func (s *ClientMetricsStore) RecordEvent(sequenceID, clientID, eventType string, fields map[string]any) error {
	now := time.Now()
	return concurrency.RunInLockWithLogger(
		&s.mu, LockNameClientMetricsRecordEvent, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// If clientID is empty, buffer the event for later flushing
			if clientID == emptyValue {
				buffered := bufferedEvent{
					EventType: eventType,
					Fields:    fields,
					Timestamp: now,
				}
				s.buffer[sequenceID] = append(s.buffer[sequenceID], buffered)
				return nil // Don't save yet - wait for clientID
			}

			// ClientID is available - flush any buffered events first, then record this event
			if buffered, exists := s.buffer[sequenceID]; exists && len(buffered) > 0 {
				for _, bufferedEvent := range buffered {
					// Record each buffered event with the clientID
					if err := s.recordEventWithClientID(sequenceID, clientID, bufferedEvent.EventType, bufferedEvent.Fields, bufferedEvent.Timestamp); err != nil {
						// Log error but continue flushing other events
						logging.FluentEvent(logging.GetLogger()).Error("Failed to record buffered event", err).Log()
					}
				}
				// Clear buffer after flushing
				delete(s.buffer, sequenceID)
			}

			// Now record the current event
			return s.recordEventWithClientID(sequenceID, clientID, eventType, fields, now)
		},
	)
}

// recordEventWithClientID records an event with a known clientID
// This is called both for direct events and when flushing buffered events
// The caller must hold s.mu.Lock() - this function will unlock/relock around Save()
func (s *ClientMetricsStore) recordEventWithClientID(sequenceID, clientID, eventType string, fields map[string]any, eventTime time.Time) error {
	metrics, exists := s.metrics[sequenceID]
	if !exists {
		// New sequence
		metrics = &ClientSequenceMetrics{
			SequenceID: sequenceID,
			ClientID:   clientID,
			FirstEvent: eventTime,
			LastEvent:  eventTime,
			Events:     make([]ClientSequenceEvent, 0),
			EventTypes: make(map[string]int),
			Anomalies:  make([]string, 0),
		}
		s.metrics[sequenceID] = metrics
		s.sequencesCreatedTotal.Add(1)
	} else if metrics.ClientID == emptyValue {
		// Update clientID if it was empty before
		metrics.ClientID = clientID
	}

	// Calculate duration since last event
	var duration time.Duration
	if len(metrics.Events) > 0 {
		lastEvent := metrics.Events[len(metrics.Events)-1]
		duration = eventTime.Sub(lastEvent.Timestamp)
	} else {
		// First event - use FirstEvent as baseline
		duration = eventTime.Sub(metrics.FirstEvent)
	}

	// Create event
	event := ClientSequenceEvent{
		EventType:   eventType,
		ClientID:    clientID,
		Timestamp:   eventTime,
		Duration:    duration,
		Fields:      fields,
		SequenceID:  sequenceID,
		SequencePos: len(metrics.Events),
	}

	// Add to sequence
	metrics.Events = append(metrics.Events, event)
	metrics.EventTypes[eventType]++
	metrics.TotalEvents++
	metrics.LastEvent = eventTime
	metrics.TotalDuration = eventTime.Sub(metrics.FirstEvent)
	s.eventsRecordedTotal.Add(1)

	// Detect anomalies
	s.detectAnomalies(metrics, event)

	// Update state based on event type
	switch eventType {
	case "initialize":
		metrics.Initialized = true
		switch toolCount := fields["tools_count"].(type) {
		case int:
			metrics.ToolsRegistered = toolCount
		case float64:
			metrics.ToolsRegistered = int(toolCount)
		}
	case "tools_list":
		// Note: tools_list_before_initialize anomaly is detected in detectAnomalies()
		// to avoid duplicates, so we don't add it here
		switch toolCount := fields["tools_count"].(type) {
		case int:
			metrics.ToolsRegistered = toolCount
		case float64:
			metrics.ToolsRegistered = int(toolCount)
		}
	}

	// Queue save operation (non-blocking)
	// Save worker will batch saves and save periodically
	// No need to unlock/relock since Save() is now non-blocking
	err := s.Save()
	return err
}

// UpdateClientID updates the clientID for a sequence and flushes any buffered events
// This should be called when clientID becomes available (e.g., in handleNotificationInitialized)
func (s *ClientMetricsStore) UpdateClientID(sequenceID, clientID string) error {
	if clientID == emptyValue {
		return nil // Nothing to update
	}

	err := concurrency.RunInLockWithLogger(
		&s.mu, LockNameClientMetricsUpdateClientId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Update clientID in existing metrics if present
			if metrics, exists := s.metrics[sequenceID]; exists {
				if metrics.ClientID == emptyValue {
					metrics.ClientID = clientID
					// Update clientID in all existing events that had empty clientID
					for i := range metrics.Events {
						if metrics.Events[i].ClientID == emptyValue {
							metrics.Events[i].ClientID = clientID
						}
					}
				}
			}

			// Flush buffered events for this sequence
			if buffered, exists := s.buffer[sequenceID]; exists && len(buffered) > 0 {
				for _, bufferedEvent := range buffered {
					// Record each buffered event with the new clientID
					if err := s.recordEventWithClientID(sequenceID, clientID, bufferedEvent.EventType, bufferedEvent.Fields, bufferedEvent.Timestamp); err != nil {
						// Log error but continue flushing other events
						// In a real implementation, you'd want to use a logger here
						logging.FluentEvent(logging.GetLogger()).Error("Failed to record buffered event", err).Log()
					}
				}
				// Clear buffer after flushing
				delete(s.buffer, sequenceID)
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	// Save after updating (unlock before Save since it uses RLock)
	return s.Save()
}

// detectAnomalies detects anomalies in the client sequence
//
//nolint:gocritic // Event passed by value for immutability
func (s *ClientMetricsStore) detectAnomalies(metrics *ClientSequenceMetrics, event ClientSequenceEvent) {
	// Helper to check if anomaly already exists
	hasAnomaly := func(anomaly string) bool {
		for _, existing := range metrics.Anomalies {
			if existing == anomaly {
				return true
			}
		}
		return false
	}

	// Check for tools/list before initialize
	if event.EventType == "tools_list" && !metrics.Initialized {
		anomaly := "tools_list_before_initialize"
		if !hasAnomaly(anomaly) {
			metrics.Anomalies = append(metrics.Anomalies, anomaly)
		}
	}

	// Check for very long gaps between events (possible connection issues)
	// Only record unique gap anomalies (one per event type)
	// Threshold: 5 minutes - gaps longer than this might indicate connection issues
	// This is well below typical idle timeouts (1.5h) but long enough to catch real problems
	longGapThreshold := DefaultLongGapThreshold
	if len(metrics.Events) > 1 && event.Duration > longGapThreshold {
		anomaly := fmt.Sprintf("long_gap_before_%s: %v", event.EventType, event.Duration)
		// Check if we already have a long_gap anomaly for this event type
		// (we only want to record the first occurrence, not every occurrence)
		hasGapAnomaly := false
		for _, existing := range metrics.Anomalies {
			if strings.HasPrefix(existing, fmt.Sprintf("long_gap_before_%s:", event.EventType)) {
				hasGapAnomaly = true
				break
			}
		}
		if !hasGapAnomaly {
			metrics.Anomalies = append(metrics.Anomalies, anomaly)
		}
	}
}

// GetSequenceMetrics returns metrics for a specific sequence
func (s *ClientMetricsStore) GetSequenceMetrics(sequenceID string) (*ClientSequenceMetrics, error) {
	var metrics *ClientSequenceMetrics
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&s.mu, LockNameClientMetricsGetSequence, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			metrics, ok = s.metrics[sequenceID]
			exists = ok
			return nil
		},
	)

	if !exists {
		return nil, errfmt.Errorf("sequence not found: %s", sequenceID)
	}

	return cloneClientSequenceMetrics(metrics), nil
}

// GetAllMetrics returns all sequence metrics
func (s *ClientMetricsStore) GetAllMetrics() (map[string]*ClientSequenceMetrics, error) {
	var metricsCopy map[string]*ClientSequenceMetrics
	_ = concurrency.RunInRLockWithLogger(
		&s.mu, LockNameClientMetricsGetAll, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			metricsCopy = cloneClientSequenceMetricsMap(s.metrics)
			return nil
		},
	)

	return metricsCopy, nil
}

// GetMetricsByClientID returns all sequences for a specific client
func (s *ClientMetricsStore) GetMetricsByClientID(clientID string) ([]*ClientSequenceMetrics, error) {
	var result []*ClientSequenceMetrics
	_ = concurrency.RunInRLockWithLogger(
		&s.mu, LockNameClientMetricsGetByClient, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, metrics := range s.metrics {
				if metrics.ClientID != clientID {
					continue
				}
				result = append(result, cloneClientSequenceMetrics(metrics))
			}
			return nil
		},
	)

	return result, nil
}

// CompressMetrics compresses old metrics by archiving sequences older than the retention period
// Old sequences are moved to monthly archive files, keeping the main file small and fast
func (s *ClientMetricsStore) CompressMetrics(retentionPeriod time.Duration) error {
	// Copy-out: Identify sequences to archive (under lock)
	var sequencesToArchive map[string]*ClientSequenceMetrics
	err := concurrency.RunInLockWithLogger(
		&s.mu, LockNameClientMetricsCompressCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if len(s.metrics) == 0 {
				return nil // Nothing to compress
			}

			cutoffTime := time.Now().Add(-retentionPeriod)
			sequencesToArchive = make(map[string]*ClientSequenceMetrics)

			// Identify sequences to archive (older than retention period)
			for sequenceID, metrics := range s.metrics {
				if metrics.LastEvent.Before(cutoffTime) {
					sequencesToArchive[sequenceID] = cloneClientSequenceMetrics(metrics)
				}
			}
			return nil
		},
	)
	if err != nil {
		return err
	}

	if len(sequencesToArchive) == 0 {
		return nil // Nothing to archive
	}

	// Process: Group sequences by month and perform I/O (outside lock)
	archivesByMonth := make(map[string]map[string]*ClientSequenceMetrics)
	for sequenceID, metrics := range sequencesToArchive {
		month := metrics.FirstEvent.Format("2006-01")
		if archivesByMonth[month] == nil {
			archivesByMonth[month] = make(map[string]*ClientSequenceMetrics)
		}
		archivesByMonth[month][sequenceID] = metrics
	}

	// Archive each month's sequences (I/O outside lock)
	archiveDir := filepath.Join(filepath.Dir(s.filePath), "archived")
	if err := fileutil.EnsureDir(archiveDir); err != nil {
		return errfmt.Newf("failed to create archive directory").Wrap(err)
	}

	for month, sequences := range archivesByMonth {
		archivePath := filepath.Join(archiveDir, fmt.Sprintf("client-metrics-%s.json", month))

		// Load existing archive if it exists
		existingArchive := make(map[string]*ClientSequenceMetrics)
		if data, err := fileutil.ReadFile(archivePath); err == nil {
			var fileData struct {
				Metrics map[string]*ClientSequenceMetrics `json:"metrics"`
			}
			if err := json.Unmarshal(data, &fileData); err == nil {
				existingArchive = fileData.Metrics
			}
		}

		// Merge new sequences into archive
		for sequenceID, metrics := range sequences {
			existingArchive[sequenceID] = metrics
		}

		// Save archive
		fileData := struct {
			Metrics map[string]*ClientSequenceMetrics `json:"metrics"`
		}{
			Metrics: existingArchive,
		}

		data, err := json.MarshalIndent(fileData, "", "  ")
		if err != nil {
			return errfmt.Newf("failed to marshal archive").Wrap(err)
		}

		// Write to temp file first, then rename (atomic write)
		tmpPath := archivePath + ".tmp"
		if err := fileutil.WriteSecureFile(tmpPath, data); err != nil {
			return errfmt.Newf("failed to write archive file").Wrap(err)
		}

		if err := fileutil.Rename(tmpPath, archivePath); err != nil {
			return errfmt.Newf("failed to rename archive file").Wrap(err)
		}

	}

	// Copy-in: Remove archived sequences from main metrics (under lock)
	sequenceIDsToRemove := make([]string, 0, len(sequencesToArchive))
	for sequenceID := range sequencesToArchive {
		sequenceIDsToRemove = append(sequenceIDsToRemove, sequenceID)
	}

	err = concurrency.RunInLockWithLogger(
		&s.mu, LockNameClientMetricsCompressRemove, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, sequenceID := range sequenceIDsToRemove {
				delete(s.metrics, sequenceID)
			}
			return nil
		},
	)
	if err != nil {
		return err
	}

	// Save the updated main metrics file (without archived sequences)
	return s.saveMetrics(s.metrics)
}

// StartPeriodicCompression starts a background goroutine that periodically compresses metrics.
// Default retention period is 7 days (sequences older than 7 days are archived).
//
// Lifecycle:
//   - Starts: Immediately when called
//   - Stops: When ctx is cancelled or ticker is stopped
//   - Cleanup: Ticker is stopped automatically when context is cancelled
//
// Resources:
//   - Creates one goroutine
//   - Uses one time.Ticker
//   - Accesses ClientMetricsStore (thread-safe via mutex)
//
// CRITICAL: The caller MUST:
//  1. Pass a cancellable context (use context.WithCancel)
//  2. Cancel the context on shutdown
//  3. Stop the returned ticker if context cancellation isn't sufficient
//
// Example:
//
//	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
//	ticker := store.StartPeriodicCompression(ctx, 24*time.Hour, 7*24*time.Hour)
//	defer ticker.Stop() // Backup cleanup
//	defer cancel()      // Primary cleanup
func (s *ClientMetricsStore) StartPeriodicCompression(ctx context.Context, interval, retentionPeriod time.Duration) *time.Ticker {
	ticker := time.NewTicker(interval)
	goroutinelabels.NewGoroutine("client_metrics_compression", "periodically compressing client metrics").
		WithCleanup(func() {
			ticker.Stop() // Ensure ticker is stopped when goroutine exits
		}).
		StartWithContext(ctx, func(ctx context.Context) error {
			for {
				select {
				case <-ctx.Done():
					// Context cancelled - exit gracefully
					return ctx.Err()
				case <-ticker.C:
					// Periodic compression tick
					if err := s.CompressMetrics(retentionPeriod); err != nil {
						// Log error but don't stop compression
						logging.FluentEvent(logging.GetLogger()).Error("Failed to compress metrics", err).Log()
					}
				}
			}
		})
	return ticker
}
