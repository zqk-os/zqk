package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/vet"
)

func main() {
	var (
		rootDir   string
		cfgFile   string
		suiteFlag string
		jsonOut   bool
		failFast  bool
	)

	flag.StringVar(&rootDir, "root", ".", "repository root directory")
	flag.StringVar(&cfgFile, "config", "", "path to gates.yaml config file (default: <root>/config/gates.yaml)")
	flag.StringVar(&suiteFlag, "suite", "all", "comma-separated list of suites to run: hygiene, tree, payload, all")
	flag.BoolVar(&jsonOut, "json", false, "output report as JSON")
	flag.BoolVar(&failFast, "fail-fast", false, "stop execution on first suite failure")
	flag.Parse()

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "zqk-vet: resolve root %s: %v\n", rootDir, err)
		os.Exit(2)
	}

	cfg, err := vet.LoadConfig(absRoot, cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "zqk-vet: load config: %v\n", err)
		os.Exit(2)
	}

	var suites []string
	for _, s := range strings.Split(suiteFlag, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			suites = append(suites, s)
		}
	}

	runner := vet.NewRunner(absRoot, cfg)
	report, err := runner.Run(vet.RunOptions{
		Suites:   suites,
		Files:    flag.Args(),
		FailFast: failFast,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "zqk-vet: error running checks: %v\n", err)
		os.Exit(2)
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "zqk-vet: encode json: %v\n", err)
			os.Exit(2)
		}
	} else {
		runner.PrintReport(os.Stdout, report)
	}

	if !report.Passed {
		os.Exit(1)
	}
}
