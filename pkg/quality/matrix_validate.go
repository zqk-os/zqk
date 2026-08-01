package quality

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
)

// MatrixValidateEntry is one matrix's validation outcome.
type MatrixValidateEntry struct {
	Name             string   `json:"name"`
	OK               bool     `json:"ok"`
	CSVPath          string   `json:"csv_path"`
	ProfilePath      string   `json:"profile_path"`
	SessionRefColumn string   `json:"session_ref_column,omitempty"`
	RowCount         int      `json:"row_count"`
	Errors           []string `json:"errors,omitempty"`
}

// MatrixValidateResult is machine output for zqk matrix validate.
type MatrixValidateResult struct {
	RegistryPath string                `json:"registry_path"`
	Entries      []MatrixValidateEntry `json:"entries"`
}

// AllOK returns true if there is at least one entry and every entry passed.
func (r *MatrixValidateResult) AllOK() bool {
	if r == nil || len(r.Entries) == 0 {
		return false
	}
	for _, e := range r.Entries {
		if !e.OK || len(e.Errors) > 0 {
			return false
		}
	}
	return true
}

// ValidateMatrixRegistry checks CSV + profile for one named matrix or all matrices.
// name empty means validate every matrix in the registry (sorted by name).
func ValidateMatrixRegistry(projectRoot, name, registryRel string) (*MatrixValidateResult, error) {
	regRel := registryRel
	if regRel == "" {
		regRel = filepath.Join(paths.DocsQualityDir, "matrix_registry.yaml")
	}
	reg, err := LoadMatrixRegistry(projectRoot, regRel)
	if err != nil {
		return nil, err
	}
	absReg := regRel
	if !filepath.IsAbs(absReg) {
		absReg = filepath.Join(projectRoot, regRel)
	}
	out := &MatrixValidateResult{RegistryPath: filepath.Clean(absReg)}

	names := make([]string, 0, len(reg.Matrices))
	if strings.TrimSpace(name) != "" {
		names = append(names, strings.TrimSpace(name))
	} else {
		for k := range reg.Matrices {
			names = append(names, k)
		}
		sort.Strings(names)
	}

	for _, n := range names {
		e, csvPath, profPath, err := reg.Resolve(projectRoot, n)
		if err != nil {
			out.Entries = append(out.Entries, MatrixValidateEntry{
				Name:        n,
				OK:          false,
				Errors:      []string{err.Error()},
				CSVPath:     "",
				ProfilePath: "",
			})
			continue
		}
		refCol := strings.TrimSpace(e.SessionRefColumn)
		rowCount, errs := validateMatrixFiles(csvPath, profPath, refCol)
		ok := len(errs) == 0
		out.Entries = append(out.Entries, MatrixValidateEntry{
			Name:             n,
			OK:               ok,
			CSVPath:          filepath.Clean(csvPath),
			ProfilePath:      filepath.Clean(profPath),
			SessionRefColumn: refCol,
			RowCount:         rowCount,
			Errors:           errs,
		})
	}

	return out, nil
}

func validateMatrixFiles(csvPath, profilePath, sessionRefColumn string) (rowCount int, errs []string) {
	if st, err := os.Stat(csvPath); err != nil {
		return 0, []string{fmt.Sprintf("csv: %v", err)}
	} else if st.IsDir() {
		return 0, []string{"csv: path is a directory"}
	}

	if _, _, err := LoadMatrixProfileYAML(profilePath); err != nil {
		errs = append(errs, fmt.Sprintf("profile: %v", err))
	}

	f, err := os.Open(csvPath)
	if err != nil {
		return 0, append(errs, fmt.Sprintf("csv open: %v", err))
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return 0, append(errs, fmt.Sprintf("csv header: %v", err))
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	if sessionRefColumn != "" {
		found := false
		for _, h := range header {
			if h == sessionRefColumn {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("csv header missing session_ref_column %q", sessionRefColumn))
		}
	}

	n := 0
	for {
		_, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("csv row %d: %v", n+1, err))
			break
		}
		n++
	}
	return n, errs
}
