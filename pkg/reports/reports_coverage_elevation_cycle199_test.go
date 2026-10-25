// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package reports

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestTypes_ModelsAndToResult(t *testing.T) {
	t.Parallel()

	meta := ReportMetadata{
		GeneratedAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
		Environment: "staging",
		Version:     "v1.2.3",
		Tags:        map[string]string{"pod": "worker-1"},
	}

	// 1. MaturationReport
	mat := MaturationReport{
		ReportType:     ReportTypeMaturation,
		Status:         StatusSuccess,
		Message:        "Maturation ok",
		ObjectsMatured: 50,
		ObjectsDraft:   10,
		ObjectsPending: 5,
		MaturationRate: 0.77,
		Details:        map[string]any{"note": "lifecycle active"},
	}
	resMat := mat.ToResult(meta)
	if resMat.ReportType != ReportTypeMaturation || resMat.Status != StatusSuccess {
		t.Fatalf("unexpected resMat: %+v", resMat)
	}
	mapMat := resMat.ToMap()
	if mapMat["report_type"] != "maturation" || mapMat[objects.FieldKeyStatus] != StatusSuccess {
		t.Fatalf("unexpected mapMat: %+v", mapMat)
	}

	// Maturation without details
	matNoDetails := MaturationReport{ReportType: ReportTypeMaturation}
	resMatNoDetails := matNoDetails.ToResult(ReportMetadata{GeneratedAt: time.Now().UTC()})
	if _, ok := resMatNoDetails.Data["details"]; ok {
		t.Fatalf("expected no details in data, got %+v", resMatNoDetails.Data)
	}

	// 2. VitalityReport
	vit := VitalityReport{
		ReportType:     ReportTypeVitality,
		Status:         StatusSuccess,
		Message:        "Vitality ok",
		HealthScore:    95,
		MemoryUsageMB:  64.5,
		GoroutineCount: 20,
		CircuitHealth:  "nominal",
		Details:        map[string]any{"region": "us-east"},
	}
	resVit := vit.ToResult(meta)
	if resVit.ReportType != ReportTypeVitality || resVit.Data["health_score"] != 95 {
		t.Fatalf("unexpected resVit: %+v", resVit)
	}
	// Vitality without details
	vitNoDetails := VitalityReport{ReportType: ReportTypeVitality}
	resVitNoDetails := vitNoDetails.ToResult(ReportMetadata{GeneratedAt: time.Now().UTC()})
	if _, ok := resVitNoDetails.Data["details"]; ok {
		t.Fatalf("expected no details, got %+v", resVitNoDetails.Data)
	}

	// 3. QASuccessReport
	qa := QASuccessReport{
		ReportType:   ReportTypeQASuccess,
		Status:       StatusSuccess,
		Message:      "QA ok",
		TestsPassed:  1200,
		TestsFailed:  0,
		TestsSkipped: 10,
		CoveragePct:  95.0,
		PassRatePct:  100.0,
		Details:      map[string]any{"suite": "hygiene"},
	}
	resQA := qa.ToResult(meta)
	if resQA.ReportType != ReportTypeQASuccess || resQA.Data["tests_passed"] != 1200 {
		t.Fatalf("unexpected resQA: %+v", resQA)
	}
	qaNoDetails := QASuccessReport{ReportType: ReportTypeQASuccess}
	resQANoDetails := qaNoDetails.ToResult(ReportMetadata{GeneratedAt: time.Now().UTC()})
	if _, ok := resQANoDetails.Data["details"]; ok {
		t.Fatalf("expected no details, got %+v", resQANoDetails.Data)
	}

	// 4. TelemetrySummary
	ts := TelemetrySummary{
		ReportType:         ReportTypeTelemetrySummary,
		Status:             StatusDegraded,
		Message:            "Some warnings",
		TotalReports:       4,
		HealthyServices:    3,
		DegradedServices:   1,
		FailedServices:     0,
		AverageHealthScore: 88.5,
		ActiveAlerts:       []string{"vitality: degraded"},
		AggregatedAt:       meta.GeneratedAt,
		Details:            map[string]any{"batch": "test-batch"},
	}
	resTS := ts.ToResult(meta)
	if resTS.ReportType != ReportTypeTelemetrySummary || resTS.Data["total_reports"] != 4 {
		t.Fatalf("unexpected resTS: %+v", resTS)
	}
	tsNoDetails := TelemetrySummary{ReportType: ReportTypeTelemetrySummary}
	resTSNoDetails := tsNoDetails.ToResult(ReportMetadata{GeneratedAt: time.Now().UTC()})
	if _, ok := resTSNoDetails.Data["active_alerts"]; ok {
		t.Fatalf("expected no active_alerts, got %+v", resTSNoDetails.Data)
	}
	if _, ok := resTSNoDetails.Data["details"]; ok {
		t.Fatalf("expected no details, got %+v", resTSNoDetails.Data)
	}

	// 5. Empty metadata ToMap
	resEmptyMeta := ReportResult{
		ReportType: ReportTypePCS,
		Status:     StatusSuccess,
		Message:    "Empty meta",
		Timestamp:  time.Now().UTC(),
		Data:       map[string]any{"pcs": 92},
	}
	mapEmptyMeta := resEmptyMeta.ToMap()
	if _, ok := mapEmptyMeta["metadata"]; ok {
		t.Fatalf("expected no metadata map when empty, got %v", mapEmptyMeta)
	}
}

