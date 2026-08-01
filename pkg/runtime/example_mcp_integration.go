package runtime

// Example: Refactoring MCP Server to use GoroutineManager
//
// This example shows how to refactor the MCP server's periodic compression
// to use the GoroutineManager for proper lifecycle tracking.

/*
// BEFORE: Direct goroutine with no tracking
func (s *Server) Serve() error {
    // ...
    if s.clientMetricsStore == nil {
        store, _ := NewClientMetricsStore(metricsPath)
        s.clientMetricsStore = store

        // Start periodic compression - NO TRACKING!
        ticker := store.StartPeriodicCompression(24*time.Hour, 7*24*time.Hour)
        s.compressionTicker = ticker // Stored but cleanup is manual
    }
    // ...
}

// AFTER: Using GoroutineManager
func (s *Server) Serve() error {
    // ...
    if s.clientMetricsStore == nil {
        store, _ := NewClientMetricsStore(metricsPath)
        s.clientMetricsStore = store

        // Start periodic compression with full tracking
        ticker := time.NewTicker(24 * time.Hour)
        id, ctx, err := s.goroutineManager.Start(GoroutineConfig{
            Name:     "mcp_metrics_compression",
            Purpose:  "Periodically compress client metrics",
            Category: "background",
            Resources: []Resource{
                {
                    Type:        "ticker",
                    ID:          "compression_ticker",
                    Description: "Periodic compression ticker",
                    CleanupFunc: func() error {
                        ticker.Stop()
                        return nil
                    },
                },
            },
            Metadata: map[string]any{
                "interval":        "24h",
                "retention_period": "7d",
            },
        }, func(ctx context.Context) error {
            defer ticker.Stop()

            for {
                select {
                case <-ctx.Done():
                    return nil
                case <-ticker.C:
                    if err := store.CompressMetrics(7 * 24 * time.Hour); err != nil {
                        // Log error but continue
                        _ = err
                    }
                }
            }
        })

        if err != nil {
            return errfmt.Newf("failed to start compression goroutine").Wrap(err)
        }

        // Store ID for later reference (optional)
        s.compressionGoroutineID = id
    }
    // ...
}

// Shutdown now properly cleans up
func (s *Server) shutdownSequence(reason string) {
    // ...

    // Stop all managed goroutines (including compression)
    if s.goroutineManager != nil {
        if err := s.goroutineManager.Shutdown(); err != nil {
            // Log shutdown errors (leaks, timeouts)
            s.traceLogf("[MCP_WARN] Goroutine shutdown errors: %v", err)
        }
    }

    // ...
}
*/
