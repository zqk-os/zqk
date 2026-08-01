# Lock Metrics Analysis and Automatic Improvements

## Overview

The system now conducts regular analysis of lock metrics to inform CLI improvements. Metrics are collected during every system check operation and analyzed to provide actionable recommendations.

## Metrics Collected

### Spec Loader Metrics (`pkg/objects/spec_loader_metrics.go`)
- **Wait Times**: Time spent waiting to acquire locks
- **Hold Times**: Time locks are held (indicates operation duration)
- **Contention Events**: Number of times wait time exceeded 50ms threshold
- **Timeout Events**: Number of operation timeouts
- **Contention Rate**: Percentage of operations experiencing contention
- **Timeout Rate**: Percentage of operations timing out
- **Max Wait Time**: Maximum observed wait time
- **Average Wait/Hold Times**: Statistical averages

### Validation Metrics (`pkg/validation/metrics.go`)
- **Lock Wait Times**: Hash registry cache lock wait durations
- **Lock Hold Times**: Hash registry cache lock hold durations
- **Contention Rate**: Calculated from wait times > 50ms
- **Timeout Rate**: Calculated from timeout events

## Analysis and Recommendations

After each system check, metrics are automatically analyzed (`cmd/zqk/system/spec_loader_metrics_analysis.go`):

### Severity Levels
- **Low**: Normal operation, no action needed
- **Medium**: Moderate contention (>10%), monitor closely
- **High**: High contention (>30%) or frequent timeouts (>10%)
- **Critical**: Severe contention (>50%) or extreme timeouts (>20%)

### Recommendations Generated
1. **Timeout Adjustments**: Suggested timeout values based on observed max wait times
2. **Semaphore Capacity**: Suggested semaphore capacity based on contention rates
3. **Lock Optimization**: Recommendations for reducing lock scope or granularity
4. **Performance Warnings**: Alerts when operations are consistently slow

## Automatic Improvements

### Dynamic Timeout Calculation (`pkg/concurrency/timeout_mutex.go`)

Timeouts are now automatically adjusted based on metrics:

- **Contention Rate > 30%**: Timeout increased by 3x
- **Contention Rate > 10%**: Timeout increased by 2x
- **Contention Rate > 5%**: Timeout increased by 1.5x
- **Timeout Rate > 20%**: Timeout increased by 4x
- **Timeout Rate > 10%**: Timeout increased by 2x
- **Timeout Rate > 5%**: Timeout increased by 1.5x
- **Max Wait Time**: Used as baseline if significantly higher than calculated timeout

### Example
If metrics show:
- Contention rate: 35%
- Timeout rate: 15%
- Max wait time: 45s

The timeout calculation will:
1. Start with base timeout (e.g., 60s for spec_loader operations)
2. Multiply by 3 for high contention (60s * 3 = 180s)
3. Multiply by 2 for high timeout rate (180s * 2 = 360s)
4. Clamp to max (120s for spec_loader under extreme contention)
5. Result: 120s timeout (up from default 60s)

## Usage

### Viewing Metrics Analysis

Metrics are automatically analyzed and logged after each system check when `--verbose` is enabled:

```bash
zqk system check --verbose
```

Output includes:
- Metrics summary (wait times, contention rates, timeout rates)
- Severity assessment
- Actionable recommendations
- Suggested timeout and semaphore values

### Example Output

```
Spec loader metrics analysis
  severity: high
  contention_rate: 35.2%
  timeout_rate: 12.5%
  max_wait_time: 45s
  avg_wait_time: 2.3s
  suggested_timeout: 90s
  suggested_semaphore: 32

Spec loader metrics recommendations
  Recommendation 1: High contention rate (>30%) - consider increasing semaphore capacity or reducing concurrent validation goroutines.
  Recommendation 2: High timeout rate (>10%, 1250 timeouts) - consider increasing lock timeout duration.
  Recommendation 3: CRITICAL: Max wait time 45s exceeds 30s - locks are severely contended. Immediate action required.
  Recommendation 4: Suggested semaphore capacity: 32 (increased from 16 due to high contention)
```

## Implementation Details

### LockMetrics Interface Extension

The `LockMetrics` interface (`pkg/concurrency/timeout_mutex.go`) now includes:
- `GetContentionRate() float64` - Returns contention rate (0.0 to 1.0)
- `GetTimeoutRate() float64` - Returns timeout rate (0.0 to 1.0)
- `GetMaxWaitTime() time.Duration` - Returns maximum wait time observed

### Implementations

- **SpecLoaderMetrics** (`pkg/objects/spec_loader_metrics.go`): Full implementation
- **ValidationMetrics** (`pkg/validation/metrics.go`): Full implementation

### Analysis Function

`analyzeSpecLoaderMetrics()` in `cmd/zqk/system/spec_loader_metrics_analysis.go`:
- Analyzes metrics snapshot
- Generates severity assessment
- Calculates suggested timeout and semaphore capacity
- Produces actionable recommendations

## Future Enhancements

1. **Historical Tracking**: Store metrics over time to detect trends
2. **Automatic Semaphore Adjustment**: Dynamically resize semaphore (requires channel recreation)
3. **Proactive Warnings**: Alert before contention reaches critical levels
4. **Metrics Dashboard**: Visual representation of contention patterns
5. **Auto-tuning**: Automatically adjust timeouts/semaphore based on historical patterns

## Best Practices

1. **Monitor Metrics Regularly**: Check metrics analysis after each system check
2. **Act on Recommendations**: Implement suggested timeout/semaphore adjustments
3. **Track Trends**: Watch for increasing contention rates over time
4. **Optimize Locks**: Reduce lock scope when hold times are consistently high
5. **Scale Resources**: Increase semaphore capacity when contention is high
