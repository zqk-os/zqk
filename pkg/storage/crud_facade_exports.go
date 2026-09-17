package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/crud"
	"github.com/lanceman/zqk/pkg/validation"
)

var _ crud.FileStorageReadFacade = (*FileObjectStorage)(nil)
var _ crud.CRUDFacade = (*FileObjectStorage)(nil)

func (f *FileObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return crud.ReadFileObject(f, ctx, secCtx, id)
}

func (f *FileObjectStorage) ApplyKeystoreAccessControl(obj map[string]any, secCtx *pkgctx.SecurityContext) map[string]any {
	return f.applyKeystoreAccessControl(obj, secCtx)
}

func (f *FileObjectStorage) GetParseCacheEntry(hash string) (map[string]any, bool) {
	if cached, ok := GetGlobalParseCache().Get(hash); ok && cached != nil {
		return cached.ToMap(), true
	}
	return nil, false
}

func (f *FileObjectStorage) PutParseCacheEntry(hash string, raw map[string]any) {
	if hash == "" || raw == nil {
		return
	}
	rawClone := make(map[string]any, len(raw))
	for k, v := range raw {
		rawClone[k] = v
	}
	parsed, err := objects.ParseObject(rawClone)
	if err == nil && parsed != nil {
		GetGlobalParseCache().Put(hash, parsed)
	}
}

func (f *FileObjectStorage) CheckPermission(secCtx *pkgctx.SecurityContext, op string, kind string) error {
	return f.checkPermission(secCtx, op, kind)
}

func (f *FileObjectStorage) GetObjectFilePath(id, kind string) (string, error) {
	return f.getObjectFilePath(id, kind)
}

func (f *FileObjectStorage) GetIDValidator() *validation.IDValidator {
	return f.idValidator
}

func (f *FileObjectStorage) ReadObjectFile(ctx context.Context, filePath string) (map[string]any, error) {
	return f.readObjectFile(ctx, filePath)
}

func (f *FileObjectStorage) ReadStreamBacked(id, kind string, secCtx *pkgctx.SecurityContext) (map[string]any, error) {
	return f.readStreamBacked(id, kind, secCtx) // passing context.Background() as a fix for interface
}

func (f *FileObjectStorage) HasWriteBufferContent() bool {
	return f.writeBuf != nil && f.writeBuf.Len() > 0
}

func (f *FileObjectStorage) GetWriteBufferObjectPending(kind, id string) (string, []byte, bool) {
	if f.writeBuf == nil {
		return "", nil, false
	}
	if op := f.writeBuf.GetPending(kind, id); op != nil {
		return op.Op, op.Data, true
	}
	return "", nil, false
}

func (f *FileObjectStorage) StreamStorageEnabledForKind(kind string) bool {
	return StreamStorageEnabledForKind(kind)
}

func (f *FileObjectStorage) RuntimeDeltaEnabledForKind(projectRoot, kind string) bool {
	return RuntimeDeltaEnabledForKind(projectRoot, kind)
}

func (f *FileObjectStorage) ReadRuntimeDeltaCurrentState(projectRoot, kind, id string) map[string]any {
	return ReadRuntimeDeltaCurrentState(projectRoot, kind, id)
}

func (f *FileObjectStorage) IsStreamCurrentPath(projectRoot, filePath string) bool {
	return IsStreamCurrentPath(projectRoot, filePath)
}

func (f *FileObjectStorage) IsObjectDraftPlanePath(projectRoot, filePath string) bool {
	return IsObjectDraftPlanePath(projectRoot, filePath)
}

func (f *FileObjectStorage) StreamPathAndOffset(pathWithOffset string) (string, int64, bool) {
	return StreamPathAndOffset(pathWithOffset)
}

func (f *FileObjectStorage) ReadRecordAt(segmentPath string, offset int64) (map[string]any, error) {
	return ReadRecordAt(segmentPath, offset)
}

func (f *FileObjectStorage) CachedLivePath(filePath string) string {
	return cachedLivePath(filePath)
}

func (f *FileObjectStorage) ApplyRuntimeDeltaOverlay(projectRoot, kind, id string, base map[string]any) map[string]any {
	return crud.ApplyRuntimeDeltaOverlay(f, projectRoot, kind, id, base)
}

func (f *FileObjectStorage) MaterializeCasYAMLMapAfterLoad(projectRoot, kind, logicalIDHint string, base map[string]any) map[string]any {
	return crud.MaterializeCasYAMLMapAfterLoad(f, projectRoot, kind, logicalIDHint, base)
}

func LiveCASBlobUnreadable(err error) bool {
	return crud.LiveCASBlobUnreadable(err)
}

type ListFilter = crud.ListFilter
type QueryResult = crud.QueryResult
type AggregationFunction = crud.AggregationFunction

const (
	AggregationCount = crud.AggregationCount
	AggregationSum   = crud.AggregationSum
	AggregationAvg   = crud.AggregationAvg
	AggregationMin   = crud.AggregationMin
	AggregationMax   = crud.AggregationMax
)

type Aggregation = crud.Aggregation
type AggregateResult = crud.AggregateResult

var CompareValues = crud.CompareValues
var CompareEqual = crud.CompareEqual
var StringContains = crud.StringContains
var StringStartsWith = crud.StringStartsWith
var StringEndsWith = crud.StringEndsWith

