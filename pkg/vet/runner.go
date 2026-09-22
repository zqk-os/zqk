package vet

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// RunOptions configures the execution of vet suites.
type RunOptions struct {
	Suites   []string
	Files    []string
	FailFast bool
}

// Runner coordinates execution of verification checks.
type Runner struct {
	Root   string
	Config *GatesConfig
}

// NewRunner creates a new Runner for the given repo root and config.
func NewRunner(root string, cfg *GatesConfig) *Runner {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Runner{
		Root:   root,
		Config: cfg,
	}
}

// Run executes the requested suites and aggregates results.
func (r *Runner) Run(opts RunOptions) (*Report, error) {
	start := time.Now()
	report := &Report{
		Passed:       true,
		SuiteResults: make(map[string]SuiteResult),
	}

	suitesToRun := r.normalizeSuites(opts.Suites)

	for _, suite := range suitesToRun {
		sStart := time.Now()
		var findings []Finding
		var err error

		switch suite {
		case "hygiene":
			findings, err = r.runHygiene(opts.Files)
		case "tree", "tree_police":
			findings, err = CheckTreePolice(r.Root, r.Config)
			suite = "tree_police"
		case "payload":
			findings, err = CheckPayload(r.Root, r.Config)
		default:
			return nil, fmt.Errorf("unknown vet suite: %s", suite)
		}

		if err != nil {
			return nil, fmt.Errorf("suite %s failed: %w", suite, err)
		}

		suitePassed := true
		for _, f := range findings {
			if f.Severity == SeverityError {
				suitePassed = false
				report.TotalErrors++
			} else if f.Severity == SeverityWarn {
				report.TotalWarns++
			}
		}

		if !suitePassed {
			report.Passed = false
		}

		report.SuiteResults[suite] = SuiteResult{
			Suite:    suite,
			Passed:   suitePassed,
			Duration: time.Since(sStart),
			Findings: findings,
		}

		if opts.FailFast && !suitePassed {
			break
		}
	}

	report.Duration = time.Since(start)
	return report, nil
}

func (r *Runner) runHygiene(files []string) ([]Finding, error) {
	var findings []Finding

	if r.Config.Hygiene.CheckPaths || r.Config.Hygiene.CheckPerms {
		pathPermFindings, err := CheckPathsAndPerms(r.Root, r.Config)
		if err != nil {
			return nil, err
		}
		findings = append(findings, pathPermFindings...)
	}

	if r.Config.Hygiene.CheckCLINames {
		cliFindings, err := CheckCLINames(r.Root, files)
		if err != nil {
			return nil, err
		}
		findings = append(findings, cliFindings...)
	}

	return findings, nil
}

func (r *Runner) normalizeSuites(suites []string) []string {
	if len(suites) == 0 {
		return []string{"hygiene", "tree_police", "payload"}
	}
	var res []string
	for _, s := range suites {
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "all" {
			return []string{"hygiene", "tree_police", "payload"}
		}
		res = append(res, s)
	}
	return res
}

// PrintReport writes a human-readable summary of the report to w.
func (r *Runner) PrintReport(w io.Writer, report *Report) {
	fmt.Fprintln(w, "=== ZQK Verification Engine (zqk-vet) ===")
	for suiteName, res := range report.SuiteResults {
		status := "PASS"
		if !res.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(w, "[%s] Suite %-12s (%v, %d findings)\n", status, suiteName, res.Duration.Round(time.Millisecond), len(res.Findings))
		for _, f := range res.Findings {
			fmt.Fprintf(w, "  %s\n", f.String())
		}
	}
	fmt.Fprintln(w, "----------------------------------------")
	if report.Passed {
		fmt.Fprintf(w, "RESULT: PASS (0 errors, %d warnings, took %v)\n", report.TotalWarns, report.Duration.Round(time.Millisecond))
	} else {
		fmt.Fprintf(w, "RESULT: FAIL (%d errors, %d warnings, took %v)\n", report.TotalErrors, report.TotalWarns, report.Duration.Round(time.Millisecond))
	}
}
