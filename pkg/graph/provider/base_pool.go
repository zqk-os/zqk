package provider

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

// BasePool provides shared connection pool logic that can be embedded by provider implementations.
// It handles connection acquisition, release, statistics, and transaction safety.
type BasePool struct {
	connections chan ConnectionWrapper
	maxSize     int
	mu          sync.RWMutex
	stats       poolStatsInternal
	config      ConnectionConfig
	recorder    any // observability.Recorder - stored as any to avoid import cycles
}

// ConnectionWrapper is an interface that provider connections must implement
// to work with the base pool. This allows the pool to manage transaction state
// and connection lifecycle without knowing provider-specific details.
type ConnectionWrapper interface {
	GraphConnection

	// Internal methods for pool management
	HasOpenTransaction() bool
	GetOpenTransaction() GraphTransaction
	Reset()               // Reset connection state for reuse
	CloseInternal() error // Close the underlying connection
}

// poolStatsInternal tracks internal pool statistics
type poolStatsInternal struct {
	active    int
	idle      int
	maxSize   int
	waitCount int64
}

// NewBasePool creates a new base pool with shared connection management logic
func NewBasePool(config *ConnectionConfig, maxSize int) *BasePool {
	if maxSize <= 0 {
		maxSize = 10 // Default
	}

	return &BasePool{
		connections: make(chan ConnectionWrapper, maxSize),
		maxSize:     maxSize,
		stats: poolStatsInternal{
			maxSize: maxSize,
		},
		config: *config,
	}
}

// GetConnection acquires a connection from the pool
// Provider implementations should call this and then wrap the result
func (p *BasePool) GetConnection(ctx context.Context, createFn func() (ConnectionWrapper, error)) (ConnectionWrapper, error) {
	// Apply pool timeout if configured
	if p.config.PoolTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.PoolTimeout)
		defer cancel()
	}

	// Try to get existing connection (non-blocking)
	select {
	case conn := <-p.connections:
		_ = concurrency.RunInLockWithLogger(
			&p.mu, LockNameBasePoolGetUpdateStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				p.stats.idle--
				p.stats.active++
				return nil
			},
		)
		return conn, nil
	default:
		// No connection available, check if we can create a new one
		var currentActive int
		_ = concurrency.RunInRLockWithLogger(
			&p.mu, LockNameBasePoolGetCheckActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				currentActive = p.stats.active
				return nil
			},
		)

		if currentActive < p.maxSize {
			// Create new connection (I/O outside lock)
			conn, err := createFn()
			if err != nil {
				return nil, err
			}
			_ = concurrency.RunInLockWithLogger(
				&p.mu, LockNameBasePoolGetNewConn, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					p.stats.active++
					return nil
				},
			)
			return conn, nil
		}

		// At max capacity, wait for a connection to be returned
		_ = concurrency.RunInLockWithLogger(
			&p.mu, LockNameBasePoolGetWaitInc, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				p.stats.waitCount++
				return nil
			},
		)

		waitStart := time.Now()
		select {
		case <-ctx.Done():
			waitDuration := time.Since(waitStart)
			recorder := p.getPoolMetricsRecorder()
			if recorder != nil && recorder.IsEnabled() {
				builder := buildPoolWaitMetric(waitDuration)
				_ = recorder.Record("pool_wait", builder)
			}
			return nil, &GraphError{
				Code:    ErrorCodeTimeout,
				Message: "timeout waiting for connection from pool",
				Cause:   ctx.Err(),
			}
		case conn := <-p.connections:
			waitDuration := time.Since(waitStart)
			recorder := p.getPoolMetricsRecorder()
			if recorder != nil && recorder.IsEnabled() {
				waitBuilder := buildPoolWaitMetric(waitDuration)
				_ = recorder.Record("pool_wait", waitBuilder)
				acquiredBuilder := buildConnectionAcquiredMetric(waitDuration)
				_ = recorder.Record("connection_acquired", acquiredBuilder)
			}
			_ = concurrency.RunInLockWithLogger(
				&p.mu, LockNameBasePoolGetWaitUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					p.stats.idle--
					p.stats.active++
					return nil
				},
			)
			return conn, nil
		}
	}
}

