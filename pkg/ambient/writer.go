package ambient

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ArtifactWriter listens to EventTypeFilesystem events from the EventHub
// and scaffolds files based on predictions from the ast_analyzer.
type ArtifactWriter struct {
	hub EventHub
}

// NewArtifactWriter creates a new ArtifactWriter and subscribes it to the EventHub.
func NewArtifactWriter(hub EventHub) *ArtifactWriter {
	aw := &ArtifactWriter{hub: hub}
	hub.Subscribe(EventTypeFilesystem, aw.handleEvent)
	return aw
}

func (aw *ArtifactWriter) handleEvent(ctx context.Context, event Event) error {
	payloadMap, ok := event.Payload.(map[string]any)
	if !ok {
		return nil
	}

	source, ok := payloadMap[objects.FieldKeySource].(string)
	if !ok || source != "ast_analyzer" {
		return nil
	}

	predictions, ok := payloadMap[objects.FieldKeyPredictions].([]Prediction)
	if !ok {
		return nil
	}

	for _, p := range predictions {
		if p.Type == "missing_test" {
			if err := aw.writeTestFile(p.FilePath); err != nil {
				// Ignore write errors to keep the pipeline moving silently.
				continue
			}
		}
	}
	return nil
}

func (aw *ArtifactWriter) writeTestFile(filePath string) error {
	// Only write if the file does not exist
	if _, err := fileutil.Stat(filePath); err == nil {
		return nil
	}

	dir := filepath.Dir(filePath)
	if err := fileutil.EnsureDir(dir); err != nil {
		return err
	}

	dirName := filepath.Base(dir)
	pkgName := dirName
	if pkgName == "." || pkgName == "/" {
		pkgName = "main"
	}

	// Sanitize package name slightly to ensure basic validity
	pkgName = strings.ReplaceAll(pkgName, "-", "_")

	content := fmt.Sprintf("package %s\n\nimport \"testing\"\n", pkgName)

	return fileutil.WriteSecureFile(filePath, []byte(content))
}
