package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	ErrInvalidCompactionConfig = errors.New("invalid compaction configuration")
	ErrInvalidStoragePage      = errors.New("invalid storage page")
	ErrNonMonotonicGeneration  = errors.New("non-monotonic generation sequence")
	ErrPageSizeOutOfBounds     = errors.New("storage page payload size out of bounds")
	ErrObjectNotFound          = errors.New("cas object not found")
	ErrChecksumMismatch        = errors.New("cas object sha256 checksum mismatch")
	ErrWriteTxNotFound         = errors.New("write transaction not found")
	ErrWriteTxNotActive        = errors.New("write transaction is no longer active")
)

// CompactionConfig defines invariant storage bounds and retention policy for generational compaction.
type CompactionConfig struct {
	MinPageSize       int
	MaxPageSize       int
	RetainGenerations uint64
}

// Validate checks that page size bounds are positive and well-ordered.
func (c CompactionConfig) Validate() error {
	if c.MinPageSize <= 0 {
		return fmt.Errorf("%w: min_page_size must be > 0, got %d", ErrInvalidCompactionConfig, c.MinPageSize)
	}
	if c.MaxPageSize < c.MinPageSize {
		return fmt.Errorf("%w: max_page_size (%d) must be >= min_page_size (%d)", ErrInvalidCompactionConfig, c.MaxPageSize, c.MinPageSize)
	}
	return nil
}

// StoragePage represents an immutable generational CAS storage page.
type StoragePage struct {
	PageID     string
	Bucket     string
	ObjectKey  string
	Generation uint64
	Payload    []byte
	Checksum   string
	Superseded bool
	CreatedAt  time.Time
}

// GenerationSnapshot captures a point-in-time view of active pages and bucket hash indexes.
type GenerationSnapshot struct {
	Generation    uint64
	ActivePages   int
	TotalBytes    int64
	BucketIndexes map[string]map[string]string
	CapturedAt    time.Time
}

// CompactionReport summarizes the outcome of a generational compaction and verification cycle.
type CompactionReport struct {
	ActiveGeneration   uint64
	CompactedPageCount int
	ReclaimedPageCount int
	ReclaimedBytes     int64
	PinnedPageCount    int
	VerifiedChecksums  int
	BucketIndexCount   int
	CompletedAt        time.Time
}

type writeTxState struct {
	id          string
	active      bool
	stagedPages []StoragePage
	pinnedKeys  map[string]struct{}
	pinnedGens  map[uint64]struct{}
}

// CASCompactor manages generational CAS storage pages, SHA-256 bucket indexing,
// compaction reclamation, and uncommitted transaction write shielding.
type CASCompactor struct {
	mu               sync.RWMutex
	cfg              CompactionConfig
	activeGeneration uint64
	pages            map[string]*StoragePage
	activeByKey      map[string]string
	bucketIndex      map[string]map[string]string
	writeTxs         map[string]*writeTxState
}

// NewCASCompactor constructs a validated CASCompactor.
func NewCASCompactor(cfg CompactionConfig) (*CASCompactor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &CASCompactor{
		cfg:         cfg,
		pages:       make(map[string]*StoragePage),
		activeByKey: make(map[string]string),
		bucketIndex: make(map[string]map[string]string),
		writeTxs:    make(map[string]*writeTxState),
	}, nil
}

func computeSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func compositeObjectKey(bucket, objectKey string) string {
	return bucket + "\x00" + objectKey
}

func validateBucketAndObjectKey(bucket, objectKey string) error {
	if err := strutil.ErrorOnBlank(bucket, "%w: bucket cannot be blank", ErrInvalidStoragePage); err != nil {
		return err
	}
	return strutil.ErrorOnBlank(objectKey, "%w: object_key cannot be blank", ErrInvalidStoragePage)
}

