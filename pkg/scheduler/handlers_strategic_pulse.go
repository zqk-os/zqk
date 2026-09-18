package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// StrategicPulseHandler periodically captures project health metrics (PCS, EDD)
// and persists them to the metrics history lake for trend analysis.
type StrategicPulseHandler struct {
	storage     storage.ObjectStorageProvider
	projectRoot string
}

// NewStrategicPulseHandler creates a new StrategicPulseHandler.
// NewStrategicPulseHandler creates a new strategic pulse handler
func NewStrategicPulseHandler(sp storage.ObjectStorageProvider, projectRoot string) StrategicPulseHandlerInterface {
	return &StrategicPulseHandler{
		storage:     sp,
		projectRoot: projectRoot,
	}
}

// Execute performs the strategic pulse capture.
func (h *StrategicPulseHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	// 1. Calculate current metrics
	projectMetrics, err := metrics.CalculateProjectMetrics(ctx, h.storage, secCtx, "", true)
	if err != nil {
		return errfmt.Newf("calculate project metrics").Wrap(err)
	}

	// 2. Persist to historical lake (JSON files)
	metricsDir := filepath.Join(h.projectRoot, "docs/reports/metrics")
	if err := fileutil.EnsureDir(metricsDir); err != nil {
		return errfmt.Newf("create metrics dir").Wrap(err)
	}

	timestamp := time.Now().Format("2006-01-02")

	// Save PCS
	pcsData := map[string]any{
		"pcs":       projectMetrics.PCS,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	if err := h.saveMetric(metricsDir, "pcs-"+timestamp+".json", pcsData); err != nil {
		return err
	}

	// Save EDD
	eddData := map[string]any{
		"edd":       projectMetrics.EDD,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	if err := h.saveMetric(metricsDir, "edd-"+timestamp+".json", eddData); err != nil {
		return err
	}

	return nil
}

func (h *StrategicPulseHandler) saveMetric(dir, filename string, data any) error {
	path := filepath.Join(dir, filename)
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal metric %s", filename).Wrap(err)
	}

	return fileutil.WriteSecureFile(path, bytes)
}
