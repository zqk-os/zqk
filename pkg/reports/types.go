// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package reports

import (
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ReportType identifies the category of diagnostic or vitality report.
type ReportType string

const (
	// ReportTypeMaturation represents the object lifecycle maturation report.
	ReportTypeMaturation ReportType = "maturation"
	// ReportTypeVitality represents system health, memory, and runtime vitality.
	ReportTypeVitality ReportType = "vitality"
	// ReportTypeQASuccess represents QA and test execution success telemetry.
	ReportTypeQASuccess ReportType = "qa_success"
	// ReportTypePCS represents Project Confidence Score.
	ReportTypePCS ReportType = "pcs"
	// ReportTypeEDD represents Effort Distribution Discrepancy.
	ReportTypeEDD ReportType = "edd"
	// ReportTypeBlockers represents Dependencies & Blockers analysis.
	ReportTypeBlockers ReportType = "blockers"
	// ReportTypeTelemetrySummary represents an aggregated cross-system summary.
	ReportTypeTelemetrySummary ReportType = "telemetry_summary"
)

// ReportFormat defines the output presentation format.
type ReportFormat string

const (
	// FormatJSON renders report as structured JSON.
	FormatJSON ReportFormat = "json"
	// FormatTable renders report as tabular text.
	FormatTable ReportFormat = "table"
	// FormatMap renders report as a generic Go map[string]any.
	FormatMap ReportFormat = "map"
	// FormatText renders report as human-readable key-value text.
	FormatText ReportFormat = "text"
)

// Standard report status constants.
const (
	StatusSuccess  = "success"
	StatusDegraded = "degraded"
	StatusFailed   = "failed"
)

// ReportMetadata captures runtime contextual data for report provenance.
type ReportMetadata struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Environment string            `json:"environment,omitempty"`
	Version     string            `json:"version,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// ReportResult represents a unified, standardized diagnostic report payload.
type ReportResult struct {
	ReportType ReportType     `json:"report_type"`
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	Timestamp  time.Time      `json:"timestamp"`
	Data       map[string]any `json:"data"`
	Metadata   ReportMetadata `json:"metadata,omitempty"`
}

// ToMap converts ReportResult into a canonical envelope map.
func (r ReportResult) ToMap() map[string]any {
	m := map[string]any{
		"report_type":          string(r.ReportType),
		objects.FieldKeyStatus: r.Status,
		"message":              r.Message,
		"timestamp":            r.Timestamp.Format(time.RFC3339),
		"data":                 r.Data,
	}
	if r.Metadata.Environment != "" || len(r.Metadata.Tags) > 0 || r.Metadata.Version != "" {
		m["metadata"] = map[string]any{
			"generated_at": r.Metadata.GeneratedAt.Format(time.RFC3339),
			"environment":  r.Metadata.Environment,
			"version":      r.Metadata.Version,
			"tags":         r.Metadata.Tags,
		}
	}
	return m
}

// MaturationReport models object lifecycle progression.
type MaturationReport struct {
	ReportType     ReportType     `json:"report_type"`
	Status         string         `json:"status"`
	Message        string         `json:"message"`
	ObjectsMatured int            `json:"objects_matured"`
	ObjectsDraft   int            `json:"objects_draft"`
	ObjectsPending int            `json:"objects_pending"`
	MaturationRate float64        `json:"maturation_rate"`
	Details        map[string]any `json:"details,omitempty"`
}

// ToResult converts MaturationReport into a unified ReportResult.
func (m MaturationReport) ToResult(meta ReportMetadata) ReportResult {
	data := map[string]any{
		"objects_matured": m.ObjectsMatured,
		"objects_draft":   m.ObjectsDraft,
		"objects_pending": m.ObjectsPending,
		"maturation_rate": m.MaturationRate,
	}
	if len(m.Details) > 0 {
		data["details"] = m.Details
	}
	return ReportResult{
		ReportType: m.ReportType,
		Status:     m.Status,
		Message:    m.Message,
		Timestamp:  meta.GeneratedAt,
		Data:       data,
		Metadata:   meta,
	}
}

// VitalityReport models runtime system vitality and health scores.
type VitalityReport struct {
	ReportType     ReportType     `json:"report_type"`
	Status         string         `json:"status"`
	Message        string         `json:"message"`
	HealthScore    int            `json:"health_score"`
	MemoryUsageMB  float64        `json:"memory_usage_mb"`
	GoroutineCount int            `json:"goroutine_count"`
	CircuitHealth  string         `json:"circuit_health"`
	Details        map[string]any `json:"details,omitempty"`
}

// ToResult converts VitalityReport into a unified ReportResult.
func (v VitalityReport) ToResult(meta ReportMetadata) ReportResult {
	data := map[string]any{
		"health_score":    v.HealthScore,
		"memory_usage_mb": v.MemoryUsageMB,
		"goroutine_count": v.GoroutineCount,
		"circuit_health":  v.CircuitHealth,
	}
	if len(v.Details) > 0 {
		data["details"] = v.Details
	}
	return ReportResult{
		ReportType: v.ReportType,
		Status:     v.Status,
		Message:    v.Message,
		Timestamp:  meta.GeneratedAt,
		Data:       data,
		Metadata:   meta,
	}
}

// QASuccessReport models automated test execution metrics and pass rates.
type QASuccessReport struct {
	ReportType   ReportType     `json:"report_type"`
	Status       string         `json:"status"`
	Message      string         `json:"message"`
	TestsPassed  int            `json:"tests_passed"`
	TestsFailed  int            `json:"tests_failed"`
	TestsSkipped int            `json:"tests_skipped"`
	CoveragePct  float64        `json:"coverage_pct"`
	PassRatePct  float64        `json:"pass_rate_pct"`
	Details      map[string]any `json:"details,omitempty"`
}

// ToResult converts QASuccessReport into a unified ReportResult.
func (q QASuccessReport) ToResult(meta ReportMetadata) ReportResult {
	data := map[string]any{
		"tests_passed":  q.TestsPassed,
		"tests_failed":  q.TestsFailed,
		"tests_skipped": q.TestsSkipped,
		"coverage_pct":  q.CoveragePct,
		"pass_rate_pct": q.PassRatePct,
	}
	if len(q.Details) > 0 {
		data["details"] = q.Details
	}
	return ReportResult{
		ReportType: q.ReportType,
		Status:     q.Status,
		Message:    q.Message,
		Timestamp:  meta.GeneratedAt,
		Data:       data,
		Metadata:   meta,
	}
}

// TelemetrySummary aggregates multiple diagnostic metrics into a unified summary.
type TelemetrySummary struct {
	ReportType         ReportType     `json:"report_type"`
	Status             string         `json:"status"`
	Message            string         `json:"message"`
	TotalReports       int            `json:"total_reports"`
	HealthyServices    int            `json:"healthy_services"`
	DegradedServices   int            `json:"degraded_services"`
	FailedServices     int            `json:"failed_services"`
	AverageHealthScore float64        `json:"average_health_score"`
	ActiveAlerts       []string       `json:"active_alerts,omitempty"`
	AggregatedAt       time.Time      `json:"aggregated_at"`
	Details            map[string]any `json:"details,omitempty"`
}

// ToResult converts TelemetrySummary into a unified ReportResult.
func (t TelemetrySummary) ToResult(meta ReportMetadata) ReportResult {
	data := map[string]any{
		"total_reports":        t.TotalReports,
		"healthy_services":     t.HealthyServices,
		"degraded_services":    t.DegradedServices,
		"failed_services":      t.FailedServices,
		"average_health_score": t.AverageHealthScore,
		"aggregated_at":        t.AggregatedAt.Format(time.RFC3339),
	}
	if len(t.ActiveAlerts) > 0 {
		data["active_alerts"] = t.ActiveAlerts
	}
	if len(t.Details) > 0 {
		data["details"] = t.Details
	}
	return ReportResult{
		ReportType: t.ReportType,
		Status:     t.Status,
		Message:    t.Message,
		Timestamp:  meta.GeneratedAt,
		Data:       data,
		Metadata:   meta,
	}
}
