package monitors

import (
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	emptyValue     = ""
	statusOK       = "ok"
	statusDegraded = "degraded"
	statusFail     = "fail"
)

const (
	summaryNoProjectRoot = "no project root"
	detailsErrorKey      = "error"
)

const (
	kindAuditEvent          = objects.KindAuditEvent
	sixHourWindow           = 6 * time.Hour
	objectVolumeSeriesFmt   = "object_volume/%s"
	streamVolumeSeriesFmt   = "stream_volume/%s"
	objectVolumeChunkWindow = time.Hour
)

const (
	detailsKindKey      = "kind"
	detailsCurrentCount = "current_count"
	detailsRatePerHour  = "rate_per_hour"
	detailsSampleCount  = "samples"
)
