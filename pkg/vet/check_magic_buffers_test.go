package vet

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCheckMagicBuffers_FlagsMagicBufferExpressions(t *testing.T) {
	tempDir := t.TempDir()

	badSrc := `package sample

import (
	"bufio"
	"bytes"
)

func parseSample(data []byte) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
}
`
	goodSrc := `package sample

import (
	"bufio"
	"bytes"
)

const (
	initialBuf = 64 * 1024
	maxBuf     = 1024 * 1024
)

func parseClean(data []byte) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, initialBuf), maxBuf)
}
`

	badFile := filepath.Join(tempDir, "bad.go")
	goodFile := filepath.Join(tempDir, "good.go")

	if err := fileutil.WriteFile(badFile, []byte(badSrc), 0644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}
	if err := fileutil.WriteFile(goodFile, []byte(goodSrc), 0644); err != nil {
		t.Fatalf("write good file: %v", err)
	}

	cfg := DefaultConfig()
	findings, err := CheckMagicBuffers(tempDir, []string{badFile, goodFile}, cfg)
	if err != nil {
		t.Fatalf("CheckMagicBuffers error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected findings for magic buffer allocation, got none")
	}

	foundBad := false
	for _, f := range findings {
		if filepath.Base(f.File) == "bad.go" {
			foundBad = true
		}
		if filepath.Base(f.File) == "good.go" {
			t.Errorf("unexpected finding for clean code in good.go: %s", f.Message)
		}
	}

	if !foundBad {
		t.Error("expected finding in bad.go")
	}
}
