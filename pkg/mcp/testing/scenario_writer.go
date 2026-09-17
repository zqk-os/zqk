package testing

import (
	"bytes"
	"io"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ScenarioWriter writes test scenarios to YAML files
type ScenarioWriter struct {
	indent int // YAML indentation (default 2)
}

// NewScenarioWriter creates a new scenario writer
func NewScenarioWriter() *ScenarioWriter {
	return &ScenarioWriter{
		indent: 2, // Default YAML indentation
	}
}

// SetIndent sets the YAML indentation (default 2)
func (sw *ScenarioWriter) Write(scenario *TestScenario, writer io.Writer) error {
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(sw.indent)
	defer encoder.Close()

	if err := encoder.Encode(scenario); err != nil {
		return errfmt.Newf("failed to encode scenario to YAML").Wrap(err)
	}

	return nil
}

// WriteToFile writes a scenario to a YAML file
func (sw *ScenarioWriter) WriteToFile(scenario *TestScenario, filePath string) error {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Open file for writing
	file, err := fileutil.Create(filePath)
	if err != nil {
		return errfmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer file.Close()

	// Write scenario
	if err := sw.Write(scenario, file); err != nil {
		return errfmt.Newf("failed to write scenario to file").Wrap(err)
	}

	return nil
}

// WriteToBytes writes a scenario to a byte slice
func (sw *ScenarioWriter) WriteToBytes(scenario *TestScenario) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(sw.indent)
	defer encoder.Close()

	if err := encoder.Encode(scenario); err != nil {
		return nil, errfmt.Newf("failed to encode scenario to YAML").Wrap(err)
	}

	return buf.Bytes(), nil
}

// WriteString writes a scenario to a string
func (sw *ScenarioWriter) WriteString(scenario *TestScenario) (string, error) {
	data, err := sw.WriteToBytes(scenario)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
