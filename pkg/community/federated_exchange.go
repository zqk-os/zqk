// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrUnsupportedDialect is returned when an operation references an unregistered dialect.
	ErrUnsupportedDialect = errors.New("federated_exchange: dialect is unregistered or unsupported")

	// ErrCircularTranslation is returned when a translation path contains a cycle.
	ErrCircularTranslation = errors.New("federated_exchange: circular dialect translation cycle detected")

	// ErrTranslationRuleNotFound is returned when no mapping rule exists between two dialects.
	ErrTranslationRuleNotFound = errors.New("federated_exchange: no translation rule found between dialects")

	// ErrInvalidDialectSchema is returned when a dialect schema has invalid metadata.
	ErrInvalidDialectSchema = errors.New("federated_exchange: invalid dialect schema definition")
)

// DialectSchema represents an ontological dialect for a specific cluster or domain.
type DialectSchema struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Version      string            `json:"version"`
	Fields       map[string]string `json:"fields"`
	RegisteredAt time.Time         `json:"registered_at"`
}

// TranslationRule defines directional field-level conversion logic from source to target dialect.
type TranslationRule struct {
	ID            string            `json:"id"`
	SourceDialect string            `json:"source_dialect"`
	TargetDialect string            `json:"target_dialect"`
	FieldMappings map[string]string `json:"field_mappings"` // sourceField -> targetField
}

// FederatedQuery represents a cross-cluster inquiry requiring dialect translation.
type FederatedQuery struct {
	ID            string                 `json:"id"`
	SourceDialect string                 `json:"source_dialect"`
	TargetDialect string                 `json:"target_dialect"`
	Payload       map[string]interface{} `json:"payload"`
	CreatedAt     time.Time              `json:"created_at"`
}

// TranslatedQuery captures the outcome of dialect conversion.
type TranslatedQuery struct {
	OriginalID        string                 `json:"original_id"`
	SourceDialect     string                 `json:"source_dialect"`
	TargetDialect     string                 `json:"target_dialect"`
	TranslatedPayload map[string]interface{} `json:"translated_payload"`
	Hops              []string               `json:"hops"`
	TranslatedAt      time.Time              `json:"translated_at"`
}

// FederatedExchange coordinates cross-cluster knowledge mapping, schema translations,
// and cyclic dependency defense.
type FederatedExchange struct {
	mu       sync.RWMutex
	dialects map[string]*DialectSchema
	rules    map[string]*TranslationRule // key: "source->target"
}

// NewFederatedExchange initializes an empty FederatedExchange.
func NewFederatedExchange() *FederatedExchange {
	return &FederatedExchange{
		dialects: make(map[string]*DialectSchema),
		rules:    make(map[string]*TranslationRule),
	}
}

