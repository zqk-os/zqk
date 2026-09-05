# Baseline Testing Results

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Complete

## Baseline Collection Summary

### Execution Details
- **Timestamp**: 2026-01-01T22:25:15-08:00
- **Command**: `zqk system check-baseline --baseline-output (PRUNED) .zqk/baseline.json`
- **Project Root**: Current project

### Performance Metrics

| Metric | Value |
|--------|-------|
| **Total Objects** | 3,861 |
| **Total Results** | 3,870 |
| **Duration** | 2.95 seconds |
| **Objects/Second** | 1,307.62 |
| **Total Issues** | 27 |

### Issues Breakdown

#### By Tier
- **Tier 1 (Blocking)**: 0
- **Tier 2 (Warning)**: 27
- **Tier 3 (Informational)**: 0
- **Tier 4 (Recommendation)**: 0

#### By Category
- **Integrity**: 27

### Objects by Kind

| Kind | Count |
|------|-------|
| change_journal_entry | 2,729 |
| doc_entry | 218 |
| backlog_item | 337 |
| criteria | 242 |
| milestone | 44 |
| priority_plan | 43 |
| requirement | 47 |
| audit_event | 20 |
| audit_aggregation_metric | 22 |
| account | 20 |
| test_case | 21 |
| workstream | 21 |
| scheduler_job | 11 |
| role | 10 |
| question | 9 |
| decision | 15 |
| policy | 26 |
| roadmap | 3 |
| goal | 17 |
| strategic_plan | 1 |
| mission | 1 |
| vision | 1 |
| agent_architecture | 1 |
| workstream_transition | 5 |
| agent_onboarding_preparation | 3 |
| evolution_management | 1 |
| kind_synonym | 2 |
| **Total** | **3,861** |

## Analysis

### Performance
- **Throughput**: 1,307.62 objects/second is excellent for synchronous validation
- **Duration**: 2.95 seconds for 3,861 objects is reasonable
- **Scalability**: System handles large object counts efficiently

### Issues
- **27 Tier 2 (Warning) issues** - All integrity-related
- **No blocking issues** - System is healthy
- **Low issue rate**: 0.7% of objects have issues

### Next Steps

1. **Async Validator Comparison**: Run async validator on same data
2. **Performance Comparison**: Compare async vs sync metrics
3. **Issue Analysis**: Investigate the 27 integrity issues
4. **Optimization**: Identify opportunities for improvement

## Comparison Targets

When running async validator, we expect:
- **Same or better results**: All 3,861 objects validated
- **Same or fewer issues**: 27 or fewer issues found
- **Faster execution**: Should complete in less than 2.95 seconds
- **Better throughput**: Should exceed 1,307.62 objects/second

## Baseline File

Full baseline metrics saved to: `.zqk/baseline.json`

This file contains:
- Complete metrics
- Timestamp
- Duration in milliseconds
- Full object breakdown
- Issue categorization
- (Optional) Full check results if `--include-results` flag used

