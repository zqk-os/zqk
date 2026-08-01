package processing

import (
	"context"
	"errors"
	"maps"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
)

const (
	opTypeCreate                = "create"
	opTypeUpdate                = "update"
	opTypeDelete                = "delete"
	opTypeGet                   = "get"
	opTypeList                  = "list"
	statusSuccess               = "success"
	statusError                 = "error"
	statusWouldExecute          = "would_execute"
	errProcessExpandTemplateFmt = "failed to expand template: %w"
	errUnsupportedFmt           = "unsupported reference file format: %s"
	errUpdateNeedsID            = "update operation requires id"
	errDeleteNeedsID            = "delete operation requires id"
	errGetNeedsID               = "get operation requires id"
	errUnknownOpFmt             = "unknown operation type: %s"
	errOpFailedFmt              = "operation %d failed: %w"
	errBulkCreateFmt            = "bulk create failed: %w"
	objectIDKey                 = "id"
	emptyValue                  = ""
)

// Processor processes reference files and executes operations
type Processor struct {
	storageProvider storage.ObjectStorageProvider
	profile         string
}

// NewProcessor creates a new reference file processor
func NewProcessor(storageProvider storage.ObjectStorageProvider, profile string) *Processor {
	return &Processor{
		storageProvider: storageProvider,
		profile:         profile,
	}
}

// ProcessReferenceFile processes a reference file and executes all operations
func (p *Processor) ProcessReferenceFile(ctx context.Context, refFile *ReferenceFile, dryRun bool) (*ProcessingResult, error) {
	logger := logging.GetLoggerFromProfile(p.profile)

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	switch refFile.Format {
	case formatOperations:
		return p.processOperations(ctx, refFile.Operations, secCtx, storageCtx, dryRun, logger)
	case formatData:
		return p.processData(ctx, refFile.Data, secCtx, storageCtx, dryRun, logger)
	case formatTemplate:
		// Expand template first
		expanded, err := refFile.Template.ExpandTemplate()
		if err != nil {
			return nil, errfmt.Errorf(errProcessExpandTemplateFmt, err)
		}
		return p.processData(ctx, expanded, secCtx, storageCtx, dryRun, logger)
	default:
		return nil, errfmt.Errorf(errUnsupportedFmt, refFile.Format)
	}
}

