// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package systemcheckwake

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/primaryorch"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockFailingWakeAdapter struct {
	failErr error
}

func (m *mockFailingWakeAdapter) Wake(_ context.Context, _ agentfeed.PeerWakeRequest) (agentfeed.PeerWakeAdapterResult, error) {
	return agentfeed.PeerWakeAdapterResult{
		Transport: "failing-mock",
		Live:      false,
	}, m.failErr
}

func discardLogger() logging.Logger {
	return logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
}

func enableAgentChatChannel(t *testing.T, root string) {
	t.Helper()
	cfg := datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeNotify,
	}
	if err := datacell.WriteAgentChatChannelConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultConfig_Values(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SchemaVersion != SchemaVersion {
		t.Fatalf("expected schema version %s, got %s", SchemaVersion, cfg.SchemaVersion)
	}
	if derefBool(cfg.PulseHuman, false) != true {
		t.Fatal("expected PulseHuman default to true")
	}
	if derefBool(cfg.FeedWake, false) != true {
		t.Fatal("expected FeedWake default to true")
	}
	if derefMin(cfg.MinDraftPlane, 0) != 1 {
		t.Fatalf("expected MinDraftPlane 1, got %d", derefMin(cfg.MinDraftPlane, 0))
	}
	if derefMin(cfg.MinErrorStatus, 0) != 1 {
		t.Fatalf("expected MinErrorStatus 1, got %d", derefMin(cfg.MinErrorStatus, 0))
	}
	if derefMin(cfg.MinBlocking, 0) != 1 {
		t.Fatalf("expected MinBlocking 1, got %d", derefMin(cfg.MinBlocking, 0))
	}
	if derefMin(cfg.MinWarnings, 0) != 1 {
		t.Fatalf("expected MinWarnings 1, got %d", derefMin(cfg.MinWarnings, 0))
	}
	if derefMin(cfg.MinInformational, 0) != 1 {
		t.Fatalf("expected MinInformational 1, got %d", derefMin(cfg.MinInformational, 0))
	}
}

