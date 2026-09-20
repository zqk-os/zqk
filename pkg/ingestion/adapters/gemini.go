package adapters

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type AgentIngestor interface {
	Ingest(ctx context.Context, dir string) error
}

type GeminiAdapter struct {
	Logger logging.Logger
}

func NewGeminiAdapter(logger logging.Logger) *GeminiAdapter {
	return &GeminiAdapter{Logger: logger}
}

func (a *GeminiAdapter) Ingest(ctx context.Context, dir string) error {
	info, err := fileutil.Stat(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			logging.Fluent(a.Logger).Info("Gemini directory not found, skipping ingestion").Dir(dir).Log()
			return nil
		}
		return err
	}
	if !info.IsDir() {
		logging.Fluent(a.Logger).Warn("Target is not a directory").Dir(dir).Log()
		return nil
	}

	logging.Fluent(a.Logger).Info("Starting Gemini artifact ingestion").Dir(dir).Log()

	engine := NewAgentTranslator()

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".json" || ext == ".yaml" || ext == ".yml" {
			data, readErr := fileutil.ReadFile(path)
			if readErr != nil {
				logging.Fluent(a.Logger).Error("Failed to read agent file", readErr).Path(path).Log()
				return readErr
			}

			format := strings.TrimPrefix(ext, ".")
			obj, transErr := engine.Translate(ctx, data, format, path)
			if transErr != nil {
				logging.Fluent(a.Logger).Error("Failed to translate agent file", transErr).Path(path).Log()
				return transErr
			}
			logging.Fluent(a.Logger).Info("Ingested agent file").Path(path).String(objects.FieldKeyKind, obj[objects.FieldKeyKind].(string)).Log()
		}
		return nil
	})

	if err != nil {
		logging.Fluent(a.Logger).Error("Error during Gemini ingestion", err).Dir(dir).Log()
		return err
	}

	logging.Fluent(a.Logger).Info("Completed Gemini artifact ingestion").Dir(dir).Log()
	return nil
}
