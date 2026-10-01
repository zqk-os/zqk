package system

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/appledouble"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	hashDupesSubdir = "hash-duplicates"
)

// QuarantineReportData is the structured output for quarantine-report.
// Intended for dashboard consumption: --format json or --format yaml provides this data;
// a future dashboard can aggregate this with other system-health data for a summarized view.
type QuarantineReportData struct {
	QuarantineRoot string                     `json:"quarantine_root" yaml:"quarantine_root"`
	TotalFiles     int                        `json:"total_files" yaml:"total_files"`
	Subfolders     []QuarantineSubfolder      `json:"subfolders" yaml:"subfolders"`
	HashDuplicates *QuarantineByKindBreakdown `json:"hash_duplicates,omitempty" yaml:"hash_duplicates,omitempty"`
}

// QuarantineSubfolder is one top-level subfolder under quarantine (e.g. hash-duplicates or a kind for CAS corruption).
type QuarantineSubfolder struct {
	Name   string `json:"name" yaml:"name"`
	Count  int    `json:"count" yaml:"count"`
	Source string `json:"source" yaml:"source"`
}

// QuarantineByKindBreakdown is per-kind file counts under hash-duplicates (included when --verbose or in JSON/YAML).
type QuarantineByKindBreakdown struct {
	ByKind map[string]int `json:"by_kind" yaml:"by_kind"`
}

// NewQuarantineReportCmd creates a command to report what's in the quarantine folder.
// Output is data-oriented: use --format json or --format yaml for dashboard/consumer use;
// default table is a human-readable summary. A future dashboard can aggregate this with other system-health data.
func NewQuarantineReportCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Report contents of the quarantine folder (data for dashboard)",
		"Outputs quarantine stats (file counts by source). Use --format json or --format yaml for structured data suitable for a dashboard or other consumers; default is a human-readable table.",
		"",
		"See "+filepath.Join(paths.ProcessDir, "system-health", "QUARANTINE_AND_ANALYSIS.md")+" for what goes in quarantine and the dashboard data story.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemQuarantineReportCommandBuilder(), &cobra.Command{
		Use:   "quarantine-report",
		Short: "Report quarantine folder contents (data for dashboard)",
		RunE:  runQuarantineReport,
	})
	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().Bool("verbose", false, "Include per-kind breakdown under hash-duplicates (always included in JSON/YAML)")
	return cmd
}

// BuildQuarantineReportData builds quarantine report data for the project.
// Used by quarantine-report and health-data; returns empty data if quarantine dir does not exist.
func BuildQuarantineReportData(projectRoot string) (*QuarantineReportData, error) {
	quarantineRoot := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, paths.QuarantineDir)
	info, err := fileutil.Stat(quarantineRoot)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return &QuarantineReportData{QuarantineRoot: quarantineRoot, TotalFiles: 0, Subfolders: nil}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, errfmt.Errorf("quarantine path is not a directory: %s", quarantineRoot)
	}
	entries, err := fileutil.ReadDir(quarantineRoot)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var totalFiles int
	var subfolders []QuarantineSubfolder
	var hashDupesPath string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dirPath := filepath.Join(quarantineRoot, name)
		count, err := countYAMLFiles(dirPath)
		if err != nil {
			return nil, err
		}
		totalFiles += count
		source := "CAS corruption repair"
		if name == hashDupesSubdir {
			source = "Stale CAS / hash-duplicates"
			hashDupesPath = dirPath
		}
		subfolders = append(subfolders, QuarantineSubfolder{Name: name, Count: count, Source: source})
	}
	data := &QuarantineReportData{
		QuarantineRoot: quarantineRoot,
		TotalFiles:     totalFiles,
		Subfolders:     subfolders,
	}
	if hashDupesPath != emptyValue {
		byKind, err := hashDuplicatesKindCounts(hashDupesPath)
		if err != nil {
			return nil, err
		}
		data.HashDuplicates = &QuarantineByKindBreakdown{ByKind: byKind}
	}
	return data, nil
}

func runQuarantineReport(cmd *cobra.Command, _ []string) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("not in a ZQK project (no .zqk found)")
	}
	verbose, _ := cmd.Flags().GetBool("verbose")
	data, err := BuildQuarantineReportData(projectRoot)
	if err != nil {
		return err
	}
	return outputQuarantineReport(cmd, data, verbose)
}

func outputQuarantineReport(cmd *cobra.Command, data *QuarantineReportData, verbose bool) error {
	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, data)
	default:
		return writeQuarantineReportTable(cmd, data, verbose)
	}
}

func writeQuarantineReportTable(cmd *cobra.Command, data *QuarantineReportData, verbose bool) error {
	var buf []byte
	buf = append(buf, fmt.Sprintf("Quarantine: %s\n", data.QuarantineRoot)...)
	buf = append(buf, fmt.Sprintf("Total files: %d\n", data.TotalFiles)...)
	if len(data.Subfolders) > 0 {
		buf = append(buf, '\n')
		for _, s := range data.Subfolders {
			buf = append(buf, fmt.Sprintf("  %s: %d files (%s)\n", s.Name, s.Count, s.Source)...)
		}
	}
	if verbose && data.HashDuplicates != nil && len(data.HashDuplicates.ByKind) > 0 {
		buf = append(buf, "  Breakdown (hash-duplicates by kind):\n"...)
		kinds := make([]string, 0, len(data.HashDuplicates.ByKind))
		for k := range data.HashDuplicates.ByKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			buf = append(buf, fmt.Sprintf("    %s: %d\n", k, data.HashDuplicates.ByKind[k])...)
		}
	}
	return cli.WriteOutput(cmd, buf)
}

func countYAMLFiles(dir string) (int, error) {
	var n int
	err := filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // continue walking on path error
		}
		if info.IsDir() {
			return nil
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml" {
			n++
		}
		return nil
	})
	return n, err
}

func hashDuplicatesKindCounts(hashDupesDir string) (map[string]int, error) {
	entries, err := fileutil.ReadDir(hashDupesDir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		kind := e.Name()
		count, err := countYAMLFiles(filepath.Join(hashDupesDir, kind))
		if err != nil {
			return nil, err
		}
		out[kind] = count
	}
	return out, nil
}
