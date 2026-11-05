package io

import (
	"net/http"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// KindIoCache is the kernel kind for external respiration scraped cache records.
	KindIoCache = "io_cache"
)

// IoCacheRecord represents a cached fetch object stored in the kernel CAS.
type IoCacheRecord struct {
	ID           string `json:"id" yaml:"id"`
	Kind         string `json:"kind" yaml:"kind"`
	Title        string `json:"title" yaml:"title"`
	URL          string `json:"url" yaml:"url"`
	Domain       string `json:"domain" yaml:"domain"`
	RawHash      string `json:"raw_hash" yaml:"raw_hash"`
	CleanHash    string `json:"clean_hash" yaml:"clean_hash"`
	RawContent   string `json:"raw_content,omitempty" yaml:"raw_content,omitempty"`
	CleanContent string `json:"clean_content" yaml:"clean_content"`
	FetchedAt    string `json:"fetched_at" yaml:"fetched_at"`
	Status       string `json:"status" yaml:"status"`
}

// ToMap converts IoCacheRecord to a storage map.
func (r *IoCacheRecord) ToMap() map[string]any {
	return map[string]any{
		objects.FieldKeyID:        r.ID,
		objects.FieldKeyKind:      KindIoCache,
		objects.FieldKeyTitle:     r.Title,
		"url":                     r.URL,
		"domain":                  r.Domain,
		"raw_hash":                r.RawHash,
		"clean_hash":              r.CleanHash,
		"raw_content":             r.RawContent,
		"clean_content":           r.CleanContent,
		"fetched_at":              r.FetchedAt,
		objects.FieldKeyStatus:    "active",
		objects.FieldKeyCreatedAt: r.FetchedAt,
	}
}

// FetchOptions configures an external respiration fetch operation.
type FetchOptions struct {
	CacheCAS    bool
	Diff        bool
	Raw         bool
	Timeout     time.Duration
	HTTPClient  *http.Client
	ProjectRoot string
}

// FetchResult contains the results of a respiration fetch.
type FetchResult struct {
	URL               string `json:"url"`
	Domain            string `json:"domain"`
	StatusCode        int    `json:"status_code"`
	RawHash           string `json:"raw_hash"`
	CleanHash         string `json:"clean_hash"`
	CleanContent      string `json:"clean_content,omitempty"`
	RawContent        string `json:"raw_content,omitempty"`
	FetchedAt         string `json:"fetched_at"`
	Cached            bool   `json:"cached"`
	PreviousHash      string `json:"previous_hash,omitempty"`
	PreviousFetchedAt string `json:"previous_fetched_at,omitempty"`
	Diff              string `json:"diff,omitempty"`
}
