package testrunner

import (
	"bytes"
	"context"
	"os"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/gotestparse"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RaceGateConfig configures an isolated race gate execution.
type RaceGateConfig struct {
	PackagePath string        `json:"package_path"`
	Timeout     time.Duration `json:"timeout"`
	ProjectRoot string        `json:"project_root"`
	ExtraArgs   []string      `json:"extra_args,omitempty"`
	Env         []string      `json:"env,omitempty"`
}

// RaceGateResult contains the structured outcome of a race gate execution.
type RaceGateResult struct {
	Passed            bool                           `json:"passed"`
	HasDataRace       bool                           `json:"has_data_race"`
	ExitCode          int                            `json:"exit_code"`
	Output            string                         `json:"output"`
	Duration          time.Duration                  `json:"duration"`
	Error             string                         `json:"error,omitempty"`
	DataRaceCount     int                            `json:"data_race_count"`
	DataRaceIncidents []gotestparse.DataRaceIncident `json:"data_race_incidents,omitempty"`
}

// RunRaceGate executes package tests under go test -race with strict isolation and timeout guarantees.
func RunRaceGate(ctx context.Context, cfg RaceGateConfig) (*RaceGateResult, error) {
	if cfg.PackagePath == "" {
		return nil, errfmt.Errorf("package_path is required for race gate")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	res := &RaceGateResult{}

	args := []string{"test", "-race", "-v"}
	if len(cfg.ExtraArgs) > 0 {
		args = append(args, cfg.ExtraArgs...)
	}
	args = append(args, cfg.PackagePath)

	cmd := execwrap.CommandContext(runCtx, "go", args...)
	if cfg.ProjectRoot != "" {
		cmd.Dir = cfg.ProjectRoot
	}

	// Ephemeral TMPDIR provisioning
	tmpDir, err := fileutil.MkdirTemp("", "zqk-racegate-")
	if err == nil {
		defer func() {
			_ = fileutil.RemoveAll(tmpDir)
		}()
	}

	// PGID Isolation
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	cmd.Env = os.Environ()
	if os.Getenv("DEVELOPER_DIR") == "" {
		if _, err := os.Stat("/Library/Developer/CommandLineTools"); err == nil {
			cmd.Env = append(cmd.Env, "DEVELOPER_DIR=/Library/Developer/CommandLineTools")
		}
	}
	if tmpDir != "" {
		cmd.Env = append(cmd.Env,
			"TMPDIR="+tmpDir,
			"TEMP="+tmpDir,
			"TMP="+tmpDir,
		)
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Env, cfg.Env...)
	}

	var combinedBuf bytes.Buffer
	cmd.Stdout = &combinedBuf
	cmd.Stderr = &combinedBuf

	runErr := cmd.Run()
	res.Duration = time.Since(start)
	res.Output = combinedBuf.String()

	if runErr != nil {
		res.ExitCode = 1
		if exitErr, ok := runErr.(interface{ ExitCode() int }); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.Error = runErr.Error()
		}
	}

	// Parse test output with gotestparse
	summary, parseErr := gotestparse.ParseGoTestOutput(res.Output)
	if parseErr == nil && summary != nil {
		if summary.HasDataRace {
			res.HasDataRace = true
			res.DataRaceCount = summary.DataRaceCount
			res.DataRaceIncidents = summary.DataRaceIncidents
		}
	}

	// Pass condition: exit code 0 AND no data races detected
	if res.ExitCode == 0 && !res.HasDataRace {
		res.Passed = true
	} else {
		res.Passed = false
		if res.HasDataRace && res.Error == "" {
			res.Error = "data race detected"
		}
	}

	return res, nil
}
