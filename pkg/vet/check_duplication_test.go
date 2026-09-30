package vet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckDuplication_CleanCodePasses(t *testing.T) {
	tmpDir := t.TempDir()

	fileA := filepath.Join(tmpDir, "pkg", "mod_a", "a.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(fileA), 0755))
	contentA := `package moda

func CalculateSum(values []int) int {
	total := 0
	for _, v := range values {
		if v > 0 {
			total += v
		}
	}
	return total
}
`
	require.NoError(t, os.WriteFile(fileA, []byte(contentA), 0644))

	fileB := filepath.Join(tmpDir, "pkg", "mod_b", "b.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(fileB), 0755))
	contentB := `package modb

func CalculateProduct(values []int) int {
	product := 1
	for _, v := range values {
		if v > 1 {
			product *= v
		}
	}
	return product
}
`
	require.NoError(t, os.WriteFile(fileB, []byte(contentB), 0644))

	cfg := DefaultConfig()
	cfg.Hygiene.GoScanDirs = []string{"pkg/"}
	cfg.Hygiene.CheckDups = true
	cfg.Hygiene.DupMinStatements = 3
	cfg.Hygiene.DupMinLines = 5

	findings, err := CheckDuplication(tmpDir, []string{"pkg/mod_a/a.go", "pkg/mod_b/b.go"}, cfg)
	require.NoError(t, err)
	require.Empty(t, findings)
}

func TestCheckDuplication_DetectsIdenticalFunctions(t *testing.T) {
	tmpDir := t.TempDir()

	fileA := filepath.Join(tmpDir, "pkg", "mod_a", "a.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(fileA), 0755))
	contentA := `package moda

import "strings"

func ProcessItems(items []any) []string {
	var results []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				results = append(results, trimmed)
			}
		}
	}
	return results
}
`
	require.NoError(t, os.WriteFile(fileA, []byte(contentA), 0644))

	fileB := filepath.Join(tmpDir, "pkg", "mod_b", "b.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(fileB), 0755))
	// Exactly the same logic, different function name
	contentB := `package modb

import "strings"

func ExtractNonEmptyStrings(items []any) []string {
	var results []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				results = append(results, trimmed)
			}
		}
	}
	return results
}
`
	require.NoError(t, os.WriteFile(fileB, []byte(contentB), 0644))

	cfg := DefaultConfig()
	cfg.Hygiene.GoScanDirs = []string{"pkg/"}
	cfg.Hygiene.CheckDups = true
	cfg.Hygiene.DupMinStatements = 3
	cfg.Hygiene.DupMinLines = 6

	findings, err := CheckDuplication(tmpDir, []string{"pkg/mod_a/a.go", "pkg/mod_b/b.go"}, cfg)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	require.Equal(t, "hygiene/duplication", findings[0].CheckID)
	require.Equal(t, SeverityWarn, findings[0].Severity)
	require.Contains(t, findings[0].Message, "duplicative function body")
	require.Contains(t, findings[0].Message, "refactor into a shared abstraction (DRY)")
}

func TestCheckDuplication_ExemptionsRespected(t *testing.T) {
	tmpDir := t.TempDir()

	fileA := filepath.Join(tmpDir, "pkg", "mod_a", "a.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(fileA), 0755))
	contentA := `package moda

func DoSomethingComplex() int {
	x := 10
	y := 20
	z := x + y
	if z > 15 {
		z *= 2
	}
	return z
}
`
	require.NoError(t, os.WriteFile(fileA, []byte(contentA), 0644))

	// fileB is a test file
	fileB := filepath.Join(tmpDir, "pkg", "mod_b", "b_test.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(fileB), 0755))
	require.NoError(t, os.WriteFile(fileB, []byte(contentA), 0644))

	cfg := DefaultConfig()
	cfg.Hygiene.GoScanDirs = []string{"pkg/"}
	cfg.Hygiene.CheckDups = true
	cfg.Hygiene.DupMinStatements = 3
	cfg.Hygiene.DupMinLines = 5

	findings, err := CheckDuplication(tmpDir, []string{"pkg/mod_a/a.go", "pkg/mod_b/b_test.go"}, cfg)
	require.NoError(t, err)
	require.Empty(t, findings)
}
