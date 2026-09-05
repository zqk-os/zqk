package semantic

import (
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/translation"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// OntologyTranslator defines the interface to translate imported ontologies into zqk format.
// It returns a slice of maps, where each map represents a ZQK object (usually an object_spec).
type OntologyTranslator interface {
	Translate(data []byte) ([]map[string]any, error)
}

// OntologyImport represents the result of importing an external ontology.
type OntologyImport struct {
	ID                string           `json:"id"`
	Kind              string           `json:"kind"`
	Title             string           `json:"title"`
	SourceFormat      string           `json:"source_format"`
	SourceFile        string           `json:"source_file"`
	TranslatedObjects []map[string]any `json:"translated_objects"`
	Status            string           `json:"status"`
	ImportedAt        time.Time        `json:"imported_at"`
}

// OntologyImporter handles importing external ontologies using the translation subsystem.
type OntologyImporter struct {
	// Importer uses pkg/translation as the engine for format-specific logic.
}

// NewOntologyImporter creates a new OntologyImporter.
func NewOntologyImporter() *OntologyImporter {
	return &OntologyImporter{}
}

// Import translates an ontology data buffer into ZQK objects using the translation engine.
func (oi *OntologyImporter) Import(filePath string, format string, data []byte) (*OntologyImport, error) {
	if len(data) == 0 {
		var err error
		data, err = fileutil.ReadFile(filePath)
		if err != nil {
			return nil, errfmt.Newf("failed to read source file: %s", filePath).Wrap(err)
		}
	}

	// Invoke translation engine (BLI-764)
	trResult, trErr := translation.TranslateByFormat(format, data, &translation.TranslateOptions{SourceFile: filePath})
	if trErr != nil {
		return nil, errfmt.Newf("translation failed for format: %s", format).Wrap(trErr)
	}

	// Extract the data from the TranslatedObject slice
	translated := make([]map[string]any, 0, len(trResult.Objects))
	for _, obj := range trResult.Objects {
		translated = append(translated, obj.Data)
	}

	// Also add specs as translated objects
	translated = append(translated, trResult.Specs...)

	importResult := &OntologyImport{
		ID:                fmt.Sprintf("IMPORT-%d", time.Now().Unix()),
		Kind:              "ontology_import",
		Title:             fmt.Sprintf("Import %s", filePath),
		SourceFormat:      format,
		SourceFile:        filePath,
		TranslatedObjects: translated,
		Status:            objects.ObjectStatusTranslated,
		ImportedAt:        time.Now(),
	}

	return importResult, nil
}