type SearchQuery = crud.SearchQuery
type SearchMatch = crud.SearchMatch
type SearchResult = crud.SearchResult

var SortSearchMatches = crud.SortSearchMatches
var ParseSearchQuery = crud.ParseSearchQuery
var HighlightMatch = crud.HighlightMatch

var NormalizeCASIDPrefix = crud.NormalizeCASIDPrefix
var IsTestOrTempProjectRoot = crud.IsTestOrTempProjectRoot
var ValidateKindDirectoryName = crud.ValidateKindDirectoryName
var CalculateSHA256Hash = crud.CalculateSHA256Hash
var VerifyContentHash = crud.VerifyContentHash
var IsHashRegistrySaveQueueFull = crud.IsHashRegistrySaveQueueFull
var VerifyEmbeddedChecksum = crud.VerifyEmbeddedChecksum
var IsExpectedMissingErr = crud.IsExpectedMissingErr
var ErrObjectNotFound = crud.ErrObjectNotFound

var WithBulkCreateDeferFlush = crud.WithBulkCreateDeferFlush
var DeferListingIndexFlushForBulkCreate = crud.DeferListingIndexFlushForBulkCreate
var WithSyncCreateForSchedulerJob = crud.WithSyncCreateForSchedulerJob
var WithSyncCreateForKind = crud.WithSyncCreateForKind

var AgentTaskWorkDoneRequiresCommit = crud.AgentTaskWorkDoneRequiresCommit
var agentTaskWorkDoneRequiresCommit = crud.AgentTaskWorkDoneRequiresCommit
var GetObjectID = crud.GetObjectID
var IsPlanMembershipHardBlockOnCreate = crud.IsPlanMembershipHardBlockOnCreate
var IsPlanMembershipHardBlockError = crud.IsPlanMembershipHardBlockError
var DetectPartialData = crud.DetectPartialData
var NormalizeObjectValues = crud.NormalizeObjectValues

var WithSuppressHashRegistryUpdate = crud.WithSuppressHashRegistryUpdate
var ShouldSuppressHashRegistryUpdate = crud.ShouldSuppressHashRegistryUpdate

var ObjectFieldReferencesID = crud.ObjectFieldReferencesID
var BuildReferenceWithNewID = crud.BuildReferenceWithNewID
var BuildNewReference = crud.BuildNewReference
var ReferenceMatches = crud.ReferenceMatches

type PathWithID = crud.PathWithID

var GetObjectIDAndKindFromPath = crud.GetObjectIDAndKindFromPath
var GetObjectIDFromPath = crud.GetObjectIDFromPath
var IsHex = crud.IsHex
var IsHashBasedFilename = crud.IsHashBasedFilename
var isHex = crud.IsHex
var isHashBasedFilename = crud.IsHashBasedFilename

var ValueInList = crud.ValueInList
var ArrayContains = crud.ArrayContains
var ArrayContainsAll = crud.ArrayContainsAll
var ArrayContainsAny = crud.ArrayContainsAny
var ParseTimestampForFilter = crud.ParseTimestampForFilter
var IsDateSemanticType = crud.IsDateSemanticType
var parseTimestampForFilter = crud.ParseTimestampForFilter
var isDateSemanticType = crud.IsDateSemanticType
var SortParsedObjectsSlice = crud.SortParsedObjectsSlice
var SortObjects = crud.SortObjects
var GroupObjects = crud.GroupObjects
var FlattenGroups = crud.FlattenGroups
var sortObjects = crud.SortObjects
var EffectiveListLimit = crud.EffectiveListLimit
var IdsFromListFilter = crud.IdsFromListFilter
var ListFilterIsOnlyCreatedAtRange = crud.ListFilterIsOnlyCreatedAtRange
var ParseCreatedAtOlderThan = crud.ParseCreatedAtOlderThan
var ParseCreatedAtRangeFromFilters = crud.ParseCreatedAtRangeFromFilters
var LoadStreamDeletedSetFast = crud.LoadStreamDeletedSetFast

type ContentAddressableStorage = crud.ContentAddressableStorage
type IDIndex = crud.IDIndex
type IndexWriteQueue = crud.IndexWriteQueue

var NewContentAddressableStorage = crud.NewContentAddressableStorage
var NewContentAddressableStorageWithIndex = crud.NewContentAddressableStorageWithIndex
var SetSkipIndexUpdateWait = crud.SetSkipIndexUpdateWait
var removeOrphanCASFilesForObjectID = crud.RemoveOrphanCASFilesForObjectID
var removeOrphanCASFilesForObjectIDs = crud.RemoveOrphanCASFilesForObjectIDs
var ComputeUpdatesMap = crud.ComputeUpdatesMap
var EffectiveUpdateFieldsForClassification = crud.EffectiveUpdateFieldsForClassification
var ClassifyUpdateMutation = crud.ClassifyUpdateMutation
var ShockwaveRouterIsArmed = crud.ShockwaveRouterIsArmed
var computeUpdatesMap = crud.ComputeUpdatesMap
var effectiveUpdateFieldsForClassification = crud.EffectiveUpdateFieldsForClassification
var classifyUpdateMutation = crud.ClassifyUpdateMutation
var shockwaveRouterIsArmed = crud.ShockwaveRouterIsArmed
