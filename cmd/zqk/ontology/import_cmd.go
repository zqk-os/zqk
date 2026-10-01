package ontology

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/semantic"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/translation"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	internal "github.com/zqk-os/zqk/pkg/zqkcli"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const emptyValue = ""

// NewImportCmd creates the ontology import command from the generated builder
func NewImportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewOntologyImportCommandBuilder()
	cli.BindAsyncProgress(cmd, runOntologyImport)
	return cmd
}

func runOntologyImport(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		filePath, _ := cmd.Flags().GetString("file")
		formatFlag, _ := cmd.Flags().GetString("input-format")

		var err error
		_ = err

		if filePath == emptyValue {
			return errfmt.Errorf("--file is required")
		}

		data, err := fileutil.ReadFile(filePath)
		if err != nil {
			proc.Logger().LogError("Failed to read ontology file", err)
			return errfmt.Newf("failed to read file").Wrap(err)
		}

		formatKey, formatLabel := detectImportFormat(filePath, string(data), formatFlag)
		lines := strings.Count(string(data), "\n") + 1
		if len(data) == 0 {
			lines = 0
		}

		// Create import_tracking record
		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		store := proc.Storage()
		storageCtx := pkgctx.NewStorageContext()
		var nextID string
		listResult, err := store.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindImportTracking, Limit: 1000})
		if err != nil {
			proc.Logger().LogError("Failed to list import_tracking", err)
		} else {
			nextID = internal.NextSequentialID(listResult.Objects, importTrackingIDRe, "IMPTRK-%03d")
			obj := map[string]any{
				objects.FieldKeyID:            nextID,
				objects.FieldKeyKind:          objects.KindImportTracking,
				objects.FieldKeyTitle:         fmt.Sprintf("Import %s", filepath.Base(filePath)),
				objects.FieldKeySourceFile:    filePath,
				objects.FieldKeySourceFormat:  formatKey,
				objects.FieldKeyImportedAt:    zqktime.NowRFC3339UTC(),
				objects.FieldKeyStatus:        objects.ObjectStatusReady,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			}
			opCtx := pkgctx.WithCacheUpdate(ctx, nextID, objects.KindImportTracking, "")
			if err := store.Create(opCtx, secCtx, obj); err != nil {
				proc.Logger().LogError("Failed to create import_tracking", err)
			}
		}

		// Invoke translation engine using the semantic importer.
		importer := semantic.NewOntologyImporter()
		// No need for explicit registration since it's now wired to use pkg/translation internally.
		importResult, trErr := importer.Import(filePath, formatKey, data)

		translationLine := "Translation: not run (no translator for format)\n"
		if trErr != nil {
			logging.FluentEvent(proc.Logger()).Warn("Translation failed (non-fatal)").WithError(trErr).Log()
			translationLine = fmt.Sprintf("Translation: error (%v)\n", trErr)
		} else if importResult != nil {
			translationLine = fmt.Sprintf("Translation: %d objects\n", len(importResult.TranslatedObjects))
			if nextID != emptyValue {
				_ = store.Update(ctx, secCtx, nextID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusTranslated})
			}
			// Persist translated objects via storage (no direct YAML edits).
			if len(importResult.TranslatedObjects) > 0 {
				// Convert back to TranslatedObject slice for existing persistTranslatedObjects helper
				trObjs := make([]translation.TranslatedObject, 0, len(importResult.TranslatedObjects))
				for _, obj := range importResult.TranslatedObjects {
					kind, _ := obj[objects.FieldKeyKind].(string)
					trObjs = append(trObjs, translation.TranslatedObject{Kind: kind, Data: obj})
				}

				persisted := persistTranslatedObjects(ctx, secCtx, store, trObjs, proc)
				if persisted > 0 {
					translationLine += fmt.Sprintf("Persisted: %d object(s)\n", persisted)
				}
			}
		}

		msg := fmt.Sprintf("Format detected: %s\n", formatLabel)
		msg += fmt.Sprintf("File: %d bytes, %d lines\n", len(data), lines)
		msg += translationLine
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}

var (
	importTrackingIDRe = regexp.MustCompile(`^IMPTRK-(\d+)$`)
	domainRegistryIDRe = regexp.MustCompile(`^DOMAIN-REG-(\d+)$`)
)

