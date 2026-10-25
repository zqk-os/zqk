// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package stewardbase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func readQueueLines(t *testing.T, queueFile string) []map[string]any {
	t.Helper()
	data, err := fileutil.ReadFile(queueFile)
	if err != nil {
		t.Fatalf("failed to read queue file %s: %v", queueFile, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	results := make([]map[string]any, 0, len(lines))
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			t.Fatalf("line %d is not valid JSON (%q): %v", idx, trimmed, err)
		}
		results = append(results, parsed)
	}
	return results
}

func stewardQueueFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "datacell", "steward_enqueue.jsonl")
}

func TestEnqueueStewardMaintenance_UnknownStorageProfileRejection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	testProfiles := []datacell.StorageProfile{
		"",
		"custom_profile",
		"unrecognized_storage",
		"null",
	}

	for _, p := range testProfiles {
		p := p
		t.Run(fmt.Sprintf("profile_%s", p), func(t *testing.T) {
			op := datacell.MaintenanceOp{Name: "test_op", Detail: "some detail"}
			err := EnqueueStewardMaintenance(ctx, tmpDir, p, op, nil)
			if err == nil {
				t.Fatalf("expected error for unknown storage profile %q, got nil", p)
			}
			expectedErrMsg := fmt.Sprintf("unknown storage_profile %q", p)
			if !strings.Contains(err.Error(), expectedErrMsg) {
				t.Fatalf("expected error to contain %q, got %q", expectedErrMsg, err.Error())
			}
		})
	}
}

func TestEnqueueStewardMaintenance_KnownProfilesSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	testCases := []struct {
		name    string
		profile datacell.StorageProfile
		op      datacell.MaintenanceOp
	}{
		{
			name:    "case_cas_entity",
			profile: datacell.ProfileCASEntity,
			op:      datacell.MaintenanceOp{Name: datacell.MaintenanceOpCompactSegments, Detail: "compact cas segments"},
		},
		{
			name:    "case_light_file",
			profile: datacell.ProfileLightFile,
			op:      datacell.MaintenanceOp{Name: datacell.MaintenanceOpRefreshSummary, Detail: "refresh light file summary"},
		},
		{
			name:    "case_stream",
			profile: datacell.ProfileStream,
			op:      datacell.MaintenanceOp{Name: datacell.MaintenanceOpRetentionSweep, Detail: "retention sweep for stream kind"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			err := EnqueueStewardMaintenance(ctx, tmpDir, tc.profile, tc.op, nil)
			if err != nil {
				t.Fatalf("unexpected error enqueueing known profile %q: %v", tc.profile, err)
			}

			queueFile := stewardQueueFilePath(tmpDir)
			records := readQueueLines(t, queueFile)
			if len(records) != 1 {
				t.Fatalf("expected 1 record in queue, found %d", len(records))
			}

			rec := records[0]
			if rec["storage_profile"] != string(tc.profile) {
				t.Fatalf("expected storage_profile %q, got %v", tc.profile, rec["storage_profile"])
			}
			if rec["op"] != tc.op.Name {
				t.Fatalf("expected op %q, got %v", tc.op.Name, rec["op"])
			}
			if rec["detail"] != tc.op.Detail {
				t.Fatalf("expected detail %q, got %v", tc.op.Detail, rec["detail"])
			}
		})
	}
}

func TestEnqueueStewardMaintenance_DetailTruncation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	testCases := []struct {
		name           string
		inputDetailLen int
		expectedLen    int
	}{
		{
			name:           "under_limit",
			inputDetailLen: 250,
			expectedLen:    250,
		},
		{
			name:           "exact_limit",
			inputDetailLen: StewardEnqueueDetailMaxBytes,
			expectedLen:    StewardEnqueueDetailMaxBytes,
		},
		{
			name:           "over_limit",
			inputDetailLen: 800,
			expectedLen:    StewardEnqueueDetailMaxBytes,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			rawDetail := strings.Repeat("x", tc.inputDetailLen)
			op := datacell.MaintenanceOp{
				Name:   "boundary_test_op",
				Detail: rawDetail,
			}

			err := EnqueueStewardMaintenance(ctx, tmpDir, datacell.ProfileCASEntity, op, nil)
			if err != nil {
				t.Fatalf("unexpected error enqueueing: %v", err)
			}

			queueFile := stewardQueueFilePath(tmpDir)
			records := readQueueLines(t, queueFile)
			if len(records) != 1 {
				t.Fatalf("expected 1 record, found %d", len(records))
			}

			detailStr, ok := records[0]["detail"].(string)
			if !ok {
				t.Fatalf("expected detail to be a string, got %T", records[0]["detail"])
			}
			if len(detailStr) != tc.expectedLen {
				t.Fatalf("expected detail len %d, got %d", tc.expectedLen, len(detailStr))
			}
			if !strings.HasPrefix(rawDetail, detailStr) {
				t.Fatalf("truncated detail does not match prefix of input")
			}
		})
	}
}

