package quality

import (
	"bytes"
	"encoding/csv"
	"io"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// projectMatrixColumns narrows Header and each row to the listed columns (order preserved).
// Empty or all-blank columns means no projection.
func projectMatrixColumns(res *MatrixGetResult, columns []string) error {
	var normalized []string
	seen := make(map[string]struct{})
	for _, c := range columns {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			return errfmt.Errorf("duplicate column %q in --field", c)
		}
		seen[c] = struct{}{}
		normalized = append(normalized, c)
	}
	if len(normalized) == 0 {
		return nil
	}
	headerSet := make(map[string]struct{}, len(res.Header))
	for _, h := range res.Header {
		headerSet[h] = struct{}{}
	}
	for _, c := range normalized {
		if _, ok := headerSet[c]; !ok {
			return errfmt.Errorf("unknown column %q (not in CSV header)", c)
		}
	}
	res.Header = append([]string(nil), normalized...)
	for i := range res.Rows {
		row := res.Rows[i]
		newRow := make(map[string]string, len(normalized))
		for _, c := range normalized {
			newRow[c] = row[c]
		}
		res.Rows[i] = newRow
	}
	return nil
}

// MatrixGetResult is machine output for zqk matrix get.
type MatrixGetResult struct {
	MatrixName       string              `json:"matrix_name" yaml:"matrix_name"`
	CSVPath          string              `json:"csv_path" yaml:"csv_path"`
	ProfilePath      string              `json:"profile_path" yaml:"profile_path"`
	SessionRefColumn string              `json:"session_ref_column,omitempty" yaml:"session_ref_column,omitempty"`
	Header           []string            `json:"header" yaml:"header"`
	Rows             []map[string]string `json:"rows" yaml:"rows"`
	RowCount         int                 `json:"row_count" yaml:"row_count"`
}

// FormatMatrixGetResultCSV renders the filtered row set as RFC 4180 CSV (header + data rows).
func FormatMatrixGetResultCSV(res *MatrixGetResult) ([]byte, error) {
	if res == nil {
		return nil, errfmt.Errorf("matrix get result is nil")
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(res.Header); err != nil {
		return nil, err
	}
	for _, row := range res.Rows {
		rec := make([]string, len(res.Header))
		for i, col := range res.Header {
			rec[i] = row[col]
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}

// QueryMatrixCSV loads the CSV and returns rows matching filters.
// filters: AND of exact column=value (after trim). Empty map applies no column filters.
// globPattern: if non-empty, applies filepath.Match to file_path column when present.
// goOnly: if true and file_path column exists, keep only rows ending in .go.
// cvsID: if non-empty, sessionRefCol must be set and rows must match that column exactly.
// limit: if > 0, stop after this many matching rows.
// columns: if non-empty (after trim), output only those CSV columns in that order; filters still apply to full row data.
func QueryMatrixCSV(csvPath, profilePath, matrixName string, filters map[string]string, globPattern string, goOnly bool, cvsID string, sessionRefCol string, limit int, columns []string) (*MatrixGetResult, error) {
	f, err := fileutil.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	colIdx := make(map[string]int, len(header))
	for i, h := range header {
		colIdx[h] = i
	}

	filePathIdx := -1
	if i, ok := colIdx[objects.FieldKeyFilePath]; ok {
		filePathIdx = i
	}
	if goOnly && filePathIdx < 0 {
		return nil, errfmt.Errorf("csv has no file_path column; cannot use --go-only")
	}

	globPat := strings.TrimSpace(globPattern)
	if globPat != "" && filePathIdx < 0 {
		return nil, errfmt.Errorf("csv has no file_path column; cannot use --glob")
	}
	cvs := strings.TrimSpace(cvsID)
	refCol := strings.TrimSpace(sessionRefCol)
	if cvs != "" && refCol == "" {
		return nil, errfmt.Errorf("--cvs-id requires a session ref column (configure session_ref_column for this matrix in matrix_registry.yaml)")
	}
	var refIdx int
	if cvs != "" {
		var ok bool
		refIdx, ok = colIdx[refCol]
		if !ok {
			return nil, errfmt.Errorf("csv has no column %q for --cvs-id", refCol)
		}
	}

	var out []map[string]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		for len(rec) < len(header) {
			rec = append(rec, "")
		}
		row := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(rec) {
				row[name] = strings.TrimSpace(rec[i])
			} else {
				row[name] = ""
			}
		}

		if goOnly && filePathIdx >= 0 {
			fp := row[objects.FieldKeyFilePath]
			if !strings.HasSuffix(strings.ToLower(fp), ".go") {
				continue
			}
		}

		if globPat != "" && filePathIdx >= 0 {
			fp := filepath.ToSlash(row[objects.FieldKeyFilePath])
			ok, err := filepath.Match(globPat, fp)
			if err != nil || !ok {
				continue
			}
		}

		if cvs != "" {
			if refIdx >= len(rec) {
				continue
			}
			if strings.TrimSpace(rec[refIdx]) != cvs {
				continue
			}
		}

		skip := false
		for col, want := range filters {
			c := strings.TrimSpace(col)
			got, ok := row[c]
			if !ok {
				skip = true
				break
			}
			if got != strings.TrimSpace(want) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		out = append(out, row)
		if limit > 0 && len(out) >= limit {
			break
		}
	}

	res := &MatrixGetResult{
		MatrixName:       matrixName,
		CSVPath:          filepath.Clean(csvPath),
		ProfilePath:      filepath.Clean(profilePath),
		SessionRefColumn: refCol,
		Header:           header,
		Rows:             out,
		RowCount:         len(out),
	}
	if err := projectMatrixColumns(res, columns); err != nil {
		return nil, err
	}
	return res, nil
}
