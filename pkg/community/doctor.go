package community

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CheckSeverity indicates the diagnostic check severity level.
type CheckSeverity string

const (
	// SeverityPass indicates that the diagnostic check passed cleanly.
	SeverityPass CheckSeverity = "PASS"
	// SeverityWarn indicates a non-fatal warning or sub-optimal setting.
	SeverityWarn CheckSeverity = "WARN"
	// SeverityFail indicates a fatal error or missing prerequisite.
	SeverityFail CheckSeverity = "FAIL"
)

// DiagnosticResult contains the outcome of an individual diagnostic check.
type DiagnosticResult struct {
	Name        string        `json:"name"`
	Category    string        `json:"category"`
	Severity    CheckSeverity `json:"severity"`
	Message     string        `json:"message"`
	Remediation string        `json:"remediation,omitempty"`
	DurationMs  int64         `json:"duration_ms"`
}

// DoctorReport aggregates all diagnostic results for an environment health check.
type DoctorReport struct {
	Timestamp   time.Time          `json:"timestamp"`
	GoVersion   string             `json:"go_version"`
	OS          string             `json:"os"`
	Arch        string             `json:"arch"`
	TotalChecks int                `json:"total_checks"`
	Passed      int                `json:"passed"`
	Warnings    int                `json:"warnings"`
	Failures    int                `json:"failures"`
	Results     []DiagnosticResult `json:"results"`
}

// DoctorChecker defines the interface for executing diagnostic checks.
type DoctorChecker interface {
	Name() string
	Category() string
	Check(ctx context.Context, rootDir string) DiagnosticResult
}

// SystemDoctor orchestrates and runs diagnostic health checks.
type SystemDoctor struct {
	rootDir  string
	checkers []DoctorChecker
}

// NewSystemDoctor creates a new SystemDoctor initialized with default community checkers.
func NewSystemDoctor(rootDir string) *SystemDoctor {
	doc := &SystemDoctor{
		rootDir: rootDir,
	}
	doc.Register(
		&GitInstalledChecker{},
		&GoInstalledChecker{},
		&FilesystemPermissionsChecker{},
		&ProcessDirectoryChecker{},
		&MemoryAndCoreChecker{},
	)
	return doc
}

// Register adds one or more checkers to the diagnostic suite.
func (d *SystemDoctor) Register(checkers ...DoctorChecker) {
	d.checkers = append(d.checkers, checkers...)
}

// RunDiagnostics executes all registered checks and returns a DoctorReport.
func (d *SystemDoctor) RunDiagnostics(ctx context.Context) DoctorReport {
	report := DoctorReport{
		Timestamp: time.Now().UTC(),
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Results:   make([]DiagnosticResult, 0, len(d.checkers)),
	}

	for _, checker := range d.checkers {
		if ctx.Err() != nil {
			report.Results = append(report.Results, DiagnosticResult{
				Name:       checker.Name(),
				Category:   checker.Category(),
				Severity:   SeverityFail,
				Message:    fmt.Sprintf("check aborted: %v", ctx.Err()),
				DurationMs: 0,
			})
			report.Failures++
			report.TotalChecks++
			continue
		}

		start := time.Now()
		res := checker.Check(ctx, d.rootDir)
		res.DurationMs = time.Since(start).Milliseconds()

		switch res.Severity {
		case SeverityPass:
			report.Passed++
		case SeverityWarn:
			report.Warnings++
		case SeverityFail:
			report.Failures++
		default:
			res.Severity = SeverityFail
			report.Failures++
		}

		report.Results = append(report.Results, res)
		report.TotalChecks++
	}

	return report
}

// FormatReportJSON serializes a DoctorReport to indented JSON.
func FormatReportJSON(report DoctorReport) (string, error) {
	bytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// GitInstalledChecker verifies that git binary is present and reachable.
type GitInstalledChecker struct{}

func (c *GitInstalledChecker) Name() string     { return "git_available" }
func (c *GitInstalledChecker) Category() string { return "toolchain" }
func (c *GitInstalledChecker) Check(ctx context.Context, rootDir string) DiagnosticResult {
	path, err := exec.LookPath("git")
	if err != nil {
		return DiagnosticResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Severity:    SeverityFail,
			Message:     "git binary not found in PATH",
			Remediation: "Install git from https://git-scm.com/ or via system package manager.",
		}
	}
	return DiagnosticResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: SeverityPass,
		Message:  fmt.Sprintf("git binary detected at %s", path),
	}
}

// GoInstalledChecker verifies that go toolchain is present and valid.
type GoInstalledChecker struct{}

