// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package reports

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ReportOptions defines configuration and filtering options for report generation.
type ReportOptions struct {
	Format         ReportFormat
	Threshold      float64
	StartTime      *time.Time
	EndTime        *time.Time
	IncludeDetails bool
	FilterTags     map[string]string
	Environment    string
}

// Option configures ReportOptions.
type Option func(*ReportOptions)

// WithFormat configures the report output format.
func WithFormat(f ReportFormat) Option {
	return func(o *ReportOptions) {
		o.Format = f
	}
}

// WithThreshold sets a numeric threshold for filtering or status evaluation.
func WithThreshold(t float64) Option {
	return func(o *ReportOptions) {
		o.Threshold = t
	}
}

// WithTimeRange defines the lower and upper temporal boundaries for report data.
func WithTimeRange(start, end time.Time) Option {
	return func(o *ReportOptions) {
		startCopy := start
		endCopy := end
		o.StartTime = &startCopy
		o.EndTime = &endCopy
	}
}

// WithIncludeDetails specifies whether detailed breakdowns are included.
func WithIncludeDetails(include bool) Option {
	return func(o *ReportOptions) {
		o.IncludeDetails = include
	}
}

// WithFilterTag adds a key-value tag filter to the options.
func WithFilterTag(key, value string) Option {
	return func(o *ReportOptions) {
		if o.FilterTags == nil {
			o.FilterTags = make(map[string]string)
		}
		o.FilterTags[key] = value
	}
}

// WithEnvironment sets the target environment (e.g. "production", "staging").
func WithEnvironment(env string) Option {
	return func(o *ReportOptions) {
		o.Environment = env
	}
}

// DefaultReportOptions returns a ReportOptions struct populated with standard defaults.
func DefaultReportOptions() ReportOptions {
	return ReportOptions{
		Format:         FormatMap,
		Threshold:      0.0,
		IncludeDetails: false,
		FilterTags:     make(map[string]string),
		Environment:    "production",
	}
}

// ApplyOptions constructs a ReportOptions struct by applying functional options over defaults.
func ApplyOptions(opts ...Option) ReportOptions {
	options := DefaultReportOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

// ParseReportOptions parses and validates raw map arguments into ReportOptions.
func ParseReportOptions(args map[string]any) (ReportOptions, error) {
	opts := DefaultReportOptions()
	if args == nil {
		return opts, nil
	}

	if err := parseFormatOption(args, &opts); err != nil {
		return opts, err
	}
	if err := parseThresholdOption(args, &opts); err != nil {
		return opts, err
	}
	if err := parseTimeRangeOptions(args, &opts); err != nil {
		return opts, err
	}
	parseDetailsAndEnvOptions(args, &opts)
	parseFilterTagsOption(args, &opts)

	return opts, nil
}

func parseFormatOption(args map[string]any, opts *ReportOptions) error {
	rawFormat, ok := args[objects.FieldKeyFormat]
	if !ok || rawFormat == nil {
		return nil
	}
	s, ok := rawFormat.(string)
	if !ok {
		return fmt.Errorf("invalid format type %T: expected string", rawFormat)
	}
	clean := strings.ToLower(strings.TrimSpace(s))
	if clean == "" {
		return nil
	}
	switch ReportFormat(clean) {
	case FormatJSON, FormatTable, FormatMap, FormatText:
		opts.Format = ReportFormat(clean)
		return nil
	default:
		return fmt.Errorf("unsupported report format %q: expected json, table, map, or text", s)
	}
}

func parseThresholdOption(args map[string]any, opts *ReportOptions) error {
	raw, ok := args["threshold"]
	if !ok || raw == nil {
		return nil
	}
	val, err := toFloat64(raw)
	if err != nil {
		return fmt.Errorf("invalid threshold value %v: %w", raw, err)
	}
	if val < 0 {
		return fmt.Errorf("threshold cannot be negative, got %v", val)
	}
	opts.Threshold = val
	return nil
}

func parseTimeRangeOptions(args map[string]any, opts *ReportOptions) error {
	if rawStart, ok := args["start_time"]; ok && rawStart != nil {
		t, err := parseTimestamp(rawStart)
		if err != nil {
			return fmt.Errorf("invalid start_time: %w", err)
		}
		opts.StartTime = &t
	}
	if rawEnd, ok := args["end_time"]; ok && rawEnd != nil {
		t, err := parseTimestamp(rawEnd)
		if err != nil {
			return fmt.Errorf("invalid end_time: %w", err)
		}
		opts.EndTime = &t
	}
	if opts.StartTime != nil && opts.EndTime != nil && opts.EndTime.Before(*opts.StartTime) {
		return fmt.Errorf("end_time %s cannot be before start_time %s",
			opts.EndTime.Format(time.RFC3339), opts.StartTime.Format(time.RFC3339))
	}
	return nil
}

func parseDetailsAndEnvOptions(args map[string]any, opts *ReportOptions) {
	if rawDetails, ok := args["include_details"]; ok && rawDetails != nil {
		if b, ok := rawDetails.(bool); ok {
			opts.IncludeDetails = b
		} else if s, ok := rawDetails.(string); ok {
			opts.IncludeDetails = strings.EqualFold(s, "true") || s == "1"
		}
	}
	if rawEnv, ok := args["environment"]; ok && rawEnv != nil {
		if s, ok := rawEnv.(string); ok && strings.TrimSpace(s) != "" {
			opts.Environment = strings.TrimSpace(s)
		}
	}
}

func parseFilterTagsOption(args map[string]any, opts *ReportOptions) {
	rawTags, ok := args["tags"]
	if !ok || rawTags == nil {
		rawTags = args["filter_tags"]
	}
	if rawTags == nil {
		return
	}
	switch typed := rawTags.(type) {
	case map[string]string:
		for k, v := range typed {
			opts.FilterTags[k] = v
		}
	case map[string]any:
		for k, v := range typed {
			opts.FilterTags[k] = fmt.Sprintf("%v", v)
		}
	}
}

func toFloat64(val any) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}

func parseTimestamp(val any) (time.Time, error) {
	switch v := val.(type) {
	case time.Time:
		return v.UTC(), nil
	case string:
		clean := strings.TrimSpace(v)
		if t, err := time.Parse(time.RFC3339Nano, clean); err == nil {
			return t.UTC(), nil
		}
		return time.Time{}, fmt.Errorf("cannot parse timestamp string %q (expected RFC3339)", v)
	case int64:
		return time.Unix(v, 0).UTC(), nil
	case int:
		return time.Unix(int64(v), 0).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type %T", val)
	}
}
