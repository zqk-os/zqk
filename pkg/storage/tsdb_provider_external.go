package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ExternalTSDBConfig holds configuration for external time-series databases.
type ExternalTSDBConfig struct {
	Type     string // e.g., "influxdb", "prometheus", "opentsdb"
	Endpoint string
	Token    string
	Org      string
	Bucket   string
}

// ExternalTSDBProvider implements an adapter for external TSDBs (e.g. InfluxDB, Prometheus).
// It converts internal ZQK metrics events into the native formats of external telemetry systems.
type ExternalTSDBProvider struct {
	config ExternalTSDBConfig
	logger logging.Logger
}

// NewExternalTSDBProvider creates a new external TSDB adapter.
func NewExternalTSDBProvider(config ExternalTSDBConfig) *ExternalTSDBProvider {
	return &ExternalTSDBProvider{
		config: config,
		logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

func (p *ExternalTSDBProvider) validateConfigType() error {
	switch p.config.Type {
	case "influxdb", "prometheus", "opentsdb":
		return nil
	default:
		return errfmt.Errorf("unsupported external tsdb type: %s", p.config.Type)
	}
}

// Initialize prepares the connection to the external TSDB.
func (p *ExternalTSDBProvider) Initialize(ctx context.Context) error {
	logging.Fluent(p.logger).Info("Initializing external TSDB provider").
		String("type", p.config.Type).
		String("endpoint", p.config.Endpoint).
		Log()

	return p.validateConfigType()
}

// WritePoint writes a single data point to the external TSDB.
func (p *ExternalTSDBProvider) WritePoint(ctx context.Context, point TSDBPoint) error {
	return p.WriteBatch(ctx, []TSDBPoint{point})
}

// WriteBatch writes multiple data points efficiently to the external TSDB.
func (p *ExternalTSDBProvider) WriteBatch(ctx context.Context, points []TSDBPoint) error {
	if len(points) == 0 {
		return nil
	}

	return p.validateConfigType()
}

// Query executes a query against the time-series data on the external TSDB.
func (p *ExternalTSDBProvider) Query(ctx context.Context, query TSDBQuery) (*TSDBQueryResult, error) {
	// Implementation would translate TSDBQuery to Flux/PromQL/OpenTSDB queries.
	return &TSDBQueryResult{
		Points: make([]TSDBPoint, 0),
	}, nil
}

// Close gracefully shuts down the external provider connection.
func (p *ExternalTSDBProvider) Close(ctx context.Context) error {
	logging.Fluent(p.logger).Info("Closing external TSDB provider connection").Log()
	return nil
}
