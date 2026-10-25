// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package reports

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// FormatResultJSON serializes a ReportResult to a JSON string.
func FormatResultJSON(res ReportResult, indent bool) (string, error) {
	var bytes []byte
	var err error
	if indent {
		bytes, err = json.MarshalIndent(res, "", "  ")
	} else {
		bytes, err = json.Marshal(res)
	}
	if err != nil {
		return "", fmt.Errorf("failed to marshal report result to json: %w", err)
	}
	return string(bytes), nil
}

// FormatResultTable renders a ReportResult as a structured tabular text string.
func FormatResultTable(res ReportResult) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== REPORT: %s ===\n", strings.ToUpper(string(res.ReportType))))
	sb.WriteString(fmt.Sprintf("Status:    %s\n", res.Status))
	sb.WriteString(fmt.Sprintf("Message:   %s\n", res.Message))
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", res.Timestamp.Format(time.RFC3339)))
	if res.Metadata.Environment != "" {
		sb.WriteString(fmt.Sprintf("Env:       %s\n", res.Metadata.Environment))
	}
	sb.WriteString("--- METRICS ---\n")

	keys := make([]string, 0, len(res.Data))
	for k := range res.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%-22s: %v\n", k, res.Data[k]))
	}

	return sb.String(), nil
}

// FormatResultMap returns a map projection of the ReportResult envelope.
func FormatResultMap(res ReportResult) map[string]any {
	return res.ToMap()
}

// FormatResultText renders a concise human-readable text representation of the report.
func FormatResultText(res ReportResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] %s: %s", strings.ToUpper(res.Status), res.ReportType, res.Message))
	if len(res.Data) > 0 {
		keys := make([]string, 0, len(res.Data))
		for k := range res.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteString(" (")
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, res.Data[k]))
		}
		sb.WriteString(strings.Join(parts, ", "))
		sb.WriteString(")")
	}
	return sb.String()
}

// FormatReport routes a ReportResult to the desired format presentation.
func FormatReport(format ReportFormat, res ReportResult) (any, error) {
	switch format {
	case FormatJSON:
		return FormatResultJSON(res, true)
	case FormatTable:
		return FormatResultTable(res)
	case FormatText:
		return FormatResultText(res), nil
	case FormatMap, "":
		return FormatResultMap(res), nil
	default:
		return nil, fmt.Errorf("unsupported report format %q", format)
	}
}
