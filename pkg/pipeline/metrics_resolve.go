package pipeline

import (
	"strings"
	"unicode"
)

// StageMetricsResolve is the reserved first stage when [MetricsConfig] is active. It validates
// the bucketing strategy and normalizes label rules once per [Pipeline.Run]; it does not emit
// per-stage metrics (see [Pipeline.Run]).
const StageMetricsResolve = "METRICS_RESOLVE"

// Metrics label normalization bounds (cardinality / size guards for sinks and backends).
const (
	maxMetricBucketKeys = 32
	maxMetricKeyRunes   = 64
	maxMetricValueRunes = 256
	emptyValue          = ""
)

// NormalizeBucketLabels trims keys and values, drops empty values, enforces max key/value lengths
// and max key count, and sanitizes keys to a safe label character set.
func NormalizeBucketLabels(buckets map[string]string) map[string]string {
	if len(buckets) == 0 {
		return nil
	}
	out := make(map[string]string, len(buckets))
	n := 0
	for k, v := range buckets {
		if n >= maxMetricBucketKeys {
			break
		}
		k = strings.TrimSpace(k)
		k = sanitizeMetricKey(k)
		if k == emptyValue {
			continue
		}
		k = truncateMetricRunes(k, maxMetricKeyRunes)
		v = strings.TrimSpace(v)
		if v == emptyValue {
			continue
		}
		v = truncateMetricRunes(v, maxMetricValueRunes)
		out[k] = v
		n++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeMetricKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '_' || r == '.' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_-.")
}

func truncateMetricRunes(s string, max int) string {
	if max <= 0 {
		return emptyValue
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func metricsResolvePreflight(pipelineKind string, cfg *MetricsConfig) StageFunc {
	return func(pctx *Context, payload any) (any, error) {
		if metricsConfigStrategyMissing(cfg) {
			return payload, nil
		}
		raw := cfg.Strategy.Buckets(pctx, pipelineKind, StageMetricsResolve)
		_ = NormalizeBucketLabels(raw)
		if pctx != nil {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[OutcomeKeyMetricsResolve] = true
		}
		return payload, nil
	}
}

func prependMetricsResolveStage(stages []namedStage, pipelineKind string, cfg *MetricsConfig) []namedStage {
	if cfg == nil {
		return stages
	}
	pre := namedStage{
		name: StageMetricsResolve,
		fn:   metricsResolvePreflight(pipelineKind, cfg),
	}
	out := make([]namedStage, 0, len(stages)+1)
	out = append(out, pre)
	out = append(out, stages...)
	return out
}
