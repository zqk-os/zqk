package storage

import (
	"encoding/json"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// CacheMetricsEncodeMeta summarizes an encoded cache metrics payload for base_metric fields.
type CacheMetricsEncodeMeta struct {
	WindowStart           time.Time
	WindowEnd             time.Time
	AsyncStrategyKinds    int
	KindValidationMetrics int
	TotalValidationsSum   int64
}

// cacheMetricsPersistPayload is the JSON envelope for CAS validation cache metrics.
type cacheMetricsPersistPayload struct {
	Timestamp             time.Time                       `json:"timestamp"`
	AsyncStrategies       map[string]AsyncStrategyStats   `json:"async_strategies"`
	KindValidationMetrics []KindValidationMetricsSnapshot `json:"kind_validation_metrics"`
}

// EncodeCacheMetricsForPersistence JSON-encodes async strategy stats and per-kind validation metrics.
func EncodeCacheMetricsForPersistence() ([]byte, CacheMetricsEncodeMeta, error) {
	now := time.Now().UTC()
	async := GetAsyncStrategyStats()
	kindRows := SnapshotAllKindValidationMetrics()
	var sumVal int64
	for _, k := range kindRows {
		sumVal += k.TotalValidations
	}
	payload := cacheMetricsPersistPayload{
		Timestamp:             now,
		AsyncStrategies:       async,
		KindValidationMetrics: kindRows,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, CacheMetricsEncodeMeta{}, errfmt.Newf(ConstMiscEncodeCacheMetrics).Wrap(err)
	}
	meta := CacheMetricsEncodeMeta{
		WindowStart:           now,
		WindowEnd:             now,
		AsyncStrategyKinds:    len(async),
		KindValidationMetrics: len(kindRows),
		TotalValidationsSum:   sumVal,
	}
	return data, meta, nil
}