func TestOptions_FunctionalAndParsing(t *testing.T) {
	t.Parallel()

	// Default and ApplyOptions
	opts := ApplyOptions(
		nil,
		WithFormat(FormatTable),
		WithThreshold(85.5),
		WithTimeRange(time.Unix(100, 0), time.Unix(200, 0)),
		WithIncludeDetails(true),
		WithFilterTag("cluster", "prod-1"),
		WithEnvironment("production-east"),
	)
	if opts.Format != FormatTable || opts.Threshold != 85.5 || !opts.IncludeDetails {
		t.Fatalf("unexpected options: %+v", opts)
	}
	if opts.StartTime == nil || opts.EndTime == nil || opts.StartTime.Unix() != 100 || opts.EndTime.Unix() != 200 {
		t.Fatalf("unexpected times: %+v", opts)
	}
	if opts.FilterTags["cluster"] != "prod-1" || opts.Environment != "production-east" {
		t.Fatalf("unexpected tags/env: %+v", opts)
	}

	// WithFilterTag when map is nil
	emptyOpts := ReportOptions{}
	WithFilterTag("k", "v")(&emptyOpts)
	if emptyOpts.FilterTags["k"] != "v" {
		t.Fatalf("expected tag to be set, got %+v", emptyOpts.FilterTags)
	}

	// ParseReportOptions - nil
	parsedNil, err := ParseReportOptions(nil)
	if err != nil || parsedNil.Format != FormatMap {
		t.Fatalf("expected default for nil, got %+v, err: %v", parsedNil, err)
	}

	// ParseReportOptions - valid values
	parsedValid, err := ParseReportOptions(map[string]any{
		"format":          " JSON ",
		"threshold":       "75.5",
		"start_time":      "2026-10-25T10:00:00Z",
		"end_time":        "2026-10-25T11:00:00Z",
		"include_details": "true",
		"environment":     "production",
		"tags":            map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("unexpected error parsing valid options: %v", err)
	}
	if parsedValid.Format != FormatJSON || parsedValid.Threshold != 75.5 || !parsedValid.IncludeDetails {
		t.Fatalf("unexpected parsed values: %+v", parsedValid)
	}

	// Parse alternate format: empty string, table, text, map
	for _, fmtStr := range []string{"", "table", "text", "map"} {
		p, err := ParseReportOptions(map[string]any{"format": fmtStr})
		if err != nil {
			t.Fatalf("format %q failed: %v", fmtStr, err)
		}
		if fmtStr != "" && string(p.Format) != fmtStr {
			t.Fatalf("expected format %s, got %s", fmtStr, p.Format)
		}
	}

	// Time options variations: RFC3339Nano, int64, int, time.Time
	now := time.Now().UTC()
	parsedTimes, err := ParseReportOptions(map[string]any{
		"start_time": now.Format(time.RFC3339Nano),
		"end_time":   now.Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("unexpected time parse err: %v", err)
	}
	if parsedTimes.StartTime == nil || parsedTimes.EndTime == nil {
		t.Fatalf("expected non-nil start and end time")
	}

	// Timestamp types: int, time.Time
	pIntTime, err := ParseReportOptions(map[string]any{"start_time": int(1700000000)})
	if err != nil || pIntTime.StartTime == nil {
		t.Fatalf("failed parsing int timestamp: %v", err)
	}
	pStructTime, err := ParseReportOptions(map[string]any{"start_time": now})
	if err != nil || pStructTime.StartTime == nil {
		t.Fatalf("failed parsing time.Time: %v", err)
	}

	// Details as bool, string ("1")
	pBool, err := ParseReportOptions(map[string]any{"include_details": true})
	if err != nil || !pBool.IncludeDetails {
		t.Fatalf("expected include_details true")
	}
	pStrOne, err := ParseReportOptions(map[string]any{"include_details": "1"})
	if err != nil || !pStrOne.IncludeDetails {
		t.Fatalf("expected include_details true for '1'")
	}

	// Tags as map[string]any and "filter_tags" key
	pAnyTags, err := ParseReportOptions(map[string]any{
		"filter_tags": map[string]any{"node": 42},
	})
	if err != nil || pAnyTags.FilterTags["node"] != "42" {
		t.Fatalf("expected filter_tags to be parsed, got %+v", pAnyTags.FilterTags)
	}

	// Numeric threshold types: float32, int, int64
	for _, numVal := range []any{float32(50.5), int(60), int64(70)} {
		pNum, err := ParseReportOptions(map[string]any{"threshold": numVal})
		if err != nil || pNum.Threshold == 0 {
			t.Fatalf("failed to parse threshold for %T: %v", numVal, err)
		}
	}
}

func TestOptions_ParseErrors(t *testing.T) {
	t.Parallel()

	// Invalid format type
	if _, err := ParseReportOptions(map[string]any{"format": 12345}); err == nil {
		t.Fatal("expected error for non-string format")
	}

	// Unsupported format string
	if _, err := ParseReportOptions(map[string]any{"format": "xml"}); err == nil {
		t.Fatal("expected error for unsupported format xml")
	}

	// Invalid threshold: negative
	if _, err := ParseReportOptions(map[string]any{"threshold": -5.0}); err == nil {
		t.Fatal("expected error for negative threshold")
	}

	// Invalid threshold: non-numeric string
	if _, err := ParseReportOptions(map[string]any{"threshold": "abc"}); err == nil {
		t.Fatal("expected error for non-numeric threshold string")
	}

	// Invalid threshold: invalid type
	if _, err := ParseReportOptions(map[string]any{"threshold": []string{"invalid"}}); err == nil {
		t.Fatal("expected error for slice threshold")
	}

	// Invalid start_time: unparseable string
	if _, err := ParseReportOptions(map[string]any{"start_time": "not-a-date"}); err == nil {
		t.Fatal("expected error for bad start_time")
	}

	// Invalid start_time: unsupported type
	if _, err := ParseReportOptions(map[string]any{"start_time": true}); err == nil {
		t.Fatal("expected error for bool start_time")
	}

	// Invalid end_time: unparseable string
	if _, err := ParseReportOptions(map[string]any{"end_time": "invalid"}); err == nil {
		t.Fatal("expected error for bad end_time")
	}

	// End time before start time
	start := "2026-10-25T12:00:00Z"
	end := "2026-10-25T11:00:00Z"
	if _, err := ParseReportOptions(map[string]any{"start_time": start, "end_time": end}); err == nil {
		t.Fatal("expected error when end_time is before start_time")
	}
}

func TestFormatters(t *testing.T) {
	t.Parallel()

	res := ReportResult{
		ReportType: ReportTypeVitality,
		Status:     StatusSuccess,
		Message:    "All systems nominal",
		Timestamp:  time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
		Data: map[string]any{
			"health_score":    99,
			"memory_usage_mb": 42.1,
		},
		Metadata: ReportMetadata{
			Environment: "production",
		},
	}

	// FormatJSON
	jsonStr, err := FormatResultJSON(res, true)
	if err != nil || !strings.Contains(jsonStr, `"health_score": 99`) {
		t.Fatalf("unexpected FormatResultJSON: %v, str: %s", err, jsonStr)
	}
	jsonCompact, err := FormatResultJSON(res, false)
	if err != nil || !strings.Contains(jsonCompact, `"status":"success"`) {
		t.Fatalf("unexpected FormatResultJSON compact: %v", err)
	}

	// FormatTable
	tableStr, err := FormatResultTable(res)
	if err != nil || !strings.Contains(tableStr, "=== REPORT: VITALITY ===") {
		t.Fatalf("unexpected table: %v, str: %s", err, tableStr)
	}

	// FormatText
	textStr := FormatResultText(res)
	if !strings.Contains(textStr, "[SUCCESS] vitality: All systems nominal") {
		t.Fatalf("unexpected text: %s", textStr)
	}

	// FormatText with empty data
	emptyDataRes := ReportResult{
		ReportType: ReportTypeMaturation,
		Status:     StatusDegraded,
		Message:    "Degraded state",
	}
	emptyText := FormatResultText(emptyDataRes)
	if emptyText != "[DEGRADED] maturation: Degraded state" {
		t.Fatalf("unexpected empty data text: %s", emptyText)
	}

	// FormatReport routing
	formattedJSON, err := FormatReport(FormatJSON, res)
	if err != nil || !strings.Contains(formattedJSON.(string), `"report_type"`) {
		t.Fatalf("FormatReport json error: %v", err)
	}

	formattedTable, err := FormatReport(FormatTable, res)
	if err != nil || !strings.Contains(formattedTable.(string), "=== REPORT:") {
		t.Fatalf("FormatReport table error: %v", err)
	}

	formattedText, err := FormatReport(FormatText, res)
	if err != nil || !strings.Contains(formattedText.(string), "[SUCCESS]") {
		t.Fatalf("FormatReport text error: %v", err)
	}

	formattedMap, err := FormatReport(FormatMap, res)
	if err != nil || formattedMap.(map[string]any)["report_type"] != "vitality" {
		t.Fatalf("FormatReport map error: %v", err)
	}

	// Unsupported format
	if _, err := FormatReport("invalid_format", res); err == nil {
		t.Fatal("expected error for unsupported format")
	}

	// Unmarshalable JSON error
	unmarshalableRes := ReportResult{
		ReportType: ReportTypeVitality,
		Data:       map[string]any{"unsupported": make(chan int)},
	}
	if _, err := FormatResultJSON(unmarshalableRes, true); err == nil {
		t.Fatal("expected error on indented json marshal with channel")
	}
	if _, err := FormatResultJSON(unmarshalableRes, false); err == nil {
		t.Fatal("expected error on compact json marshal with channel")
	}
}

func TestGenerateMaturationReport_OptionsAndThresholds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 1. Success with custom args and JSON format
	resJSON, err := GenerateMaturationReport(ctx, map[string]any{
		"format":          "json",
		"objects_matured": 80,
		"objects_draft":   15,
		"objects_pending": 5,
		"threshold":       0.70,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jsonStr, ok := resJSON.(string)
	if !ok || !strings.Contains(jsonStr, `"objects_matured": 80`) {
		t.Fatalf("unexpected JSON: %v", resJSON)
	}

	// 2. Degraded when below threshold
	resDegraded, err := GenerateMaturationReport(ctx, map[string]any{
		"objects_matured": 10,
		"objects_draft":   80,
		"objects_pending": 10,
		"threshold":       0.50,
	})
	if err != nil {
		t.Fatal(err)
	}
	mDegraded, ok := resDegraded.(map[string]any)
	if !ok || mDegraded[objects.FieldKeyStatus] != StatusDegraded {
		t.Fatalf("expected degraded status, got %+v", resDegraded)
	}

	// 3. Zero objects edge case
	resZero, err := GenerateMaturationReport(ctx, map[string]any{
		"objects_matured": 0,
		"objects_draft":   0,
		"objects_pending": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	mZero, ok := resZero.(map[string]any)
	if !ok || mZero[objects.FieldKeyStatus] != StatusSuccess {
		t.Fatalf("expected success for zero total: %+v", resZero)
	}

	// 4. Cancelled context
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := GenerateMaturationReport(cancCtx, nil); err == nil {
		t.Fatal("expected error on cancelled context")
	}

	// 5. Invalid options error
	if _, err := GenerateMaturationReport(ctx, map[string]any{"format": "invalid"}); err == nil {
		t.Fatal("expected error on invalid format")
	}
}

func TestGenerateVitalityReport_OptionsAndThresholds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 1. Success with custom parameters and table format
	resTable, err := GenerateVitalityReport(ctx, map[string]any{
		"format":          "table",
		"health_score":    95,
		"memory_usage_mb": 128.5,
		"goroutine_count": 32,
		"circuit_health":  "nominal",
		"threshold":       90.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tableStr, ok := resTable.(string)
	if !ok || !strings.Contains(tableStr, "health_score") {
		t.Fatalf("unexpected table: %v", resTable)
	}

	// 2. Degraded when below threshold
	resDegraded, err := GenerateVitalityReport(ctx, map[string]any{
		"health_score": 75,
		"threshold":    80.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	mDegraded, ok := resDegraded.(map[string]any)
	if !ok || mDegraded[objects.FieldKeyStatus] != StatusDegraded {
		t.Fatalf("expected degraded status, got %+v", resDegraded)
	}

	// 3. Cancelled context
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := GenerateVitalityReport(cancCtx, nil); err == nil {
		t.Fatal("expected error on cancelled context")
	}

	// 4. Invalid options error
	if _, err := GenerateVitalityReport(ctx, map[string]any{"threshold": -1.0}); err == nil {
		t.Fatal("expected error on negative threshold")
	}
}

func TestGenerateQASuccessReport_OptionsAndThresholds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 1. Success with text format
	resText, err := GenerateQASuccessReport(ctx, map[string]any{
		"format":        "text",
		"tests_passed":  2000,
		"tests_failed":  0,
		"tests_skipped": 20,
		"coverage_pct":  98.2,
		"threshold":     90.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	txt, ok := resText.(string)
	if !ok || !strings.Contains(txt, "[SUCCESS]") {
		t.Fatalf("unexpected text output: %v", resText)
	}

	// 2. Failed tests -> StatusFailed
	resFailed, err := GenerateQASuccessReport(ctx, map[string]any{
		"tests_passed": 100,
		"tests_failed": 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	mFailed, ok := resFailed.(map[string]any)
	if !ok || mFailed[objects.FieldKeyStatus] != StatusFailed {
		t.Fatalf("expected StatusFailed, got %+v", resFailed)
	}

	// 3. Degraded coverage threshold
	resDegraded, err := GenerateQASuccessReport(ctx, map[string]any{
		"tests_passed": 100,
		"tests_failed": 0,
		"coverage_pct": 75.0,
		"threshold":    85.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	mDegraded, ok := resDegraded.(map[string]any)
	if !ok || mDegraded[objects.FieldKeyStatus] != StatusDegraded {
		t.Fatalf("expected StatusDegraded for low coverage, got %+v", resDegraded)
	}

	// 4. Cancelled context
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := GenerateQASuccessReport(cancCtx, nil); err == nil {
		t.Fatal("expected error on cancelled context")
	}

	// 5. Invalid options error
	if _, err := GenerateQASuccessReport(ctx, map[string]any{"format": "invalid_fmt"}); err == nil {
		t.Fatal("expected error on invalid format")
	}
}

func TestGenerateTelemetrySummary_AggregationAndStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 1. Success aggregation with multiple report types
	res1 := ReportResult{
		ReportType: ReportTypeVitality,
		Status:     StatusSuccess,
		Data:       map[string]any{"health_score": 95},
	}
	res2 := ReportResult{
		ReportType: ReportTypeVitality,
		Status:     StatusSuccess,
		Data:       map[string]any{"health_score": 85},
	}
	res3 := ReportResult{
		ReportType: ReportTypeMaturation,
		Status:     StatusSuccess,
		Data:       map[string]any{"objects_matured": 40},
	}

	sum, err := GenerateTelemetrySummary(ctx, []ReportResult{res1, res2, res3}, WithFormat(FormatMap))
	if err != nil {
		t.Fatalf("unexpected summary err: %v", err)
	}
	sumMap, ok := sum.(map[string]any)
	if !ok || sumMap[objects.FieldKeyStatus] != StatusSuccess {
		t.Fatalf("expected success summary, got %+v", sum)
	}
	dataMap := sumMap["data"].(map[string]any)
	if dataMap["total_reports"] != 3 || dataMap["healthy_services"] != 3 || dataMap["average_health_score"] != 90.0 {
		t.Fatalf("unexpected summary data: %+v", dataMap)
	}

	// 2. Degraded when containing degraded items or below threshold
	resDegraded := ReportResult{
		ReportType: ReportTypeVitality,
		Status:     StatusDegraded,
		Data:       map[string]any{"health_score": 70},
	}
	sumDegraded, err := GenerateTelemetrySummary(ctx, []ReportResult{res1, resDegraded})
	if err != nil {
		t.Fatal(err)
	}
	sDegradedMap := sumDegraded.(map[string]any)
	if sDegradedMap[objects.FieldKeyStatus] != StatusDegraded {
		t.Fatalf("expected degraded status, got %+v", sumDegraded)
	}

	// Degraded due to threshold option
	sumThresholdDegraded, err := GenerateTelemetrySummary(ctx, []ReportResult{res1}, WithThreshold(98.0))
	if err != nil {
		t.Fatal(err)
	}
	sThreshMap := sumThresholdDegraded.(map[string]any)
	if sThreshMap[objects.FieldKeyStatus] != StatusDegraded {
		t.Fatalf("expected degraded status due to threshold, got %+v", sumThresholdDegraded)
	}

	// 3. Failed when containing failed items
	resFailed := ReportResult{
		ReportType: ReportTypeQASuccess,
		Status:     StatusFailed,
		Data:       map[string]any{"tests_failed": 2},
	}
	sumFailed, err := GenerateTelemetrySummary(ctx, []ReportResult{res1, resFailed})
	if err != nil {
		t.Fatal(err)
	}
	sFailedMap := sumFailed.(map[string]any)
	if sFailedMap[objects.FieldKeyStatus] != StatusFailed {
		t.Fatalf("expected failed status, got %+v", sumFailed)
	}

	// 4. Cancelled context
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := GenerateTelemetrySummary(cancCtx, nil); err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestBuildReportResult(t *testing.T) {
	t.Parallel()

	// 1. Success with custom fields
	res, err := BuildReportResult(
		ReportTypePCS,
		StatusSuccess,
		"PCS calculated",
		map[string]any{"score": 94},
		WithEnvironment("production"),
		WithFilterTag("source", "cli"),
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.ReportType != ReportTypePCS || res.Status != StatusSuccess || res.Metadata.Environment != "production" {
		t.Fatalf("unexpected res: %+v", res)
	}

	// 2. Defaults applied when empty status/message
	resDefaults, err := BuildReportResult(ReportTypeEDD, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resDefaults.Status != StatusSuccess || resDefaults.Message != "Report generated" {
		t.Fatalf("unexpected defaults: %+v", resDefaults)
	}

	// 3. Error on empty reportType
	if _, err := BuildReportResult("", StatusSuccess, "msg", nil); err == nil {
		t.Fatal("expected error for empty reportType")
	}
}

func TestConcurrentStress_ThreadSafety(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const workers = 16
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("reports_stress_test", fmt.Sprintf("worker-%d", workerID))
		builder.StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Maturation
				mat, err := GenerateMaturationReport(ctx, map[string]any{
					"objects_matured": workerID + j,
					"objects_draft":   5,
					"format":          "json",
				})
				if err != nil || mat == nil {
					panic(fmt.Sprintf("worker %d iteration %d Maturation failed: %v", workerID, j, err))
				}

				// Vitality
				vit, err := GenerateVitalityReport(ctx, map[string]any{
					"health_score": 90 + (j % 10),
					"format":       "table",
				})
				if err != nil || vit == nil {
					panic(fmt.Sprintf("worker %d iteration %d Vitality failed: %v", workerID, j, err))
				}

				// QA
				qa, err := GenerateQASuccessReport(ctx, map[string]any{
					"tests_passed": 1000 + j,
					"format":       "text",
				})
				if err != nil || qa == nil {
					panic(fmt.Sprintf("worker %d iteration %d QA failed: %v", workerID, j, err))
				}

				// TelemetrySummary
				r1, _ := BuildReportResult(ReportTypeVitality, StatusSuccess, "ok", map[string]any{"health_score": 95})
				r2, _ := BuildReportResult(ReportTypeMaturation, StatusSuccess, "ok", map[string]any{"objects_matured": 40})
				sum, err := GenerateTelemetrySummary(ctx, []ReportResult{r1, r2}, WithFormat(FormatMap))
				if err != nil || sum == nil {
					panic(fmt.Sprintf("worker %d iteration %d Summary failed: %v", workerID, j, err))
				}
			}
		})
	}

	wg.Wait()
}
