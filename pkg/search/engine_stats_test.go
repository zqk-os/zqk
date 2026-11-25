package search

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEngine_BuildTrigramIndexStats(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	files := map[string]string{
		"main.go":  "package main\nfunc HandleRequest() {}\n",
		"app.ts":   "export class Router { dispatch() {} }\n",
		"rules.py": "def evaluate_policy(): return True\n",
	}
	for rel, body := range files {
		if err := fileutil.WriteFile(filepath.Join(root, rel), []byte(body), paths.FilePerm644); err != nil {
			t.Fatalf("failed writing %s: %v", rel, err)
		}
	}

	engine := NewEngine(root)
	stats, err := engine.BuildTrigramIndexStats(SearchOptions{UseIndex: true})
	if err != nil {
		t.Fatalf("BuildTrigramIndexStats failed: %v", err)
	}
	if stats.IndexedFiles != len(files) {
		t.Errorf("expected %d indexed files, got %d", len(files), stats.IndexedFiles)
	}
	if stats.IndexBytes <= 0 {
		t.Errorf("expected positive persisted index size in bytes, got %d", stats.IndexBytes)
	}
	if stats.CachePath != engine.CachePath() {
		t.Errorf("expected cache path %q, got %q", engine.CachePath(), stats.CachePath)
	}
	if stats.BuildDurationMs < 0 {
		t.Errorf("expected non-negative build duration, got %f", stats.BuildDurationMs)
	}
}
