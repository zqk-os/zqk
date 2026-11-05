package core

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWriter struct {
	writtenFiles map[string]TestArtifact
	writeErr     error
}

func newMockWriter() *mockWriter {
	return &mockWriter{writtenFiles: make(map[string]TestArtifact)}
}

func (w *mockWriter) Write(artifact TestArtifact, writer io.Writer) error {
	if w.writeErr != nil {
		return w.writeErr
	}
	_, err := writer.Write([]byte(artifact.Name))
	return err
}

func (w *mockWriter) WriteToFile(artifact TestArtifact, filePath string) error {
	if w.writeErr != nil {
		return w.writeErr
	}
	w.writtenFiles[filePath] = artifact
	return nil
}

func (w *mockWriter) WriteToBytes(artifact TestArtifact) ([]byte, error) {
	if w.writeErr != nil {
		return nil, w.writeErr
	}
	return []byte(artifact.Name), nil
}

func (w *mockWriter) WriteToString(artifact TestArtifact) (string, error) {
	if w.writeErr != nil {
		return "", w.writeErr
	}
	return artifact.Name, nil
}

type errBuilderFactory struct{}

func (f *errBuilderFactory) CreateBuilder(spec TestSpec) Builder[TestArtifact] {
	return &errBuilder{}
}

type errBuilder struct{}

func (b *errBuilder) Build() TestArtifact {
	return TestArtifact{Name: "err"}
}

func TestBaseGenerator_LifecycleAndGeneration(t *testing.T) {
	factory := &TestBuilderFactory{}
	writer := newMockWriter()
	constGen := &TestConstantsGenerator{}
	tmpDir := t.TempDir()

	bg := NewBaseGenerator[TestSpec, TestArtifact](factory, writer, tmpDir)
	require.NotNil(t, bg)
	assert.Equal(t, tmpDir, bg.GetOutputDir())

	bgWithConst := NewBaseGeneratorWithConstants[TestSpec, TestArtifact](factory, writer, constGen, tmpDir)
	require.NotNil(t, bgWithConst)
	assert.Equal(t, constGen, bgWithConst.GetConstantsGenerator())

	// Set and get output dir
	bg.SetOutputDir("/custom/dir")
	assert.Equal(t, "/custom/dir", bg.GetOutputDir())
	bg.SetOutputDir(tmpDir)

	// Set and get constants generator
	bg.SetConstantsGenerator(constGen)
	assert.Equal(t, constGen, bg.GetConstantsGenerator())

	// GenerateFromSpec
	art, err := bg.GenerateFromSpec(TestSpec{Name: "item-1"})
	require.NoError(t, err)
	assert.Equal(t, "item-1", art.Name)

	// GenerateFromSpecs
	arts, err := bg.GenerateFromSpecs([]TestSpec{
		{Name: "item-1"},
		{Name: "item-2"},
	})
	require.NoError(t, err)
	require.Len(t, arts, 2)
	assert.Equal(t, "item-1", arts[0].Name)
	assert.Equal(t, "item-2", arts[1].Name)

	// GenerateAndWriteFromSpec with explicit path
	err = bg.GenerateAndWriteFromSpec(TestSpec{Name: "explicit"}, "explicit.yaml")
	require.NoError(t, err)
	assert.Contains(t, writer.writtenFiles, filepath.Join(tmpDir, "explicit.yaml"))

	// GenerateAndWriteFromSpec with empty path (derived from spec name)
	err = bg.GenerateAndWriteFromSpec(TestSpec{Name: "auto-named item"}, "")
	require.NoError(t, err)
	assert.Contains(t, writer.writtenFiles, filepath.Join(tmpDir, "auto_named_item.yaml"))

	// GenerateAndWriteFromSpecs
	err = bg.GenerateAndWriteFromSpecs([]TestSpec{
		{Name: "spec-a"},
		{Name: "spec-b"},
	})
	require.NoError(t, err)

	// GenerateConstants
	c, err := bg.GenerateConstants(TestSpec{Name: "const-spec"})
	require.NoError(t, err)
	assert.Equal(t, "const-spec", c.GetOntology())

	// GenerateConstantsFromSpecs
	cs, err := bg.GenerateConstantsFromSpecs([]TestSpec{{Name: "c1"}, {Name: "c2"}})
	require.NoError(t, err)
	require.Len(t, cs, 2)
}

func TestBaseGenerator_NilConstantsGenerator(t *testing.T) {
	factory := &TestBuilderFactory{}
	writer := newMockWriter()

	bg := NewBaseGenerator[TestSpec, TestArtifact](factory, writer, "")
	_, err := bg.GenerateConstants(TestSpec{Name: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "constants generator not set")

	_, err = bg.GenerateConstantsFromSpecs([]TestSpec{{Name: "test"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "constants generator not set")
}

func TestBaseGenerator_WriteError(t *testing.T) {
	factory := &TestBuilderFactory{}
	writer := newMockWriter()
	writer.writeErr = errors.New("write failure")

	bg := NewBaseGenerator[TestSpec, TestArtifact](factory, writer, "/tmp")
	err := bg.GenerateAndWriteFromSpec(TestSpec{Name: "fail"}, "fail.yaml")
	require.Error(t, err)

	err = bg.GenerateAndWriteFromSpecs([]TestSpec{{Name: "fail"}})
	require.Error(t, err)
}

func TestBaseGenerator_FilenameGenerationAndSanitize(t *testing.T) {
	factory := &TestBuilderFactory{}
	writer := newMockWriter()
	bg := NewBaseGenerator[TestSpec, TestArtifact](factory, writer, "")

	// Test sanitize filename
	assert.Equal(t, "unnamed", SanitizeFilename(""))
	assert.Equal(t, "hello_world", SanitizeFilename("hello world"))
	assert.Equal(t, "foo_bar_1.2", SanitizeFilename("foo--bar__1.2"))
	assert.Equal(t, "test", SanitizeFilename("test___"))

	// Test generateFilename with special cases
	fn1 := bg.generateFilename("Hello World-123", 0)
	assert.Equal(t, "Hello_World_123.yaml", fn1)

	fnEmpty := bg.generateFilename("", 5)
	assert.Equal(t, "spec_5.yaml", fnEmpty)

	fnTrailing := bg.generateFilename("trailing-", 1)
	assert.Equal(t, "trailing.yaml", fnTrailing)
}

func TestMockWriter_AuxiliaryMethods(t *testing.T) {
	writer := newMockWriter()
	art := TestArtifact{Name: "data"}

	var buf strings.Builder
	err := writer.Write(art, &buf)
	require.NoError(t, err)
	assert.Equal(t, "data", buf.String())

	bytes, err := writer.WriteToBytes(art)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), bytes)

	str, err := writer.WriteToString(art)
	require.NoError(t, err)
	assert.Equal(t, "data", str)

	writer.writeErr = errors.New("mock error")
	assert.Error(t, writer.Write(art, &buf))
	_, err = writer.WriteToBytes(art)
	assert.Error(t, err)
	_, err = writer.WriteToString(art)
	assert.Error(t, err)
}