func (c *CASCompactor) validateAndNormalizePage(page StoragePage) (StoragePage, error) {
	if err := strutil.ErrorOnBlank(page.PageID, "%w: page_id cannot be blank", ErrInvalidStoragePage); err != nil {
		return StoragePage{}, err
	}
	if err := validateBucketAndObjectKey(page.Bucket, page.ObjectKey); err != nil {
		return StoragePage{}, err
	}
	if len(page.Payload) == 0 {
		return StoragePage{}, fmt.Errorf("%w: payload cannot be empty", ErrInvalidStoragePage)
	}
	if len(page.Payload) < c.cfg.MinPageSize || len(page.Payload) > c.cfg.MaxPageSize {
		return StoragePage{}, fmt.Errorf("%w: payload size %d outside [%d, %d]", ErrPageSizeOutOfBounds, len(page.Payload), c.cfg.MinPageSize, c.cfg.MaxPageSize)
	}
	if page.Generation == 0 {
		return StoragePage{}, fmt.Errorf("%w: generation must be >= 1", ErrNonMonotonicGeneration)
	}

	computed := computeSHA256Hex(page.Payload)
	if page.Checksum != "" && page.Checksum != computed {
		return StoragePage{}, fmt.Errorf("%w: provided %s != computed %s", ErrChecksumMismatch, page.Checksum, computed)
	}

	normalized := page
	normalized.Payload = append([]byte(nil), page.Payload...)
	normalized.Checksum = computed
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC()
	}
	return normalized, nil
}

func (c *CASCompactor) commitPageLocked(page StoragePage) error {
	if _, exists := c.pages[page.PageID]; exists {
		return fmt.Errorf("%w: duplicate page_id %q", ErrInvalidStoragePage, page.PageID)
	}

	key := compositeObjectKey(page.Bucket, page.ObjectKey)
	if prevID, hasPrev := c.activeByKey[key]; hasPrev {
		if prevPage, ok := c.pages[prevID]; ok {
			prevPage.Superseded = true
		}
	}

	stored := page
	c.pages[stored.PageID] = &stored
	c.activeByKey[key] = stored.PageID

	if c.bucketIndex[stored.Bucket] == nil {
		c.bucketIndex[stored.Bucket] = make(map[string]string)
	}
	c.bucketIndex[stored.Bucket][stored.ObjectKey] = stored.Checksum
	return nil
}

// AppendGeneration validates and commits a batch of pages for a strictly monotonic generation.
func (c *CASCompactor) AppendGeneration(generation uint64, pages []StoragePage) error {
	if len(pages) == 0 {
		return fmt.Errorf("%w: generation batch cannot be empty", ErrInvalidStoragePage)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if generation <= c.activeGeneration {
		return fmt.Errorf("%w: generation %d must be > active generation %d", ErrNonMonotonicGeneration, generation, c.activeGeneration)
	}

	normalized := make([]StoragePage, 0, len(pages))
	seenIDs := make(map[string]struct{}, len(pages))
	for _, p := range pages {
		if p.Generation != generation {
			return fmt.Errorf("%w: page %q generation %d != batch generation %d", ErrNonMonotonicGeneration, p.PageID, p.Generation, generation)
		}
		norm, err := c.validateAndNormalizePage(p)
		if err != nil {
			return err
		}
		if _, dup := seenIDs[norm.PageID]; dup {
			return fmt.Errorf("%w: duplicate page_id %q in batch", ErrInvalidStoragePage, norm.PageID)
		}
		seenIDs[norm.PageID] = struct{}{}
		normalized = append(normalized, norm)
	}

	for _, norm := range normalized {
		if err := c.commitPageLocked(norm); err != nil {
			return err
		}
	}
	c.activeGeneration = generation
	return nil
}

// PutPage validates and commits a single storage page with strictly monotonic generation ordering.
func (c *CASCompactor) PutPage(page StoragePage) error {
	return c.AppendGeneration(page.Generation, []StoragePage{page})
}

// BeginWriteTx starts an uncommitted write transaction that shields staged pages and pinned keys from compaction.
func (c *CASCompactor) BeginWriteTx(txID string) error {
	if err := strutil.ErrorOnBlank(txID, "%w: tx_id cannot be blank", ErrWriteTxNotFound); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.writeTxs[txID]; exists {
		return fmt.Errorf("%w: tx %q already exists", ErrWriteTxNotActive, txID)
	}
	c.writeTxs[txID] = &writeTxState{
		id:         txID,
		active:     true,
		pinnedKeys: make(map[string]struct{}),
		pinnedGens: make(map[uint64]struct{}),
	}
	return nil
}

// StageWritePage validates and stages a storage page inside an active write transaction,
// pinning its object key and generation against concurrent compaction.
func (c *CASCompactor) StageWritePage(txID string, page StoragePage) error {
	if err := strutil.ErrorOnBlank(txID, "%w: tx_id cannot be blank", ErrWriteTxNotFound); err != nil {
		return err
	}

	norm, err := c.validateAndNormalizePage(page)
	if err != nil {
		return err
	}

	return c.withActiveTx(txID, func(tx *writeTxState) error {
		if norm.Generation <= c.activeGeneration {
			return fmt.Errorf("%w: staged generation %d must be > active generation %d", ErrNonMonotonicGeneration, norm.Generation, c.activeGeneration)
		}
		tx.stagedPages = append(tx.stagedPages, norm)
		tx.pinnedKeys[compositeObjectKey(norm.Bucket, norm.ObjectKey)] = struct{}{}
		tx.pinnedGens[norm.Generation] = struct{}{}
		return nil
	})
}

func (c *CASCompactor) withActiveTx(txID string, fn func(tx *writeTxState) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	tx, exists := c.writeTxs[txID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrWriteTxNotFound, txID)
	}
	if !tx.active {
		return fmt.Errorf("%w: %s", ErrWriteTxNotActive, txID)
	}
	return fn(tx)
}