func TestEnqueueStewardMaintenance_LoggerEmission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	var buf bytes.Buffer
	logger := logging.NewLogger(&buf, logging.InfoLevel, logging.NewJSONFormatter(ctx))

	op := datacell.MaintenanceOp{
		Name:   "op_with_logger",
		Detail: "detail_with_logger",
	}

	err := EnqueueStewardMaintenance(ctx, tmpDir, datacell.ProfileStream, op, logger)
	if err != nil {
		t.Fatalf("unexpected error enqueueing with logger: %v", err)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, LogEventStewardEnqueue) {
		t.Fatalf("expected log output to contain event %q, got: %s", LogEventStewardEnqueue, logOutput)
	}
	if !strings.Contains(logOutput, string(datacell.ProfileStream)) {
		t.Fatalf("expected log output to contain storage_profile %q, got: %s", datacell.ProfileStream, logOutput)
	}
	if !strings.Contains(logOutput, "op_with_logger") {
		t.Fatalf("expected log output to contain op name %q, got: %s", "op_with_logger", logOutput)
	}
	if !strings.Contains(logOutput, "detail_with_logger") {
		t.Fatalf("expected log output to contain detail %q, got: %s", "detail_with_logger", logOutput)
	}
}

func TestEnqueueStewardMaintenance_NilLoggerAndRecordSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	op := datacell.MaintenanceOp{
		Name:   "op_nil_logger",
		Detail: "detail_nil_logger",
	}

	err := EnqueueStewardMaintenance(ctx, tmpDir, datacell.ProfileLightFile, op, nil)
	if err != nil {
		t.Fatalf("unexpected error enqueueing with nil logger: %v", err)
	}

	queueFile := stewardQueueFilePath(tmpDir)
	records := readQueueLines(t, queueFile)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec["schema_version"] != "steward_enqueue_v1" {
		t.Fatalf("expected schema_version 'steward_enqueue_v1', got %v", rec["schema_version"])
	}

	timestampStr, ok := rec["enqueued_at_rfc3339"].(string)
	if !ok {
		t.Fatalf("expected enqueued_at_rfc3339 to be string, got %T", rec["enqueued_at_rfc3339"])
	}

	parsedTime, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		t.Fatalf("failed to parse enqueued_at_rfc3339 (%q) as RFC3339: %v", timestampStr, err)
	}

	diff := time.Since(parsedTime)
	if diff < 0 || diff > 5*time.Minute {
		t.Fatalf("timestamp %v is suspiciously far from now (%v)", parsedTime, diff)
	}
}

func TestAppendStewardEnqueueRecord_RootValidationCases(t *testing.T) {
	t.Parallel()

	invalidRoots := []string{
		"",
		".",
		"./",
	}

	sampleRec := map[string]any{
		"sample_field": "sample_value",
	}

	for _, r := range invalidRoots {
		r := r
		t.Run(fmt.Sprintf("root_%q", r), func(t *testing.T) {
			err := AppendStewardEnqueueRecord(r, sampleRec)
			if err == nil {
				t.Fatalf("expected error for invalid root %q, got nil", r)
			}
			expected := "append steward enqueue: project root required"
			if !strings.Contains(err.Error(), expected) {
				t.Fatalf("expected error message to contain %q, got %q", expected, err.Error())
			}
		})
	}
}

func TestAppendStewardEnqueueRecord_FilesystemBlockError(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Block directory creation by writing a regular file where .zqk directory would be
	blockedPath := filepath.Join(tmpDir, paths.ProjectDataDir)
	if err := fileutil.WriteFile(blockedPath, []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocking file: %v", err)
	}

	rec := map[string]any{"key": "value"}
	err := AppendStewardEnqueueRecord(tmpDir, rec)
	if err == nil {
		t.Fatalf("expected error when .zqk is a regular file, got nil")
	}

	expectedPrefix := "append steward enqueue: "
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Fatalf("expected error to start with %q, got %q", expectedPrefix, err.Error())
	}
}

func TestEnqueueStewardMaintenance_ConcurrentQueueWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	const workerCount = 15
	var wg sync.WaitGroup
	wg.Add(workerCount)

	errCh := make(chan error, workerCount)

	for i := 0; i < workerCount; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("cycle213_concurrent_enqueue", fmt.Sprintf("worker-%d", workerID))
		builder.StartSimple(func() {
			defer wg.Done()
			op := datacell.MaintenanceOp{
				Name:   fmt.Sprintf("worker_op_%d", workerID),
				Detail: fmt.Sprintf("detail for worker %d", workerID),
			}
			err := EnqueueStewardMaintenance(ctx, tmpDir, datacell.ProfileCASEntity, op, nil)
			if err != nil {
				errCh <- fmt.Errorf("worker %d enqueue failed: %w", workerID, err)
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrency error: %v", err)
	}

	queueFile := stewardQueueFilePath(tmpDir)
	records := readQueueLines(t, queueFile)
	if len(records) != workerCount {
		t.Fatalf("expected %d records written concurrently, got %d", workerCount, len(records))
	}

	seenOps := make(map[string]bool)
	for _, r := range records {
		opName, ok := r["op"].(string)
		if !ok {
			t.Fatalf("expected op field to be string, got %T", r["op"])
		}
		seenOps[opName] = true
	}

	for i := 0; i < workerCount; i++ {
		expectedOp := fmt.Sprintf("worker_op_%d", i)
		if !seenOps[expectedOp] {
			t.Fatalf("missing op %q from concurrent execution", expectedOp)
		}
	}
}

func TestPackageConstants_ContractVerification(t *testing.T) {
	t.Parallel()

	if StewardEnqueueDetailMaxBytes != 512 {
		t.Fatalf("expected StewardEnqueueDetailMaxBytes to be 512, got %d", StewardEnqueueDetailMaxBytes)
	}

	if LogEventStewardEnqueue != "datacell_steward_enqueue" {
		t.Fatalf("expected LogEventStewardEnqueue to be 'datacell_steward_enqueue', got %q", LogEventStewardEnqueue)
	}
}
