package systemcheckwake

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadConfig_ExistingValid(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Dir(ConfigPath(root))
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	one := 5
	pulse := false
	cfg := Config{
		MinDraftPlane:          &one,
		MinBlocking:            &one,
		PulseHuman:             &pulse,
		IncludeRecommendations: true,
	}
	data, _ := json.Marshal(cfg)
	if err := fileutil.WriteFile(ConfigPath(root), data, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}
	if derefMin(loaded.MinDraftPlane, 0) != 5 {
		t.Errorf("expected MinDraftPlane 5, got %d", derefMin(loaded.MinDraftPlane, 0))
	}
	if derefBool(loaded.PulseHuman, true) != false {
		t.Errorf("expected PulseHuman false, got true")
	}
	if !loaded.IncludeRecommendations {
		t.Errorf("expected IncludeRecommendations true")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Dir(ConfigPath(root))
	_ = fileutil.MkdirAll(configDir, paths.DirPerm755)
	_ = fileutil.WriteFile(ConfigPath(root), []byte("{bad-json"), paths.FilePerm644)

	_, err := LoadConfig(root)
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestWriteExampleConfig(t *testing.T) {
	root := t.TempDir()
	err := WriteExampleConfig(root, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error writing example config: %v", err)
	}

	p := ConfigPath(root) + ".example"
	if _, err := fileutil.Stat(p); err != nil {
		t.Fatalf("expected example config file to exist: %v", err)
	}
}

func TestNotify_ThresholdsClear(t *testing.T) {
	root := t.TempDir()
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-1",
		Summary:     Summary{},
		Config:      DefaultConfig(),
		Logger:      logger,
	}

	res := Notify(opts)
	if !res.Requested {
		t.Error("expected Requested=true")
	}
	if res.Tripped {
		t.Error("expected Tripped=false")
	}
	if res.Skipped != "thresholds_clear" {
		t.Errorf("expected skipped=thresholds_clear, got %s", res.Skipped)
	}
}

func TestNotify_Tripped_EmptyAgentID(t *testing.T) {
	root := t.TempDir()
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "",
		Summary: Summary{
			BlockingIssues: 1,
		},
		Config: DefaultConfig(),
	}

	res := Notify(opts)
	if !res.Tripped {
		t.Error("expected Tripped=true")
	}
	if res.Skipped != "empty_to_agent_id" {
		t.Errorf("expected skipped=empty_to_agent_id, got %s", res.Skipped)
	}
}

func TestNotify_Tripped_FeedWake(t *testing.T) {
	root := t.TempDir()
	// Set up .zqk dir structure
	_ = fileutil.MkdirAll(filepath.Join(root, paths.ProjectDataDir), paths.DirPerm755)

	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	feedWake := true
	pulseHuman := true
	opts := NotifyOpts{
		ProjectRoot: root,
		ToAgentID:   "agent-worker-1",
		Summary: Summary{
			DraftPlaneTotal: 2,
			Warnings:        3,
		},
		Config: Config{
			FeedWake:   &feedWake,
			PulseHuman: &pulseHuman,
		},
		Logger:  logger,
		Context: context.Background(),
	}

	res := Notify(opts)
	if !res.Tripped {
		t.Error("expected Tripped=true")
	}
	if len(res.Reasons) != 2 {
		t.Errorf("expected 2 trip reasons, got %d", len(res.Reasons))
	}
	if res.ToAgentID != "agent-worker-1" {
		t.Errorf("expected to_agent_id=agent-worker-1, got %s", res.ToAgentID)
	}
}