func TestConfigPath_Construction(t *testing.T) {
	root := filepath.Join("var", "testroot")
	expected := paths.AgentRuntimeFile(root, ConfigFileName)
	got := ConfigPath(root)
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestDerefMinAndBool_EdgeCases(t *testing.T) {
	if got := derefMin(nil, 42); got != 42 {
		t.Fatalf("expected fallback 42, got %d", got)
	}
	val := 17
	if got := derefMin(&val, 42); got != 17 {
		t.Fatalf("expected 17, got %d", got)
	}
	if got := derefBool(nil, true); got != true {
		t.Fatal("expected fallback true")
	}
	if got := derefBool(nil, false); got != false {
		t.Fatal("expected fallback false")
	}
	bTrue := true
	if got := derefBool(&bTrue, false); got != true {
		t.Fatal("expected true")
	}
	bFalse := false
	if got := derefBool(&bFalse, true); got != false {
		t.Fatal("expected false")
	}
}

func TestLoadConfig_ComprehensiveOverrides(t *testing.T) {
	root := t.TempDir()
	p := ConfigPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	payload := `{
		"schema_version": "2.0-test",
		"pulse_human": false,
		"feed_wake": false,
		"min_draft_plane": 3,
		"min_error_status": 4,
		"min_blocking": 5,
		"min_warnings": 6,
		"min_informational": 7,
		"include_recommendations": true,
		"min_recommendations": 8
	}`
	if err := fileutil.WriteFile(p, []byte(payload), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.SchemaVersion != "2.0-test" {
		t.Fatalf("expected schema version 2.0-test, got %s", loaded.SchemaVersion)
	}
	if derefBool(loaded.PulseHuman, true) != false {
		t.Fatal("expected PulseHuman false")
	}
	if derefBool(loaded.FeedWake, true) != false {
		t.Fatal("expected FeedWake false")
	}
	if derefMin(loaded.MinDraftPlane, 0) != 3 {
		t.Fatalf("expected MinDraftPlane 3, got %d", derefMin(loaded.MinDraftPlane, 0))
	}
	if derefMin(loaded.MinErrorStatus, 0) != 4 {
		t.Fatalf("expected MinErrorStatus 4, got %d", derefMin(loaded.MinErrorStatus, 0))
	}
	if derefMin(loaded.MinBlocking, 0) != 5 {
		t.Fatalf("expected MinBlocking 5, got %d", derefMin(loaded.MinBlocking, 0))
	}
	if derefMin(loaded.MinWarnings, 0) != 6 {
		t.Fatalf("expected MinWarnings 6, got %d", derefMin(loaded.MinWarnings, 0))
	}
	if derefMin(loaded.MinInformational, 0) != 7 {
		t.Fatalf("expected MinInformational 7, got %d", derefMin(loaded.MinInformational, 0))
	}
	if derefMin(loaded.MinRecommendations, 0) != 8 {
		t.Fatalf("expected MinRecommendations 8, got %d", derefMin(loaded.MinRecommendations, 0))
	}
	if !loaded.IncludeRecommendations {
		t.Fatal("expected IncludeRecommendations true")
	}
}

func TestLoadConfig_DirectoryReadError(t *testing.T) {
	root := t.TempDir()
	p := ConfigPath(root)
	if err := fileutil.MkdirAll(p, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(root)
	if err == nil {
		t.Fatal("expected read error when config path is a directory")
	}
	if !strings.Contains(err.Error(), "systemcheckwake: read config") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestWriteExampleConfig_EmptySchemaVersionAndError(t *testing.T) {
	root := t.TempDir()
	p := ConfigPath(root) + ".example"
	if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	cfg := Config{}
	if err := WriteExampleConfig(root, cfg); err != nil {
		t.Fatalf("WriteExampleConfig failed: %v", err)
	}
	if _, err := fileutil.Stat(p); err != nil {
		t.Fatalf("example file was not created: %v", err)
	}

	customCfg := Config{SchemaVersion: "example-v3"}
	if err := WriteExampleConfig(root, customCfg); err != nil {
		t.Fatalf("WriteExampleConfig with custom schema failed: %v", err)
	}

	badRoot := t.TempDir()
	badExamplePath := ConfigPath(badRoot) + ".example"
	if err := fileutil.MkdirAll(badExamplePath, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := WriteExampleConfig(badRoot, cfg); err == nil {
		t.Fatal("expected error when example config path is an existing directory")
	}
}

func TestEvaluate_AllThresholdTypesAndDisabled(t *testing.T) {
	zero := 0
	cfg := Config{
		MinDraftPlane:          &zero,
		MinErrorStatus:         &zero,
		MinBlocking:            &zero,
		MinWarnings:            &zero,
		MinInformational:       &zero,
		MinRecommendations:     &zero,
		IncludeRecommendations: true,
	}
	sum := Summary{
		DraftPlaneTotal:    10,
		ErrorStatusObjects: 10,
		BlockingIssues:     10,
		Warnings:           10,
		Informational:      10,
		Recommendations:    10,
	}
	if reasons := Evaluate(sum, cfg); len(reasons) != 0 {
		t.Fatalf("disabled thresholds should not trip: %v", reasons)
	}

	one := 2
	cfgActive := Config{
		MinDraftPlane:          &one,
		MinErrorStatus:         &one,
		MinBlocking:            &one,
		MinWarnings:            &one,
		MinInformational:       &one,
		MinRecommendations:     &one,
		IncludeRecommendations: true,
	}
	sumActive := Summary{
		DraftPlaneTotal:    2,
		ErrorStatusObjects: 3,
		BlockingIssues:     2,
		Warnings:           4,
		Informational:      2,
		Recommendations:    1,
	}
	reasons := Evaluate(sumActive, cfgActive)
	if len(reasons) != 5 {
		t.Fatalf("expected 5 reasons, got %d (%v)", len(reasons), reasons)
	}

	cfgActive.IncludeRecommendations = false
	if reasonsNoRec := Evaluate(Summary{Recommendations: 10}, cfgActive); len(reasonsNoRec) != 0 {
		t.Fatalf("recommendations should not trip when disabled: %v", reasonsNoRec)
	}
}

func TestResolveNotifyAgent_AllBranches(t *testing.T) {
	root := t.TempDir()

	custom, err := ResolveNotifyAgent(root, "  agent-worker-42  ")
	if err != nil {
		t.Fatal(err)
	}
	if custom != "agent-worker-42" {
		t.Fatalf("expected trimmed agent-worker-42, got %q", custom)
	}

	defAgent, err := ResolveNotifyAgent(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if defAgent == "" {
		t.Fatal("expected non-empty default agent")
	}

	orchPath := primaryorch.ConfigPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(orchPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(orchPath, []byte("{invalid-json"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNotifyAgent(root, ""); err == nil {
		t.Fatal("expected parse binding error on invalid json")
	}

	blankBinding := `{"schema_version": "1", "agent_id": "   "}`
	if err := fileutil.WriteFile(orchPath, []byte(blankBinding), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNotifyAgent(root, PrimarySentinel); err == nil {
		t.Fatal("expected error on empty agent_id")
	}
}

func TestNotify_ThresholdsClearWithAndWithoutLogger(t *testing.T) {
	root := t.TempDir()
	optsNoLogger := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-1",
		Summary:     Summary{},
		Config:      DefaultConfig(),
	}
	res1 := Notify(optsNoLogger)
	if res1.Skipped != "thresholds_clear" || res1.Tripped {
		t.Fatalf("unexpected result: %#v", res1)
	}

	optsWithLogger := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-1",
		Summary:     Summary{},
		Config:      DefaultConfig(),
		Logger:      discardLogger(),
	}
	res2 := Notify(optsWithLogger)
	if res2.Skipped != "thresholds_clear" || res2.Tripped {
		t.Fatalf("unexpected result with logger: %#v", res2)
	}
}

func TestNotify_EmptySchemaVersionDefaults(t *testing.T) {
	root := t.TempDir()
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-1",
		Summary:     Summary{BlockingIssues: 1},
		Config:      Config{},
	}
	res := Notify(opts)
	if !res.Tripped {
		t.Fatal("expected Tripped=true with defaulted config")
	}
}

func TestNotify_FeedWakeDisabledWithPulseHuman(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Dir(ConfigPath(root))
	if err := fileutil.MkdirAll(runtimeDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	feedOff := false
	pulseOn := true
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-worker-1",
		Summary:     Summary{BlockingIssues: 1},
		Config: Config{
			SchemaVersion: SchemaVersion,
			FeedWake:      &feedOff,
			PulseHuman:    &pulseOn,
		},
		Logger: discardLogger(),
	}

	res := Notify(opts)
	if !res.Tripped {
		t.Fatal("expected Tripped=true")
	}
	if !res.IdeBridgeQueued {
		t.Fatal("expected IdeBridgeQueued=true when PulseHuman is enabled")
	}
}

func TestNotify_FeedWakeAndPulseDisabled(t *testing.T) {
	root := t.TempDir()
	feedOff := false
	pulseOff := false
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-worker-1",
		Summary:     Summary{BlockingIssues: 1},
		Config: Config{
			SchemaVersion: SchemaVersion,
			FeedWake:      &feedOff,
			PulseHuman:    &pulseOff,
		},
	}

	res := Notify(opts)
	if !res.Tripped {
		t.Fatal("expected Tripped=true")
	}
	if res.IdeBridgeQueued {
		t.Fatal("expected IdeBridgeQueued=false when both are disabled")
	}
}

func TestNotify_FeedAppendFailureBranch(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := fileutil.WriteFile(tempFile, []byte("plain file"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	feedOn := true
	opts := NotifyOpts{
		ProjectRoot: tempFile,
		ToAgentID:   "agent-worker-1",
		Summary:     Summary{BlockingIssues: 1},
		Config: Config{
			SchemaVersion: SchemaVersion,
			FeedWake:      &feedOn,
		},
		Logger: discardLogger(),
	}

	res := Notify(opts)
	if !res.Tripped {
		t.Fatal("expected Tripped=true")
	}
	if res.Error == "" {
		t.Fatal("expected Error to be recorded when feed append fails")
	}
}

func TestNotify_PeerAckAwaitFailureBranch(t *testing.T) {
	root := t.TempDir()
	enableAgentChatChannel(t, root)

	awaitPath := paths.PeerAckAwaitsPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(awaitPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(awaitPath, []byte("{corrupt-json"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	feedOn := true
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-worker-1",
		Summary:     Summary{BlockingIssues: 1},
		Config: Config{
			SchemaVersion: SchemaVersion,
			FeedWake:      &feedOn,
		},
		Logger: discardLogger(),
	}

	res := Notify(opts)
	if !res.Tripped {
		t.Fatal("expected Tripped=true")
	}
	if res.Error == "" {
		t.Fatal("expected Error to be recorded on peer-ack await failure")
	}
	if !strings.Contains(res.Error, "parse peer_ack awaits") {
		t.Fatalf("unexpected error message: %s", res.Error)
	}
}

func TestNotify_WakeAdapterErrorBranch(t *testing.T) {
	root := t.TempDir()
	enableAgentChatChannel(t, root)

	simulatedErr := errors.New("simulated wake delivery failure")
	mockAdapter := &mockFailingWakeAdapter{failErr: simulatedErr}

	agentfeed.SetPeerWakeAdapter(mockAdapter)
	t.Cleanup(func() {
		agentfeed.SetPeerWakeAdapter(nil)
	})

	feedOn := true
	pulseOff := false
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-worker-9",
		Summary:     Summary{DraftPlaneTotal: 2},
		Config: Config{
			SchemaVersion: SchemaVersion,
			FeedWake:      &feedOn,
			PulseHuman:    &pulseOff,
		},
		Logger: discardLogger(),
	}

	res := Notify(opts)
	if !res.Tripped {
		t.Fatal("expected Tripped=true")
	}
	if res.Error != simulatedErr.Error() {
		t.Fatalf("expected error %q, got %q", simulatedErr.Error(), res.Error)
	}
}

func TestNotify_FeedWakeSuccessAndLoggerNil(t *testing.T) {
	root := t.TempDir()
	enableAgentChatChannel(t, root)

	feedOn := true
	pulseOn := true
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-worker-success",
		Summary:     Summary{BlockingIssues: 2},
		Config: Config{
			SchemaVersion: SchemaVersion,
			FeedWake:      &feedOn,
			PulseHuman:    &pulseOn,
		},
		Logger:  nil, // tests opts.Logger == nil branch
		Context: nil, // tests opts.Context == nil branch
	}

	res := Notify(opts)
	if !res.Tripped {
		t.Fatal("expected Tripped=true")
	}
	if res.EventID == "" {
		t.Fatal("expected non-empty EventID")
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
}

func TestFormatReasonsAndWakeMessage(t *testing.T) {
	if got := formatReasons(nil); got != "" {
		t.Fatalf("expected empty string for nil reasons, got %q", got)
	}

	reasons := []TripReason{
		{Name: "blocking", Count: 3, Min: 1},
		{Name: "warnings", Count: 5, Min: 2},
	}
	formattedReasons := formatReasons(reasons)
	expectedReasons := "blocking=3,warnings=5"
	if formattedReasons != expectedReasons {
		t.Fatalf("expected %q, got %q", expectedReasons, formattedReasons)
	}

	sum := Summary{
		DraftPlaneTotal:    1,
		ErrorStatusObjects: 2,
		BlockingIssues:     3,
		Warnings:           4,
		Informational:      5,
	}
	msg := formatWakeMessage(sum, reasons)
	if !strings.Contains(msg, "ATTN system-check --notify") {
		t.Fatalf("wake message missing header: %s", msg)
	}
	if !strings.Contains(msg, expectedReasons) {
		t.Fatalf("wake message missing reasons: %s", msg)
	}
}

func TestConcurrentEvaluationsAndLoadConfig(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	if err := WriteExampleConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
	p := ConfigPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(p, []byte(`{"schema_version":"1","min_warnings":2}`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 40)

	for i := 0; i < 20; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("concurrent_evaluator", "evaluating system check summaries").
			WithWaitGroup(&wg).
			StartSimple(func() {
				sum := Summary{Warnings: workerID}
				reasons := Evaluate(sum, cfg)
				if workerID >= 1 && len(reasons) == 0 {
					errCh <- errors.New("expected reasons for non-zero warnings")
				}
			})
	}

	for i := 0; i < 20; i++ {
		goroutinelabels.NewGoroutine("concurrent_loader", "loading config concurrently").
			WithWaitGroup(&wg).
			StartSimple(func() {
				loaded, err := LoadConfig(root)
				if err != nil {
					errCh <- err
					return
				}
				if derefMin(loaded.MinWarnings, 0) != 2 {
					errCh <- errors.New("unexpected MinWarnings value")
				}
			})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent worker failed: %v", err)
		}
	}
}