// RegisterDialect registers a new dialect schema in the exchange.
func (e *FederatedExchange) RegisterDialect(schema DialectSchema) error {
	if err := strutil.ErrorOnBlank(schema.Name, "dialect name cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(schema.Namespace, "dialect namespace cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(schema.Version, "dialect version cannot be blank"); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if schema.RegisteredAt.IsZero() {
		schema.RegisteredAt = time.Now().UTC()
	}

	// Make shallow copy of fields
	fieldsCopy := make(map[string]string, len(schema.Fields))
	for k, v := range schema.Fields {
		fieldsCopy[k] = v
	}
	schema.Fields = fieldsCopy

	e.dialects[schema.Name] = &schema
	return nil
}

// RegisterRule records a translation rule from source dialect to target dialect.
func (e *FederatedExchange) RegisterRule(rule TranslationRule) error {
	if err := strutil.ErrorOnBlank(rule.ID, "rule ID cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(rule.SourceDialect, "source dialect cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(rule.TargetDialect, "target dialect cannot be blank"); err != nil {
		return err
	}
	if rule.SourceDialect == rule.TargetDialect {
		return fmt.Errorf("%w: source and target dialect cannot be identical", ErrInvalidDialectSchema)
	}
	if len(rule.FieldMappings) == 0 {
		return fmt.Errorf("translation rule must provide at least one field mapping")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.dialects[rule.SourceDialect]; !exists {
		return fmt.Errorf("%w: source dialect %q", ErrUnsupportedDialect, rule.SourceDialect)
	}
	if _, exists := e.dialects[rule.TargetDialect]; !exists {
		return fmt.Errorf("%w: target dialect %q", ErrUnsupportedDialect, rule.TargetDialect)
	}

	key := fmt.Sprintf("%s->%s", rule.SourceDialect, rule.TargetDialect)
	e.rules[key] = &rule
	return nil
}

// TranslateDirect performs a 1-hop field mapping from source dialect to target dialect.
func (e *FederatedExchange) TranslateDirect(query FederatedQuery) (*TranslatedQuery, error) {
	if err := strutil.ErrorOnBlank(query.ID, "query ID cannot be blank"); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(query.SourceDialect, "source dialect cannot be blank"); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(query.TargetDialect, "target dialect cannot be blank"); err != nil {
		return nil, err
	}
	if len(query.Payload) == 0 {
		return nil, fmt.Errorf("query payload cannot be empty")
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, exists := e.dialects[query.SourceDialect]; !exists {
		return nil, fmt.Errorf("%w: source dialect %q", ErrUnsupportedDialect, query.SourceDialect)
	}
	if _, exists := e.dialects[query.TargetDialect]; !exists {
		return nil, fmt.Errorf("%w: target dialect %q", ErrUnsupportedDialect, query.TargetDialect)
	}

	ruleKey := fmt.Sprintf("%s->%s", query.SourceDialect, query.TargetDialect)
	rule, exists := e.rules[ruleKey]
	if !exists {
		return nil, fmt.Errorf("%w: %s to %s", ErrTranslationRuleNotFound, query.SourceDialect, query.TargetDialect)
	}

	translatedPayload := make(map[string]interface{})
	for srcKey, val := range query.Payload {
		targetKey, mapped := rule.FieldMappings[srcKey]
		if mapped {
			translatedPayload[targetKey] = val
		} else {
			// Unmapped fields are preserved or passed with source key
			translatedPayload[srcKey] = val
		}
	}

	return &TranslatedQuery{
		OriginalID:        query.ID,
		SourceDialect:     query.SourceDialect,
		TargetDialect:     query.TargetDialect,
		TranslatedPayload: translatedPayload,
		Hops:              []string{query.SourceDialect, query.TargetDialect},
		TranslatedAt:      time.Now().UTC(),
	}, nil
}

// TranslateChain transforms a query across a sequence of intermediate dialect hops with cycle detection.
func (e *FederatedExchange) TranslateChain(query FederatedQuery, intermediateHops []string) (*TranslatedQuery, error) {
	if err := strutil.ErrorOnBlank(query.ID, "query ID cannot be blank"); err != nil {
		return nil, err
	}

	fullPath := make([]string, 0, len(intermediateHops)+2)
	fullPath = append(fullPath, query.SourceDialect)
	fullPath = append(fullPath, intermediateHops...)
	fullPath = append(fullPath, query.TargetDialect)

	// Cycle detection: ensure no dialect is visited more than once in path
	visited := make(map[string]bool)
	for _, hop := range fullPath {
		if visited[hop] {
			return nil, fmt.Errorf("%w: dialect %q repeated in path %v", ErrCircularTranslation, hop, fullPath)
		}
		visited[hop] = true
	}

	currentPayload := query.Payload
	for i := 0; i < len(fullPath)-1; i++ {
		src := fullPath[i]
		tgt := fullPath[i+1]
		hopQuery := FederatedQuery{
			ID:            query.ID,
			SourceDialect: src,
			TargetDialect: tgt,
			Payload:       currentPayload,
		}
		res, err := e.TranslateDirect(hopQuery)
		if err != nil {
			return nil, fmt.Errorf("chain failure at hop %s->%s: %w", src, tgt, err)
		}
		currentPayload = res.TranslatedPayload
	}

	return &TranslatedQuery{
		OriginalID:        query.ID,
		SourceDialect:     query.SourceDialect,
		TargetDialect:     query.TargetDialect,
		TranslatedPayload: currentPayload,
		Hops:              fullPath,
		TranslatedAt:      time.Now().UTC(),
	}, nil
}