// ReturnConnection returns a connection to the pool with transaction safety
func (p *BasePool) ReturnConnection(conn ConnectionWrapper) error {
	// Safety: Rollback any open transaction
	if conn.HasOpenTransaction() {
		if tx := conn.GetOpenTransaction(); tx != nil {
			//nolint:errcheck // Intentional error ignored
			_ = tx.Rollback(pkgctx.NewSystemContext())
		}
	}

	// Reset connection state
	conn.Reset()

	// Return to pool (non-blocking)
	select {
	case p.connections <- conn:
		recorder := p.getPoolMetricsRecorder()
		if recorder != nil && recorder.IsEnabled() {
			releasedBuilder := buildConnectionReleasedMetric()
			_ = recorder.Record("connection_released", releasedBuilder)
		}
		var active, idle, maxSize int
		_ = concurrency.RunInLockWithLogger(
			&p.mu, LockNameBasePoolReturnUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				p.stats.active--
				p.stats.idle++
				active = p.stats.active
				idle = p.stats.idle
				maxSize = p.maxSize
				return nil
			},
		)
		if recorder != nil && recorder.IsEnabled() {
			sizeBuilder := buildPoolSizeChangeMetric(active, idle, maxSize)
			_ = recorder.Record("pool_size_change", sizeBuilder)
		}
		return nil
	default:
		// Pool is full, close the connection
		return conn.CloseInternal()
	}
}

// Execute executes an operation using a connection from the pool
func (p *BasePool) Execute(ctx context.Context, createFn func() (ConnectionWrapper, error), fn func(GraphConnection) error) error {
	conn, err := p.GetConnection(ctx, createFn)
	if err != nil {
		return err
	}
	//nolint:errcheck // Connection cleanup - error acceptable
	defer func() { _ = p.ReturnConnection(conn) }()
	return fn(conn)
}

// Stats returns pool statistics
func (p *BasePool) Stats() PoolStats {
	var stats PoolStats
	_ = concurrency.RunInRLockWithLogger(
		&p.mu, LockNameBasePoolStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			stats = PoolStats{
				Active:    p.stats.active,
				Idle:      p.stats.idle,
				MaxSize:   p.stats.maxSize,
				WaitCount: p.stats.waitCount,
			}
			return nil
		},
	)
	return stats
}

// Close closes the pool and all connections
func (p *BasePool) Close() error {
	close(p.connections)

	// Close all connections in the pool
	for conn := range p.connections {
		// Rollback any open transactions
		if conn.HasOpenTransaction() {
			if tx := conn.GetOpenTransaction(); tx != nil {
				//nolint:errcheck // Intentional error ignored
				_ = tx.Rollback(pkgctx.NewSystemContext())
			}
			//nolint:errcheck // Connection cleanup - error acceptable
		}
		//nolint:errcheck // Connection cleanup - error acceptable
		_ = conn.CloseInternal()
	}

	return nil
}

// PrePopulate pre-populates the pool with initial connections
func (p *BasePool) PrePopulate(ctx context.Context, createFn func() (ConnectionWrapper, error), count int) error {
	if count <= 0 {
		count = p.maxSize / 2 // Default to half the pool size
	}
	if count > p.maxSize {
		count = p.maxSize
	}

	for i := 0; i < count; i++ {
		conn, err := createFn()
		if err != nil {
			// If we can't create initial connections, that's okay
			// They'll be created on demand
			continue
		}

		select {
		case p.connections <- conn:
			_ = concurrency.RunInLockWithLogger(
				&p.mu, LockNameBasePoolPrepopulate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					p.stats.idle++
					return nil
				},
			)
		//nolint:errcheck // Connection cleanup - error acceptable
		default:
			// Pool is full (shouldn't happen during pre-population)
			_ = conn.CloseInternal()
		}
	}

	return nil
}