// CommitWriteTx atomically commits all staged pages in the transaction and releases its compaction pins.
func (c *CASCompactor) CommitWriteTx(txID string) error {
	return c.withActiveTx(txID, func(tx *writeTxState) error {
		var maxGen uint64
		for _, p := range tx.stagedPages {
			if p.Generation <= c.activeGeneration {
				return fmt.Errorf("%w: commit generation %d <= active generation %d", ErrNonMonotonicGeneration, p.Generation, c.activeGeneration)
			}
			if p.Generation > maxGen {
				maxGen = p.Generation
			}
		}

		for _, p := range tx.stagedPages {
			if err := c.commitPageLocked(p); err != nil {
				return err
			}
		}
		if maxGen > c.activeGeneration {
			c.activeGeneration = maxGen
		}
		tx.active = false
		return nil
	})
}

// AbortWriteTx discards staged uncommitted writes and releases all compaction pins held by the transaction.
func (c *CASCompactor) AbortWriteTx(txID string) error {
	return c.withActiveTx(txID, func(tx *writeTxState) error {
		tx.stagedPages = nil
		tx.pinnedKeys = nil
		tx.pinnedGens = nil
		tx.active = false
		return nil
	})
}

func (c *CASCompactor) isPagePinnedLocked(page *StoragePage) bool {
	key := compositeObjectKey(page.Bucket, page.ObjectKey)
	for _, tx := range c.writeTxs {
		if !tx.active {
			continue
		}
		if _, pinnedKey := tx.pinnedKeys[key]; pinnedKey {
			return true
		}
		if _, pinnedGen := tx.pinnedGens[page.Generation]; pinnedGen {
			return true
		}
	}
	return false
}

func (c *CASCompactor) isReclaimableLocked(page *StoragePage) bool {
	if !page.Superseded {
		return false
	}
	if c.isPagePinnedLocked(page) {
		return false
	}
	if c.cfg.RetainGenerations == 0 {
		return true
	}
	if c.activeGeneration <= c.cfg.RetainGenerations {
		return false
	}
	cutoff := c.activeGeneration - c.cfg.RetainGenerations
	return page.Generation <= cutoff
}

