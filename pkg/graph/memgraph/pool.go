package memgraph

import (
	"context"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
)

// MemGraphConnectionPool implements the ConnectionPool interface for MemGraph
type MemGraphConnectionPool struct {
	config      MemGraphConfig
	connections chan *memgraphConnection
	maxSize     int
	mu          sync.RWMutex
	stats       poolStats
}

// poolStats tracks pool statistics
type poolStats struct {
	active    int
	idle      int
	maxSize   int
	waitCount int64
}

// NewMemGraphConnectionPool creates a new MemGraph connection pool
func NewMemGraphConnectionPool(config *MemGraphConfig) (*MemGraphConnectionPool, error) {
	maxSize := config.PoolSize
	if maxSize <= 0 {
		maxSize = 10 // Default pool size
	}

	pool := &MemGraphConnectionPool{
		config:      *config,
		connections: make(chan *memgraphConnection, maxSize),
		maxSize:     maxSize,
		stats: poolStats{
			maxSize: maxSize,
		},
	}

	// Pre-populate pool with initial connections (lazy initialization)
	// Connections will be created on-demand in GetConnection

	return pool, nil
}

// newConnection creates a new MemGraph connection
func newConnection(config *MemGraphConfig) (*memgraphConnection, error) {
	boltClient, err := newBoltClient(config)
	if err != nil {
		return nil, errfmt.Newf("failed to create Bolt client").Wrap(err)
	}

	return &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     boltClient,
		config:         *config,
	}, nil
}

// GetConnection acquires a connection from the pool
func (p *MemGraphConnectionPool) GetConnection(ctx context.Context) (provider.GraphConnection, error) {
	// Try to get from channel (non-blocking)
	select {
	case conn := <-p.connections:
		// Got connection from pool
		logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
		_ = concurrency.WithLockLogger(
			&p.mu,
			LockNameMemgraphPoolGetFromPool,
			logger,
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
		logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
		_ = concurrency.WithRLockLogger(
			&p.mu,
			LockNameMemgraphPoolCheckActive,
			logger,
			func() error {
				currentActive = p.stats.active
				return nil
			},
		)

		if currentActive < p.maxSize {
			// Create new connection
			conn, err := newConnection(&p.config)
			if err != nil {
				return nil, err
			}
			_ = concurrency.WithLockLogger(
				&p.mu,
				LockNameMemgraphPoolIncActiveNew,
				logger,
				func() error {
					p.stats.active++
					return nil
				},
			)
			return conn, nil
		}

		// At max capacity, wait for a connection to be returned
		_ = concurrency.WithLockLogger(
			&p.mu,
			LockNameMemgraphPoolIncWait,
			logger,
			func() error {
				p.stats.waitCount++
				return nil
			},
		)

		select {
		case conn := <-p.connections:
			// Got connection from waiting
			_ = concurrency.WithLockLogger(
				&p.mu,
				LockNameMemgraphPoolGetFromWait,
				logger,
				func() error {
					// Only update stats if we actually got a connection from the pool
					// (idle should already be decremented when it was returned)
					if p.stats.idle > 0 {
						p.stats.idle--
					}
					p.stats.active++
					return nil
				},
			)
			return conn, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// ReturnConnection returns a connection to the pool
func (p *MemGraphConnectionPool) ReturnConnection(conn provider.GraphConnection) error {
	// TODO: Implement connection return
	// 1. Check if connection has open transaction
	// 2. If yes, rollback the transaction
	// 3. Reset connection state
	// 4. Return to pool channel
	// 5. Update stats

	// Type assert to our internal type
	mgConn, ok := conn.(*memgraphConnection)
	if !ok {
		return errfmt.Errorf("invalid connection type")
	}

	// Rollback any open transaction
	if mgConn.HasOpenTransaction() {
		if tx := mgConn.GetOpenTransaction(); tx != nil {
			//nolint:errcheck // Intentional error ignored
			// Use system context for transaction rollback during connection cleanup
			_ = tx.Rollback(pkgctx.NewSystemContext())
		}
	}

	// Reset connection state
	mgConn.Reset()

	// Return to pool (non-blocking)
	select {
	case p.connections <- mgConn:
		// Successfully returned to pool
		_ = concurrency.RunInLockWithLogger(
			&p.mu, LockNameMemgraphPoolReturnUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				p.stats.active--
				p.stats.idle++
				// Ensure stats don't exceed maxSize (safety check)
				if p.stats.idle > p.maxSize {
					p.stats.idle = p.maxSize
				}
				if p.stats.active < 0 {
					p.stats.active = 0
				}
				return nil
			},
		)
		return nil
	default:
		// Pool channel is full (shouldn't happen, but handle gracefully)
		// This means we have more connections than maxSize - close this one
		_ = concurrency.RunInLockWithLogger(
			&p.mu, LockNameMemgraphPoolReturnFull, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				p.stats.active--
				if p.stats.active < 0 {
					p.stats.active = 0
				}
				return nil
			},
		)
		return mgConn.CloseInternal()
	}
}

// Execute executes an operation using a connection from the pool
func (p *MemGraphConnectionPool) Execute(ctx context.Context, fn func(conn provider.GraphConnection) error) error {
	conn, err := p.GetConnection(ctx)
	if err != nil {
		return err
	}

	err = fn(conn)

	// If context was canceled or timed out, the underlying socket might be blocked
	// reading a stream, so we must discard it to prevent driver panics (e.g., racingReader).
	if ctx.Err() != nil {
		// Discard connection entirely
		mgConn, ok := conn.(*memgraphConnection)
		if ok {
			_ = concurrency.RunInLockWithLogger(
				&p.mu, LockNameMemgraphPoolReturnFull, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					p.stats.active--
					if p.stats.active < 0 {
						p.stats.active = 0
					}
					return nil
				},
			)
			_ = mgConn.CloseInternal()
		} else {
			_ = p.ReturnConnection(conn)
		}
	} else {
		_ = p.ReturnConnection(conn)
	}

	return err
}

// Stats returns pool statistics
func (p *MemGraphConnectionPool) Stats() provider.PoolStats {
	var stats poolStats
	_ = concurrency.RunInRLockWithLogger(
		&p.mu, LockNameMemgraphPoolStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			stats = p.stats
			return nil
		},
	)
	return provider.PoolStats{
		Active:    stats.active,
		Idle:      stats.idle,
		MaxSize:   stats.maxSize,
		WaitCount: stats.waitCount,
	}
}

// Close closes the pool and all connections
func (p *MemGraphConnectionPool) Close() error {
	// TODO: Implement pool closure
	// 1. Rollback all open transactions
	// 2. Close all connections
	// 3. Close the channel (safely - check if already closed)

	// Close all connections in the pool first
	// Drain the channel
	for {
		select {
		case conn := <-p.connections:
			if err := conn.CloseInternal(); err != nil {
				// Log error but continue closing others
			}
		default:
			// Channel empty, proceed to close
			goto closeChannel
		}
	}

closeChannel:
	// Close the channel (will panic if already closed, but that's expected)
	// In production, use sync.Once or check channel state
	select {
	case <-p.connections:
		// Channel already closed or empty
	default:
		close(p.connections)
	}

	return nil
}