func (c *GoInstalledChecker) Name() string     { return "go_available" }
func (c *GoInstalledChecker) Category() string { return "toolchain" }
func (c *GoInstalledChecker) Check(ctx context.Context, rootDir string) DiagnosticResult {
	path, err := exec.LookPath("go")
	if err != nil {
		return DiagnosticResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Severity:    SeverityWarn,
			Message:     "go binary not found in PATH",
			Remediation: "Install Go toolchain (go >= 1.22) from https://golang.org/dl/.",
		}
	}
	return DiagnosticResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: SeverityPass,
		Message:  fmt.Sprintf("go toolchain detected at %s (%s)", path, runtime.Version()),
	}
}

// FilesystemPermissionsChecker verifies write permissions in the specified workspace root.
type FilesystemPermissionsChecker struct{}

func (c *FilesystemPermissionsChecker) Name() string     { return "workspace_writable" }
func (c *FilesystemPermissionsChecker) Category() string { return "filesystem" }
func (c *FilesystemPermissionsChecker) Check(ctx context.Context, rootDir string) DiagnosticResult {
	if rootDir == "" {
		rootDir = "."
	}
	info, err := fileutil.Stat(rootDir)
	if err != nil {
		return DiagnosticResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Severity:    SeverityFail,
			Message:     fmt.Sprintf("cannot stat workspace root: %v", err),
			Remediation: "Ensure the workspace directory exists and is readable.",
		}
	}
	if !info.IsDir() {
		return DiagnosticResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Severity:    SeverityFail,
			Message:     fmt.Sprintf("workspace path %q is not a directory", rootDir),
			Remediation: "Point workspace root to a valid directory.",
		}
	}

	testFile := filepath.Join(rootDir, fmt.Sprintf(".doctor_write_probe_%d.tmp", time.Now().UnixNano()))
	if err := fileutil.WriteSecureFile(testFile, []byte("zqk-doctor-probe")); err != nil {
		return DiagnosticResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Severity:    SeverityFail,
			Message:     fmt.Sprintf("workspace directory is not writable: %v", err),
			Remediation: "Grant write permissions to the workspace directory.",
		}
	}
	_ = fileutil.RemoveFile(testFile)

	return DiagnosticResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: SeverityPass,
		Message:  "workspace directory is writable",
	}
}

// ProcessDirectoryChecker checks for existence and accessibility of .zqk process storage.
type ProcessDirectoryChecker struct{}

func (c *ProcessDirectoryChecker) Name() string     { return "process_storage" }
func (c *ProcessDirectoryChecker) Category() string { return "storage" }
func (c *ProcessDirectoryChecker) Check(ctx context.Context, rootDir string) DiagnosticResult {
	if rootDir == "" {
		rootDir = "."
	}
	zqkDir := filepath.Join(rootDir, ".zqk")
	if info, err := fileutil.Stat(zqkDir); err == nil && info.IsDir() {
		return DiagnosticResult{
			Name:     c.Name(),
			Category: c.Category(),
			Severity: SeverityPass,
			Message:  fmt.Sprintf("kernel process storage directory found at %s", zqkDir),
		}
	}
	return DiagnosticResult{
		Name:        c.Name(),
		Category:    c.Category(),
		Severity:    SeverityWarn,
		Message:     "no .zqk metadata directory found in workspace root",
		Remediation: "Run 'zqk init' or ensure current directory is a valid ZQK repository.",
	}
}

// MemoryAndCoreChecker checks runtime CPU count and basic allocation sanity.
type MemoryAndCoreChecker struct{}

func (c *MemoryAndCoreChecker) Name() string     { return "runtime_resources" }
func (c *MemoryAndCoreChecker) Category() string { return "environment" }
func (c *MemoryAndCoreChecker) Check(ctx context.Context, rootDir string) DiagnosticResult {
	cpus := runtime.NumCPU()
	if cpus < 1 {
		return DiagnosticResult{
			Name:     c.Name(),
			Category: c.Category(),
			Severity: SeverityFail,
			Message:  "invalid CPU core count detected",
		}
	}
	if cpus == 1 {
		return DiagnosticResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Severity:    SeverityWarn,
			Message:     "single CPU core detected; multi-threaded scheduler operations may experience contention",
			Remediation: "Allocate at least 2 CPU cores for optimal performance.",
		}
	}
	return DiagnosticResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: SeverityPass,
		Message:  fmt.Sprintf("%d CPU cores detected", cpus),
	}
}
