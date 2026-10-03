package quality

import (
	"encoding/csv"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// MatrixUpdateResult is machine output for zqk matrix update.
type MatrixUpdateResult struct {
	MatrixName  string            `json:"matrix_name"`
	CSVPath     string            `json:"csv_path"`
	ProfilePath string            `json:"profile_path"`
	MatchColumn string            `json:"match_column"`
	MatchValue  string            `json:"match_value"`
	RowIndex    int               `json:"row_index"` // 0-based index among data rows (excluding header)
	Updates     map[string]string `json:"updates"`
	DryRun      bool              `json:"dry_run"`
	Wrote       bool              `json:"wrote"`
	PreviousRow map[string]string `json:"previous_row,omitempty"`
	RowAfter    map[string]string `json:"row_after,omitempty"` // full row after applying updates (preview + post-write)
	BackupPath  string            `json:"backup_path,omitempty"`
	// CvsActivityAppend is set by the CLI when --append-cvs-activity runs after a successful write.
	CvsActivityAppend *MatrixCvsActivityAppendResult `json:"cvs_activity_append,omitempty"`
}

// MatrixBulkUpdateResult is machine output for zqk matrix update --filter ...
type MatrixBulkUpdateResult struct {
	MatrixName      string              `json:"matrix_name"`
	CSVPath         string              `json:"csv_path"`
	ProfilePath     string              `json:"profile_path"`
	Filters         map[string]string   `json:"filters"`
	Updates         map[string]string   `json:"updates"`
	RowIndices      []int               `json:"row_indices"` // 0-based data row indices
	UpdatedCount    int                 `json:"updated_count"`
	DryRun          bool                `json:"dry_run"`
	Wrote           bool                `json:"wrote"`
	BackupPath      string              `json:"backup_path,omitempty"`
	RowAfterSamples []map[string]string `json:"row_after_samples,omitempty"` // first N updated rows (full column map)
	// CvsIDs is the sorted unique non-empty session ref column values from updated rows when the caller
	// passed sessionRefCol to UpdateMatrixCSVByFilter (e.g. registry session_ref_column).
	CvsIDs []string `json:"cvs_ids,omitempty"`
	// CvsActivityAppend is set by the CLI when --append-cvs-activity runs after a successful write.
	CvsActivityAppend *MatrixCvsActivityAppendResult `json:"cvs_activity_append,omitempty"`
}

// MatrixCvsActivityAppendResult reports convergence_session activity_log append attempts after matrix update.
type MatrixCvsActivityAppendResult struct {
	Requested bool              `json:"requested"`
	Skipped   string            `json:"skipped,omitempty"`
	IDs       []string          `json:"ids,omitempty"`
	Succeeded []string          `json:"succeeded,omitempty"`
	Failed    map[string]string `json:"failed,omitempty"`
}

// MatrixWriteOpts configures an optional backup of the current CSV before atomic replace.
// If BackupPath is non-empty, it is used as the destination. Otherwise, if Backup is true,
// the destination is csvPath+".bak".
type MatrixWriteOpts struct {
	Backup     bool
	BackupPath string
}

func (o *MatrixWriteOpts) resolveBackupDest(csvPath string) (string, bool) {
	if o == nil {
		return "", false
	}
	if p := strings.TrimSpace(o.BackupPath); p != "" {
		return filepath.Clean(p), true
	}
	if o.Backup {
		return csvPath + ".bak", true
	}
	return "", false
}

func copyFileForBackup(src, dst string) error {
	if filepath.Clean(src) == filepath.Clean(dst) {
		return errfmt.Errorf("backup path must differ from csv path")
	}
	data, err := fileutil.ReadFile(src)
	if err != nil {
		return err
	}
	if err := fileutil.EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	return fileutil.WriteSecureFile(dst, data)
}

const maxRowAfterSamples = 5

// matrixRowToMap builds a column-name -> cell map for one CSV row (trimmed values).
func matrixRowToMap(header []string, row []string) map[string]string {
	out := make(map[string]string, len(header))
	for i, h := range header {
		h = strings.TrimSpace(h)
		v := ""
		if i < len(row) {
			v = row[i]
		}
		out[h] = strings.TrimSpace(v)
	}
	return out
}

func maybeBackupCSV(csvPath string, opts *MatrixWriteOpts) (written string, err error) {
	var dest string
	var ok bool
	if opts != nil {
		dest, ok = opts.resolveBackupDest(csvPath)
	}
	if !ok {
		return "", nil
	}
	if err := copyFileForBackup(csvPath, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func normalizePathMatch(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(s))
}

func parseSetFlags(pairs []string) (map[string]string, error) {
	return ParseColumnValuePairs(pairs, true, "--set")
}

func gateColumnSet(prof *MatrixProfileYAML) map[string]struct{} {
	m := make(map[string]struct{})
	for _, g := range prof.Completion.GateColumns {
		m[strings.TrimSpace(g)] = struct{}{}
	}
	return m
}

// rowMatchesFilters returns true if rec satisfies all filters (AND). file_path values are compared
// using normalizePathMatch; other columns use trimmed string equality.
func rowMatchesFilters(rec []string, colIdx map[string]int, filters map[string]string) bool {
	for col, want := range filters {
		c := strings.TrimSpace(col)
		idx, ok := colIdx[c]
		if !ok {
			return false
		}
		var got string
		if idx < len(rec) {
			got = strings.TrimSpace(rec[idx])
		}
		want = strings.TrimSpace(want)
		if c == "file_path" {
			if normalizePathMatch(got) != normalizePathMatch(want) {
				return false
			}
		} else {
			if got != want {
				return false
			}
		}
	}
	return true
}

// applyUpdatesToRow copies rec, applies updates (expanding length as needed) with gate validation.
func applyUpdatesToRow(rec []string, colIdx map[string]int, headerLen int, updates map[string]string, gates map[string]struct{}, doneVals map[string]struct{}) ([]string, error) {
	row := append([]string(nil), rec...)
	for len(row) < headerLen {
		row = append(row, "")
	}
	for col, val := range updates {
		c := strings.TrimSpace(col)
		v := strings.TrimSpace(val)
		if _, isGate := gates[c]; isGate {
			nv := strings.ToLower(v)
			if nv == "" {
				nv = "pending"
			}
			if _, ok := doneVals[nv]; !ok {
				return nil, errfmt.Errorf("invalid value for gate column %q: %q (not in profile completion.done_values)", c, v)
			}
		}
		idx := colIdx[c]
		for len(row) <= idx {
			row = append(row, "")
		}
		row[idx] = v
	}
	return row, nil
}

// collectUniqueCvsIDs returns sorted unique non-empty values in sessionCol for the given row indices.
func collectUniqueCvsIDs(colIdx map[string]int, sessionCol string, rows [][]string, indices []int) []string {
	sessionCol = strings.TrimSpace(sessionCol)
	if sessionCol == "" {
		return nil
	}
	idx, ok := colIdx[sessionCol]
	if !ok {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, ri := range indices {
		if ri < 0 || ri >= len(rows) {
			continue
		}
		row := rows[ri]
		v := ""
		if idx < len(row) {
			v = strings.TrimSpace(row[idx])
		}
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}

// UpdateMatrixCSVByFilter updates every row matching filters (AND) with the same updates.
// valueMap: optional registry value_map (token -> canonical); applied to parsed --set values before validation.
// limit: if > 0, only the first limit matching rows (in file order) are updated.
// sessionRefCol: when non-empty and present in the CSV header, result CvsIDs lists unique non-empty values from updated rows.
func UpdateMatrixCSVByFilter(csvPath, profilePath, matrixName string, filters map[string]string, setPairs []string, valueMap map[string]string, limit int, dryRun bool, writeOpts *MatrixWriteOpts, sessionRefCol string) (*MatrixBulkUpdateResult, error) {
	if len(filters) == 0 {
		return nil, errfmt.Errorf("at least one --filter column=value is required for bulk update")
	}
	updates, err := parseSetFlags(setPairs)
	if err != nil {
		return nil, err
	}
	ApplyValueMapToUpdates(updates, valueMap)
	if len(updates) == 0 {
		return nil, errfmt.Errorf("at least one --set column=value is required")
	}

	prof, doneVals, err := LoadMatrixProfileYAML(profilePath)
	if err != nil {
		return nil, err
	}
	gates := gateColumnSet(prof)

	cr, err := openMatrixCSV(csvPath)
	if err != nil {
		return nil, err
	}
	defer cr.Close()

	r := cr.reader
	header := cr.header
	colIdx := cr.colIdx
	for col := range filters {
		c := strings.TrimSpace(col)
		if _, ok := colIdx[c]; !ok {
			return nil, errfmt.Errorf("unknown filter column %q (not in csv header)", c)
		}
	}
	for col := range updates {
		c := strings.TrimSpace(col)
		if _, ok := colIdx[c]; !ok {
			return nil, errfmt.Errorf("unknown column %q (not in csv header)", c)
		}
	}

	var rows [][]string
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
		rows = append(rows, rec)
	}

	var matchIdx []int
	for i, rec := range rows {
		if rowMatchesFilters(rec, colIdx, filters) {
			matchIdx = append(matchIdx, i)
			if limit > 0 && len(matchIdx) >= limit {
				break
			}
		}
	}
	if len(matchIdx) == 0 {
		return nil, errfmt.Errorf("no rows match --filter")
	}

	for _, fi := range matchIdx {
		newRow, err := applyUpdatesToRow(rows[fi], colIdx, len(header), updates, gates, doneVals)
		if err != nil {
			return nil, err
		}
		rows[fi] = newRow
	}

	samples := make([]map[string]string, 0, min(len(matchIdx), maxRowAfterSamples))
	for _, fi := range matchIdx {
		if len(samples) >= maxRowAfterSamples {
			break
		}
		samples = append(samples, matrixRowToMap(header, rows[fi]))
	}

	res := &MatrixBulkUpdateResult{
		MatrixName:      matrixName,
		CSVPath:         filepath.Clean(csvPath),
		ProfilePath:     filepath.Clean(profilePath),
		Filters:         filters,
		Updates:         updates,
		RowIndices:      matchIdx,
		UpdatedCount:    len(matchIdx),
		DryRun:          dryRun,
		RowAfterSamples: samples,
		CvsIDs:          collectUniqueCvsIDs(colIdx, sessionRefCol, rows, matchIdx),
	}

	if dryRun {
		return res, nil
	}

	bp, err := maybeBackupCSV(csvPath, writeOpts)
	if err != nil {
		return nil, err
	}
	if bp != "" {
		res.BackupPath = bp
	}
	if err := writeCSVAtomic(csvPath, header, rows); err != nil {
		return nil, err
	}
	res.Wrote = true
	return res, nil
}

// UpdateMatrixCSVRow finds the row where matchColumn equals matchValue, applies updates, validates
// gate columns against doneVals, and atomically replaces the CSV file (unless dryRun).
// valueMap: optional registry value_map (token -> canonical); applied to parsed --set values before validation.
func UpdateMatrixCSVRow(csvPath, profilePath, matrixName, matchColumn, matchValue string, setPairs []string, valueMap map[string]string, dryRun bool, writeOpts *MatrixWriteOpts) (*MatrixUpdateResult, error) {
	updates, err := parseSetFlags(setPairs)
	if err != nil {
		return nil, err
	}
	ApplyValueMapToUpdates(updates, valueMap)
	matchColumn = strings.TrimSpace(matchColumn)
	if matchColumn == "" {
		return nil, errfmt.Errorf("match column is required")
	}
	matchValue = strings.TrimSpace(matchValue)
	if matchValue == "" {
		return nil, errfmt.Errorf("match value is required")
	}

	prof, doneVals, err := LoadMatrixProfileYAML(profilePath)
	if err != nil {
		return nil, err
	}
	gates := gateColumnSet(prof)

	cr, err := openMatrixCSV(csvPath)
	if err != nil {
		return nil, err
	}
	defer cr.Close()

	r := cr.reader
	header := cr.header
	colIdx := cr.colIdx
	matchKey := strings.TrimSpace(matchColumn)
	mi, ok := colIdx[matchKey]
	if !ok {
		return nil, errfmt.Errorf("csv missing match column %q", matchKey)
	}
	for col := range updates {
		c := strings.TrimSpace(col)
		if _, ok := colIdx[c]; !ok {
			return nil, errfmt.Errorf("unknown column %q (not in csv header)", c)
		}
	}

	var rows [][]string
	var matchNorm string
	if matchKey == "file_path" {
		matchNorm = normalizePathMatch(matchValue)
	} else {
		matchNorm = matchValue
	}

	found := -1
	rowNum := 0
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
		cell := ""
		if mi < len(rec) {
			cell = strings.TrimSpace(rec[mi])
		}
		var cellNorm string
		if matchKey == "file_path" {
			cellNorm = normalizePathMatch(cell)
		} else {
			cellNorm = cell
		}
		if cellNorm == matchNorm {
			found = rowNum
		}
		rows = append(rows, rec)
		rowNum++
	}
	if found < 0 {
		return nil, errfmt.Errorf("no row with %s=%q", matchKey, matchValue)
	}

	row := rows[found]
	prev := make(map[string]string, len(header))
	for name, idx := range colIdx {
		if idx < len(row) {
			prev[name] = row[idx]
		} else {
			prev[name] = ""
		}
	}

	newRow, err := applyUpdatesToRow(row, colIdx, len(header), updates, gates, doneVals)
	if err != nil {
		return nil, err
	}
	rows[found] = newRow

	res := &MatrixUpdateResult{
		MatrixName:  matrixName,
		CSVPath:     filepath.Clean(csvPath),
		ProfilePath: filepath.Clean(profilePath),
		MatchColumn: matchKey,
		MatchValue:  matchValue,
		RowIndex:    found,
		Updates:     updates,
		DryRun:      dryRun,
		PreviousRow: prev,
		RowAfter:    matrixRowToMap(header, rows[found]),
	}

	if dryRun {
		return res, nil
	}

	bp, err := maybeBackupCSV(csvPath, writeOpts)
	if err != nil {
		return nil, err
	}
	if bp != "" {
		res.BackupPath = bp
	}
	if err := writeCSVAtomic(csvPath, header, rows); err != nil {
		return nil, err
	}
	res.Wrote = true
	return res, nil
}

func writeCSVAtomic(csvPath string, header []string, rows [][]string) error {
	dir := filepath.Dir(csvPath)
	tmp, err := fileutil.CreateTemp(dir, filepath.Base(csvPath)+".*.tmp")
	if err != nil {
		return errfmt.Newf("matrix update: temp file").Wrap(err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = fileutil.Remove(tmpPath) }()

	w := csv.NewWriter(tmp)
	if err := w.Write(header); err != nil {
		_ = tmp.Close()
		return err
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := fileutil.Rename(tmpPath, csvPath); err != nil {
		return errfmt.Newf("matrix update: replace csv").Wrap(err)
	}
	return nil
}