// processOperations processes a list of operations
func (p *Processor) processOperations(ctx context.Context, operations []Operation, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, dryRun bool, logger logging.Logger) (*ProcessingResult, error) {
	result := &ProcessingResult{
		Operations: make([]OperationResult, 0),
	}

	for i, op := range operations {
		opResult := OperationResult{
			Index:       i,
			Type:        op.Type,
			Description: op.Description,
		}

		if dryRun {
			opResult.Status = statusWouldExecute
			result.Operations = append(result.Operations, opResult)
			logging.Fluent(logger).Info("Would execute operation").
				Int("index", i).
				String("type", op.Type).
				String("description", op.Description).
				Log()
			continue
		}

		var err error
		switch op.Type {
		case opTypeCreate:
			err = p.storageProvider.Create(ctx, secCtx, op.Object)
			if err == nil {
				opResult.Status = statusSuccess
				if id, ok := op.Object[objectIDKey].(string); ok {
					opResult.ObjectID = id
				}
			}
		case opTypeUpdate:
			if op.ID == emptyValue {
				err = errors.New(errUpdateNeedsID)
			} else {
				// Read existing object
				existing, readErr := p.storageProvider.Read(ctx, secCtx, op.ID)
				if readErr != nil {
					err = readErr
				} else {
					// Merge updates
					merged := mergeMaps(existing, op.Updates)
					err = p.storageProvider.Update(ctx, secCtx, op.ID, merged)
					if err == nil {
						opResult.Status = statusSuccess
						opResult.ObjectID = op.ID
					}
				}
			}
		case opTypeDelete:
			if op.ID == emptyValue {
				err = errors.New(errDeleteNeedsID)
			} else {
				err = p.storageProvider.Delete(ctx, secCtx, op.ID, false)
				if err == nil {
					opResult.Status = statusSuccess
					opResult.ObjectID = op.ID
				}
			}
		case opTypeGet:
			if op.ID == emptyValue {
				err = errors.New(errGetNeedsID)
			} else {
				_, err = p.storageProvider.Read(ctx, secCtx, op.ID)
				if err == nil {
					opResult.Status = statusSuccess
					opResult.ObjectID = op.ID
				}
			}
		case opTypeList:
			filter := storage.ListFilter{
				Kind:    op.Kind,
				Filters: op.Filter,
			}
			_, err = p.storageProvider.List(ctx, secCtx, storageCtx, filter)
			if err == nil {
				opResult.Status = statusSuccess
			}
		default:
			err = errfmt.Errorf(errUnknownOpFmt, op.Type)
		}

		if err != nil {
			opResult.Status = statusError
			opResult.Error = err.Error()
			result.FailureCount++

			if op.ContinueOnError {
				logging.Fluent(logger).Warn("Operation failed but continuing").
					Int("index", i).
					String("type", op.Type).
					WithError(err).
					Log()
			} else {
				logging.Fluent(logger).Error("Operation failed", err).
					Int("index", i).
					String("type", op.Type).
					Log()
				result.Operations = append(result.Operations, opResult)
				return result, errfmt.Errorf(errOpFailedFmt, i, err)
			}
		} else {
			result.SuccessCount++
		}

		result.Operations = append(result.Operations, opResult)
	}

	return result, nil
}

// processData processes a list of data items (assumes all are creates)
func (p *Processor) processData(ctx context.Context, data []map[string]any, secCtx *pkgctx.SecurityContext, _ *pkgctx.StorageContext, dryRun bool, _ logging.Logger) (*ProcessingResult, error) {
	if dryRun {
		result := &ProcessingResult{
			Operations: make([]OperationResult, 0),
		}
		for i := range data {
			result.Operations = append(result.Operations, OperationResult{
				Index:  i,
				Type:   opTypeCreate,
				Status: statusWouldExecute,
			})
		}
		result.SuccessCount = len(data)
		return result, nil
	}

	// Use bulk create for efficiency
	bulkResult, err := p.storageProvider.BulkCreate(ctx, secCtx, data)
	if err != nil {
		return nil, errfmt.Errorf(errBulkCreateFmt, err)
	}

	result := &ProcessingResult{
		SuccessCount: bulkResult.SuccessCount,
		FailureCount: bulkResult.FailureCount,
		Operations:   make([]OperationResult, 0),
	}

	// Convert bulk result to operation results
	for i, obj := range bulkResult.Results {
		result.Operations = append(result.Operations, OperationResult{
			Index:    i,
			Type:     opTypeCreate,
			Status:   statusSuccess,
			ObjectID: getString(obj, objectIDKey),
		})
	}

	for _, bulkErr := range bulkResult.Errors {
		result.Operations = append(result.Operations, OperationResult{
			Index:    bulkErr.Index,
			Type:     opTypeCreate,
			Status:   statusError,
			Error:    bulkErr.Message,
			ObjectID: bulkErr.ID,
		})
	}

	return result, nil
}

// ProcessingResult contains the results of processing a reference file
type ProcessingResult struct {
	SuccessCount int
	FailureCount int
	Operations   []OperationResult
}

// OperationResult represents the result of a single operation
type OperationResult struct {
	Index       int
	Type        string
	Status      string // "success", "error", "would_execute"
	ObjectID    string
	Description string
	Error       string
}

// Helper functions
func mergeMaps(base, updates map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy base using maps.Copy
	maps.Copy(result, base)

	// Apply updates
	maps.Copy(result, updates)

	return result
}

func getString(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return emptyValue
}
