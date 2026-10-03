package quality

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type matrixCSVReader struct {
	file   *os.File
	reader *csv.Reader
	header []string
	colIdx map[string]int
}

func openMatrixCSV(csvPath string) (*matrixCSVReader, error) {
	f, err := fileutil.Open(csvPath)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	colIdx := make(map[string]int, len(header))
	for i, h := range header {
		trimmed := strings.TrimSpace(h)
		header[i] = trimmed
		colIdx[trimmed] = i
	}
	return &matrixCSVReader{
		file:   f,
		reader: r,
		header: header,
		colIdx: colIdx,
	}, nil
}

func (m *matrixCSVReader) Close() error {
	return m.file.Close()
}

func (m *matrixCSVReader) hasCol(col string) bool {
	if m == nil || m.colIdx == nil {
		return false
	}
	_, ok := m.colIdx[strings.TrimSpace(col)]
	return ok
}

func prepareSetUpdates(setPairs []string, valueMap map[string]string) (map[string]string, error) {
	updates, err := parseSetFlags(setPairs)
	if err != nil {
		return nil, err
	}
	ApplyValueMapToUpdates(updates, valueMap)
	return updates, nil
}

type matrixCSVUpdateContext struct {
	cr       *matrixCSVReader
	prof     *MatrixProfileYAML
	doneVals map[string]struct{}
	gates    map[string]struct{}
}

func openMatrixForUpdate(csvPath, profilePath string) (*matrixCSVUpdateContext, error) {
	prof, doneVals, err := LoadMatrixProfileYAML(profilePath)
	if err != nil {
		return nil, err
	}
	cr, err := openMatrixCSV(csvPath)
	if err != nil {
		return nil, err
	}
	return &matrixCSVUpdateContext{
		cr:       cr,
		prof:     prof,
		doneVals: doneVals,
		gates:    gateColumnSet(prof),
	}, nil
}

func commitMatrixCSVUpdate(csvPath string, writeOpts *MatrixWriteOpts, header []string, rows [][]string, dryRun bool) (string, bool, error) {
	if dryRun {
		return "", false, nil
	}
	bp, err := maybeBackupCSV(csvPath, writeOpts)
	if err != nil {
		return "", false, err
	}
	if err := writeCSVAtomic(csvPath, header, rows); err != nil {
		return "", false, err
	}
	return bp, true, nil
}

func resolveAndLoadRegistry(projectRoot, registryRel string) (*MatrixRegistry, string, error) {
	if registryRel == "" {
		registryRel = filepath.Join(paths.DocsQualityDir, "matrix_registry.yaml")
	}
	reg, err := LoadMatrixRegistry(projectRoot, registryRel)
	if err != nil {
		return nil, "", err
	}
	absReg := registryRel
	if !filepath.IsAbs(absReg) {
		absReg = filepath.Join(projectRoot, registryRel)
	}
	return reg, filepath.Clean(absReg), nil
}
