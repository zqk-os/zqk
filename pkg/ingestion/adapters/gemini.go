package adapters

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
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
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Fluent(a.Logger).Info("Gemini directory not found, skipping ingestion").String("dir", dir).Log()
			return nil
		}
		return err
	}
	if !info.IsDir() {
		logging.Fluent(a.Logger).Warn("Target is not a directory").String("dir", dir).Log()
		return nil
	}

	logging.Fluent(a.Logger).Info("Starting Gemini artifact ingestion").String("dir", dir).Log()

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
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				logging.Fluent(a.Logger).Error("Failed to read agent file", readErr).String("path", path).Log()
				return readErr
			}

			format := strings.TrimPrefix(ext, ".")
			obj, transErr := engine.Translate(ctx, data, format, path)
			if transErr != nil {
				logging.Fluent(a.Logger).Error("Failed to translate agent file", transErr).String("path", path).Log()
				return transErr
			}
			logging.Fluent(a.Logger).Info("Ingested agent file").String("path", path).String(objects.FieldKeyKind, obj[objects.FieldKeyKind].(string)).Log()
		}
		return nil
	})

	if err != nil {
		logging.Fluent(a.Logger).Error("Error during Gemini ingestion", err).String("dir", dir).Log()
		return err
	}

	logging.Fluent(a.Logger).Info("Completed Gemini artifact ingestion").String("dir", dir).Log()
	return nil
}
