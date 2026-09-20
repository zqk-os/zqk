package yaml

import (
	"bytes"
	"io"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// YAMLWriter is a generic YAML writer for artifacts
type YAMLWriter[T any] struct {
	indent int
}

// NewYAMLWriter creates a new YAML writer
func NewYAMLWriter[T any]() *YAMLWriter[T] {
	return &YAMLWriter[T]{
		indent: 2, // Default YAML indentation
	}
}

// SetIndent sets the YAML indentation (default 2)
func (yw *YAMLWriter[T]) SetIndent(indent int) {
	yw.indent = indent
}

// Write implements core.Writer
func (yw *YAMLWriter[T]) Write(artifact T, writer io.Writer) error {
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(yw.indent)
	defer encoder.Close()

	if err := encoder.Encode(artifact); err != nil {
		return errfmt.Newf("failed to encode to YAML").Wrap(err)
	}

	return nil
}

// WriteToFile implements core.Writer
func (yw *YAMLWriter[T]) WriteToFile(artifact T, filePath string) error {
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

	// Write artifact
	return yw.Write(artifact, file)
}

// WriteToBytes implements core.Writer
func (yw *YAMLWriter[T]) WriteToBytes(artifact T) ([]byte, error) {
	var buf bytes.Buffer
	if err := yw.Write(artifact, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteToString implements core.Writer
func (yw *YAMLWriter[T]) WriteToString(artifact T) (string, error) {
	data, err := yw.WriteToBytes(artifact)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