// Compact reclaims unpinned superseded generation pages, rebuilds SHA-256 bucket indexes,
// and verifies zero-loss cryptographic integrity of all retained pages.
func (c *CASCompactor) Compact() (CompactionReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var reclaimedCount int
	var reclaimedBytes int64
	var pinnedCount int

	for id, page := range c.pages {
		if page.Superseded && c.isPagePinnedLocked(page) {
			pinnedCount++
			continue
		}
		if c.isReclaimableLocked(page) {
			reclaimedCount++
			reclaimedBytes += int64(len(page.Payload))
			delete(c.pages, id)
		}
	}

	rebuiltIndex := make(map[string]map[string]string)
	verifiedCount := 0
	indexCount := 0
	for _, page := range c.pages {
		if computeSHA256Hex(page.Payload) != page.Checksum {
			return CompactionReport{}, fmt.Errorf("%w: page %q failed integrity check during compaction", ErrChecksumMismatch, page.PageID)
		}
		verifiedCount++
		if !page.Superseded {
			if rebuiltIndex[page.Bucket] == nil {
				rebuiltIndex[page.Bucket] = make(map[string]string)
			}
			rebuiltIndex[page.Bucket][page.ObjectKey] = page.Checksum
			indexCount++
		}
	}
	c.bucketIndex = rebuiltIndex

	return CompactionReport{
		ActiveGeneration:   c.activeGeneration,
		CompactedPageCount: len(c.pages),
		ReclaimedPageCount: reclaimedCount,
		ReclaimedBytes:     reclaimedBytes,
		PinnedPageCount:    pinnedCount,
		VerifiedChecksums:  verifiedCount,
		BucketIndexCount:   indexCount,
		CompletedAt:        time.Now().UTC(),
	}, nil
}

func (c *CASCompactor) lookupVerifiedActivePageLocked(bucket, objectKey string) (*StoragePage, error) {
	pageID, ok := c.activeByKey[compositeObjectKey(bucket, objectKey)]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrObjectNotFound, bucket, objectKey)
	}
	page, exists := c.pages[pageID]
	if !exists {
		return nil, fmt.Errorf("%w: page %s missing for %s/%s", ErrObjectNotFound, pageID, bucket, objectKey)
	}
	computed := computeSHA256Hex(page.Payload)
	if computed != page.Checksum || c.bucketIndex[bucket][objectKey] != computed {
		return nil, fmt.Errorf("%w: %s/%s", ErrChecksumMismatch, bucket, objectKey)
	}
	return page, nil
}

// VerifyChecksum verifies the SHA-256 digest of the active object in the specified bucket.
func (c *CASCompactor) VerifyChecksum(bucket, objectKey string) (string, error) {
	_, checksum, err := c.retrieveVerified(bucket, objectKey, false)
	return checksum, err
}

// RetrieveObject verifies cryptographic integrity and returns a defensive copy of the active object payload and its SHA-256 checksum.
func (c *CASCompactor) RetrieveObject(bucket, objectKey string) ([]byte, string, error) {
	return c.retrieveVerified(bucket, objectKey, true)
}

func (c *CASCompactor) retrieveVerified(bucket, objectKey string, withPayload bool) ([]byte, string, error) {
	if err := validateBucketAndObjectKey(bucket, objectKey); err != nil {
		return nil, "", err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	page, err := c.lookupVerifiedActivePageLocked(bucket, objectKey)
	if err != nil {
		return nil, "", err
	}
	if !withPayload {
		return nil, page.Checksum, nil
	}
	out := append([]byte(nil), page.Payload...)
	return out, page.Checksum, nil
}

// Snapshot returns a point-in-time GenerationSnapshot of active pages and bucket indexes.
func (c *CASCompactor) Snapshot() GenerationSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	indexes := make(map[string]map[string]string, len(c.bucketIndex))
	for bucket, objs := range c.bucketIndex {
		copied := make(map[string]string, len(objs))
		for k, v := range objs {
			copied[k] = v
		}
		indexes[bucket] = copied
	}

	var totalBytes int64
	activePages := 0
	for _, p := range c.pages {
		totalBytes += int64(len(p.Payload))
		if !p.Superseded {
			activePages++
		}
	}

	return GenerationSnapshot{
		Generation:    c.activeGeneration,
		ActivePages:   activePages,
		TotalBytes:    totalBytes,
		BucketIndexes: indexes,
		CapturedAt:    time.Now().UTC(),
	}
}
