package quality

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// MatrixProfileYAML is the subset of vetting / test-bundle profiles needed for reporting.
type MatrixProfileYAML struct {
	Fieldnames []string `yaml:"fieldnames"`
	Completion struct {
		GateColumns []string `yaml:"gate_columns"`
		DoneValues  []any    `yaml:"done_values"`
	} `yaml:"completion"`
}

// LoadMatrixProfileYAML loads gate columns and done values from a profile file.
func LoadMatrixProfileYAML(profilePath string) (*MatrixProfileYAML, map[string]struct{}, error) {
	data, err := fileutil.ReadFile(profilePath)
	if err != nil {
		return nil, nil, err
	}
	var raw MatrixProfileYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	if len(raw.Completion.GateColumns) == 0 {
		return nil, nil, errfmt.Errorf("profile %s: completion.gate_columns is empty", profilePath)
	}
	done := make(map[string]struct{})
	for _, v := range raw.Completion.DoneValues {
		s := normalizeDoneValue(v)
		if s != "" {
			done[s] = struct{}{}
		}
	}
	if len(done) == 0 {
		return nil, nil, errfmt.Errorf("profile %s: completion.done_values is empty", profilePath)
	}
	return &raw, done, nil
}

func normalizeDoneValue(v any) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "yes"
		}
		return "no"
	case string:
		return strings.TrimSpace(strings.ToLower(t))
	default:
		return strings.TrimSpace(strings.ToLower(fmt.Sprint(t)))
	}
}

// GateColumnCounts holds per-gate tallies for one matrix.
type GateColumnCounts struct {
	Yes     int `json:"yes"`
	No      int `json:"no"`
	NA      int `json:"na"`
	Pending int `json:"pending"`
	Other   int `json:"other"`
}

// MatrixReportSummary is machine output for zqk matrix report.
type MatrixReportSummary struct {
	MatrixName    string                      `json:"matrix_name"`
	CSVPath       string                      `json:"csv_path"`
	ProfilePath   string                      `json:"profile_path"`
	GoOnly        bool                        `json:"go_only"`
	RowTotal      int                         `json:"row_total"`
	FullyDoneRows int                         `json:"fully_done_rows"`
	GateColumns   []string                    `json:"gate_columns"`
	PerGate       map[string]GateColumnCounts `json:"per_gate"`
	DoneValues    []string                    `json:"done_values"`
	SessionRefCol string                      `json:"session_ref_column,omitempty"`
}

// SummarizeMatrixCSV walks the CSV and counts gate column states using the loaded profile.
func SummarizeMatrixCSV(csvPath string, prof *MatrixProfileYAML, doneVals map[string]struct{}, goOnly bool, sessionRefCol string) (*MatrixReportSummary, error) {
	cr, err := openMatrixCSV(csvPath)
	if err != nil {
		return nil, err
	}
	defer cr.Close()
	cr.reader.ReuseRecord = true

	var (
		r      = cr.reader
		header = cr.header
		colIdx = cr.colIdx
	)
	gates := prof.Completion.GateColumns
	for _, g := range gates {
		if _, ok := colIdx[g]; !ok {
			return nil, errfmt.Errorf("csv missing gate column %q", g)
		}
	}
	filePathIdx := -1
	for _, name := range []string{"file_path", "bundle_label"} {
		if i, ok := colIdx[name]; ok {
			filePathIdx = i
			break
		}
	}

	var doneList []string
	for v := range doneVals {
		doneList = append(doneList, v)
	}
	perGate := make(map[string]GateColumnCounts)
	for _, g := range gates {
		perGate[g] = GateColumnCounts{}
	}

	rowTotal := 0
	fullyDone := 0

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < len(header) {
			continue
		}
		if goOnly && filePathIdx >= 0 {
			fp := strings.TrimSpace(rec[filePathIdx])
			if !strings.HasSuffix(strings.ToLower(fp), ".go") {
				continue
			}
		}
		rowTotal++

		rowAllDone := true
		for _, g := range gates {
			raw := ""
			if j := colIdx[g]; j < len(rec) {
				raw = strings.TrimSpace(rec[j])
			}
			v := strings.ToLower(raw)
			if v == "" {
				v = "pending"
			}
			if _, ok := doneVals[v]; !ok {
				rowAllDone = false
			}
			c := perGate[g]
			switch v {
			case "yes":
				c.Yes++
			case "no":
				c.No++
			case "na":
				c.NA++
			case "pending":
				c.Pending++
			default:
				c.Other++
			}
			perGate[g] = c
		}
		if rowAllDone {
			fullyDone++
		}
	}

	return &MatrixReportSummary{
		CSVPath:       csvPath,
		GoOnly:        goOnly,
		RowTotal:      rowTotal,
		FullyDoneRows: fullyDone,
		GateColumns:   gates,
		PerGate:       perGate,
		DoneValues:    doneList,
		SessionRefCol: sessionRefCol,
	}, nil
}

// SummarizeMatrixFromPaths loads profile + csv and returns a summary (used by CLI).
func SummarizeMatrixFromPaths(csvPath, profilePath string, goOnly bool, sessionRefCol string) (*MatrixReportSummary, error) {
	prof, doneVals, err := LoadMatrixProfileYAML(profilePath)
	if err != nil {
		return nil, err
	}
	sum, err := SummarizeMatrixCSV(csvPath, prof, doneVals, goOnly, sessionRefCol)
	if err != nil {
		return nil, err
	}
	sum.ProfilePath = filepath.Clean(profilePath)
	sum.CSVPath = filepath.Clean(csvPath)
	return sum, nil
}
