package testrunner

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRunRaceGate_MissingPackage(t *testing.T) {
	t.Parallel()
	_, err := RunRaceGate(context.Background(), RaceGateConfig{})
	if err == nil {
		t.Fatal("expected error for empty package path")
	}
}

func TestRunRaceGate_CleanPackage(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := RunRaceGate(ctx, RaceGateConfig{
		PackagePath: "./pkg/gotestparse",
		ExtraArgs:   []string{"-run", "TestParseGoTestOutput_FailWithoutPackagePrefix"},
		ProjectRoot: filepathDirN(wd, 2),
		Timeout:     20 * time.Second,
	})
	if err != nil {
		t.Fatalf("RunRaceGate failed: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected res.Passed=true, got false. Output:\n%s", res.Output)
	}
	if res.HasDataRace {
		t.Errorf("expected no data races, got %d", res.DataRaceCount)
	}
}

func filepathDirN(path string, n int) string {
	res := path
	for i := 0; i < n; i++ {
		res = res + "/.."
	}
	return res
}