// persistTranslatedObjects creates translated objects via storage; assigns next ID for placeholders (e.g. DOMAIN-REG-000).
// Returns the number of objects successfully persisted.
func persistTranslatedObjects(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider, translated []translation.TranslatedObject, proc *cli.Processor) int {
	storageCtx := pkgctx.NewStorageContext()
	persisted := 0
	for _, tobj := range translated {
		data := tobj.Data
		if data == nil {
			continue
		}
		kind := tobj.Kind
		if kind == emptyValue {
			kind, _ = data[objects.FieldKeyKind].(string)
		}
		id, _ := data[objects.FieldKeyID].(string)
		// Replace placeholder ID with next available for domain_registry.
		if kind == objects.KindDomainRegistry && (id == emptyValue || id == "DOMAIN-REG-000") {
			listResult, err := store.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindDomainRegistry, Limit: 1000})
			if err != nil {
				logging.FluentEvent(proc.Logger()).Warn("Failed to list domain_registry for next ID").WithError(err).Log()
				continue
			}
			id = internal.NextSequentialID(listResult.Objects, domainRegistryIDRe, "DOMAIN-REG-%03d")
			data[objects.FieldKeyID] = id
		}
		if id == emptyValue {
			logging.FluentEvent(proc.Logger()).Warn("Skipping translated object with no id").
				String(objects.FieldKeyKind, kind).
				Log()
			continue
		}
		data[objects.FieldKeyKind] = kind
		opCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")
		if err := store.Create(opCtx, secCtx, data); err != nil {
			logging.FluentEvent(proc.Logger()).Warn("Failed to persist translated object").
				String(objects.FieldKeyID, id).
				String(objects.FieldKeyKind, kind).
				WithError(err).
				Log()
			continue
		}
		persisted++
	}
	return persisted
}

// detectImportFormat returns format key and human-readable label from path, content, or flag.
// Supports RDF/OWL, Cypher, JSON Schema, and OpenAPI.
func detectImportFormat(path, content, formatFlag string) (key, label string) {
	if formatFlag != emptyValue && formatFlag != "auto" {
		label := formatFlag
		switch formatFlag {
		case "rdf_owl":
			label = "RDF/OWL"
		case "turtle":
			label = "Turtle"
		case "rdf_xml":
			label = "RDF/XML"
		case "jsonld":
			label = "JSON-LD"
		case "cypher":
			label = "Cypher"
		case "json_schema":
			label = "JSON Schema"
		case "openapi":
			label = "OpenAPI"
		}
		return formatFlag, label
	}
	ext := strings.ToLower(filepath.Ext(path))
	trimmed := strings.TrimSpace(content)
	// Cypher: .cypher or schema-like CREATE CONSTRAINT/INDEX
	if ext == ".cypher" || strings.Contains(trimmed, "CREATE CONSTRAINT") || strings.Contains(trimmed, "CREATE INDEX") {
		return "cypher", "Cypher"
	}
	// OpenAPI: openapi: 3.x or swagger: 2.x
	if strings.HasPrefix(trimmed, "openapi:") || strings.HasPrefix(trimmed, "swagger:") ||
		strings.Contains(trimmed, "\nopenapi:") || strings.Contains(trimmed, "\nswagger:") {
		return "openapi", "OpenAPI"
	}
	// JSON Schema: $schema and definitions
	if (ext == ".json" || strings.HasPrefix(trimmed, "{")) && strings.Contains(content, "$schema") && strings.Contains(content, "definitions") {
		return "json_schema", "JSON Schema"
	}
	// RDF/OWL
	switch ext {
	case ".ttl":
		return "turtle", "Turtle"
	case ".owl":
		return "rdf_owl", "RDF/OWL"
	case ".rdf", ".xml":
		return "rdf_xml", "RDF/XML"
	case ".jsonld", ".json":
		if strings.Contains(content, "@context") || strings.Contains(content, "\"@id\"") {
			return "jsonld", "JSON-LD"
		}
		return "jsonld", "JSON-LD"
	default:
		if strings.HasPrefix(trimmed, "@prefix") || strings.Contains(content, " a ") {
			return "turtle", "Turtle"
		}
		return "rdf_owl", "RDF/OWL"
	}
}
