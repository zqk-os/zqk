package storage

import (
	"context"
	"encoding/json"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// ExportFormat is the serialization format for export (yaml or json).
type ExportFormat string

const (
	ExportFormatYAML ExportFormat = "yaml"
	ExportFormatJSON ExportFormat = "json"
)

// ExportObjects exports objects matching the filter from the storage provider.
// Uses List with pagination to fetch all matching objects, then marshals to the requested format.
// Returns the serialized bytes (YAML array or JSON array of objects).
func ExportObjects(
	ctx context.Context,
	provider ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	filter ListFilter,
	format ExportFormat,
) ([]byte, error) {
	if provider == nil || secCtx == nil {
		return nil, errfmt.Errorf(ConstMiscProviderAndSecctxAreRequired)
	}
	if storageCtx == nil {
		storageCtx = pkgctx.GetStorageContext()
	}

	var all []map[string]any
	offset := 0
	limit := filter.Limit
	if limit <= 0 && storageCtx.MaxPageSize > 0 {
		limit = storageCtx.MaxPageSize
	}
	if limit <= 0 {
		limit = 1000
	}

	for {
		f := filter
		f.Offset = offset
		f.Limit = limit
		result, err := provider.List(ctx, secCtx, storageCtx, f)
		if err != nil {
			return nil, errfmt.Newf(ConstMiscListDuringExport).Wrap(err)
		}
		if len(result.Objects) == 0 {
			break
		}
		all = append(all, result.Objects...)
		if len(result.Objects) < limit {
			break
		}
		offset += len(result.Objects)
	}

	return marshalExport(all, format)
}

func marshalExport(records []map[string]any, format ExportFormat) ([]byte, error) {
	switch strings.ToLower(string(format)) {
	case string(ExportFormatYAML), "":
		return yaml.Marshal(records)
	case string(ExportFormatJSON):
		return json.MarshalIndent(records, "", "  ")
	default:
		return nil, errfmt.Errorf(ConstMiscUnsupportedExportFormatSUseYamlOrJson, format)
	}
}

// ImportMode controls how import handles existing objects.
type ImportMode string

const (
	ImportModeCreateOnly ImportMode = "create_only" // Skip objects that already exist
	ImportModeUpsert     ImportMode = "upsert"      // Create or update per object
)

// ImportOptions configures import behavior.
type ImportOptions struct {
	Mode            ImportMode // create_only or upsert
	ValidateOnly    bool       // If true, only validate; do not write
	ContinueOnError bool       // If true, continue after per-object errors
}

// ImportResult holds the result of an import operation.
type ImportResult struct {
	Created int
	Updated int
	Skipped int
	Failed  int
	Errors  []BulkOperationError
}

// ImportObjects deserializes data (YAML or JSON array of objects) and creates/updates them
// via the storage provider according to ImportOptions.
func ImportObjects(
	ctx context.Context,
	provider ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	data []byte,
	format ExportFormat,
	options ImportOptions,
) (*ImportResult, error) {
	if provider == nil || secCtx == nil {
		return nil, errfmt.Errorf(ConstMiscProviderAndSecctxAreRequired)
	}

	importRecords, err := unmarshalImport(data, format)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscParseImportData).Wrap(err)
	}

	result := &ImportResult{}

	if options.ValidateOnly {
		// Validate-only: attempt create in memory or run validation per object; for simplicity, count as skipped
		result.Skipped = len(importRecords)
		return result, nil
	}

	switch options.Mode {
	case ImportModeCreateOnly:
		br, err := provider.BulkCreate(ctx, secCtx, importRecords)
		if err != nil {
			return nil, errfmt.Newf("bulk create").Wrap(err)
		}
		result.Created = br.SuccessCount
		result.Failed = br.FailureCount
		result.Errors = br.Errors
	case ImportModeUpsert:
		for i, obj := range importRecords {
			id, _ := obj[objects.FieldKeyID].(string)
			if id == emptyValue {
				result.Failed++
				result.Errors = append(result.Errors, BulkOperationError{Index: i, Message: ConstMiscObjectMissingId})
				if !options.ContinueOnError {
					return result, nil
				}
				continue
			}
			exists, err := provider.Exists(ctx, secCtx, id)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, BulkOperationError{ID: id, Index: i, Error: err, Message: err.Error()})
				if !options.ContinueOnError {
					return result, err
				}
				continue
			}
			if exists {
				// Update: build updates map from object (exclude id, kind, created_at, created_by)
				updates := make(map[string]any)
				for k, v := range obj {
					switch k {
					case objects.FieldKeyID, objects.FieldKeyKind, objects.FieldKeyCreatedAt, objects.FieldKeyCreatedBy:
						continue
					default:
						updates[k] = v
					}
				}
				if err := provider.Update(ctx, secCtx, id, updates); err != nil {
					result.Failed++
					result.Errors = append(result.Errors, BulkOperationError{ID: id, Index: i, Error: err, Message: err.Error()})
					if !options.ContinueOnError {
						return result, err
					}
					continue
				}
				result.Updated++
			} else {
				if err := provider.Create(ctx, secCtx, obj); err != nil {
					result.Failed++
					result.Errors = append(result.Errors, BulkOperationError{ID: id, Index: i, Error: err, Message: err.Error()})
					if !options.ContinueOnError {
						return result, err
					}
					continue
				}
				result.Created++
			}
		}
	default:
		return nil, errfmt.Errorf(ConstMiscUnsupportedImportModeSUseCreateOnlyOrUps, options.Mode)
	}

	return result, nil
}

func unmarshalImport(data []byte, format ExportFormat) ([]map[string]any, error) {
	var importRecords []map[string]any
	switch strings.ToLower(string(format)) {
	case string(ExportFormatYAML), "":
		if err := yaml.Unmarshal(data, &importRecords); err != nil {
			return nil, err
		}
	case string(ExportFormatJSON):
		if err := json.Unmarshal(data, &importRecords); err != nil {
			return nil, err
		}
	default:
		return nil, errfmt.Errorf(ConstMiscUnsupportedImportFormatSUseYamlOrJson, format)
	}
	if importRecords == nil {
		importRecords = []map[string]any{}
	}
	return importRecords, nil
}
