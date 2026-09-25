package processhygiene_test

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSupplyNoticeAttributionAndReproducibleBuilds(t *testing.T) {
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatal("could not locate repo root containing go.mod")
		}
		repoRoot = parent
	}

	// 1. Verify NOTICE attribution for transitive dependencies
	noticePath := filepath.Join(repoRoot, "NOTICE")
	noticeContent, err := fileutil.ReadFile(noticePath)
	if err != nil {
		t.Fatalf("failed to read NOTICE file: %v", err)
	}
	text := string(noticeContent)
	for _, dep := range []string{"github.com/google/go-cmp", "github.com/joho/godotenv", "github.com/mitchellh/go-ps", "go.uber.org/goleak"} {
		if !strings.Contains(text, dep) {
			t.Errorf("NOTICE file missing open source attribution for %s", dep)
		}
	}

	// 2. Verify Makefile reproducible build flags
	makefilePath := filepath.Join(repoRoot, "Makefile")
	makefileContent, err := fileutil.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("failed to read Makefile: %v", err)
	}
	makeStr := string(makefileContent)
	if !strings.Contains(makeStr, "-trimpath") {
		t.Errorf("Makefile missing -trimpath flag in compilation")
	}
	if !strings.Contains(makeStr, "SOURCE_DATE_EPOCH") {
		t.Errorf("Makefile missing SOURCE_DATE_EPOCH handling for reproducible builds")
	}
}
