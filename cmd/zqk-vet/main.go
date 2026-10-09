package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/cmd/zqk/app"
	"github.com/zqk-os/zqk/pkg/vet"
)

func runMain(args []string, stdout, stderr io.Writer) int {
	var (
		rootDir   string
		cfgFile   string
		suiteFlag string
		jsonOut   bool
		failFast  bool
	)

	fs := flag.NewFlagSet("zqk-vet", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.StringVar(&rootDir, "root", ".", "repository root directory")
	fs.StringVar(&cfgFile, "config", "", "path to gates.yaml config file (default: <root>/config/gates.yaml)")
	fs.StringVar(&suiteFlag, "suite", "all", "comma-separated list of suites to run: hygiene, tree, payload, all")
	fs.BoolVar(&jsonOut, "json", false, "output report as JSON")
	fs.BoolVar(&failFast, "fail-fast", false, "stop execution on first suite failure")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		fmt.Fprintf(stderr, "zqk-vet: resolve root %s: %v\n", rootDir, err)
		return 2
	}

	cfg, err := vet.LoadConfig(absRoot, cfgFile)
	if err != nil {
		fmt.Fprintf(stderr, "zqk-vet: load config: %v\n", err)
		return 2
	}

	var suites []string
	for _, s := range strings.Split(suiteFlag, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			suites = append(suites, s)
		}
	}

	runner := vet.NewRunner(absRoot, cfg)
	runner.RootCommand = app.NewRootCommand()
	report, err := runner.Run(vet.RunOptions{
		Suites:   suites,
		Files:    fs.Args(),
		FailFast: failFast,
	})
	if err != nil {
		fmt.Fprintf(stderr, "zqk-vet: error running checks: %v\n", err)
		return 2
	}

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "zqk-vet: encode json: %v\n", err)
			return 2
		}
	} else {
		runner.PrintReport(stdout, report)
	}

	if !report.Passed {
		return 1
	}

	return 0
}

func main() {
	os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}
