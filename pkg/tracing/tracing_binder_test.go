// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package tracing

import (
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestNewBinder(t *testing.T) {
	t.Parallel()

	b := NewBinder()
	if b == nil {
		t.Fatal("expected non-nil Binder from NewBinder")
	}
}

func TestBindOrPark_EmptyInputs(t *testing.T) {
	t.Parallel()

	b := NewBinder()

	// Both nil/empty
	bound, parked := b.BindOrPark(nil, nil)
	if len(bound) != 0 || len(parked) != 0 {
		t.Fatalf("expected empty slices for nil inputs, got bound=%d, parked=%d", len(bound), len(parked))
	}

	// Empty slices
	bound, parked = b.BindOrPark([]Ownership{}, []Ownership{})
	if len(bound) != 0 || len(parked) != 0 {
		t.Fatalf("expected empty slices for empty inputs, got bound=%d, parked=%d", len(bound), len(parked))
	}

	// Empty reqs with non-empty blis
	blis := []Ownership{
		{ID: "bli-1", Kind: "backlog_item", Status: "active"},
	}
	bound, parked = b.BindOrPark([]Ownership{}, blis)
	if len(bound) != 0 || len(parked) != 0 {
		t.Fatalf("expected empty slices when reqs is empty, got bound=%d, parked=%d", len(bound), len(parked))
	}
}

func TestBindOrPark_NonActiveStatusesIgnored(t *testing.T) {
	t.Parallel()

	b := NewBinder()
	nonActiveStatuses := []string{
		"draft",
		"in_progress",
		"completed",
		"parked",
		"archived",
		"rejected",
		"pending",
		"",
	}

	reqs := make([]Ownership, 0, len(nonActiveStatuses))
	for i, st := range nonActiveStatuses {
		reqs = append(reqs, Ownership{
			ID:     fmt.Sprintf("req-%d", i),
			Kind:   "req",
			Status: st,
		})
	}

	blis := []Ownership{
		{ID: "bli-1", Kind: "backlog_item", Status: "active"},
		{ID: "bli-2", Kind: "backlog_item", Status: "active"},
	}

	bound, parked := b.BindOrPark(reqs, blis)
	if len(bound) != 0 {
		t.Fatalf("expected 0 bound for non-active reqs, got %d", len(bound))
	}
	if len(parked) != 0 {
		t.Fatalf("expected 0 parked for non-active reqs, got %d", len(parked))
	}
}

func TestBindOrPark_ExactMatch(t *testing.T) {
	t.Parallel()

	b := NewBinder()
	reqs := []Ownership{
		{ID: "req-1", Kind: "req", Status: "active"},
		{ID: "req-2", Kind: "req", Status: "active"},
		{ID: "req-3", Kind: "req", Status: "active"},
	}
	blis := []Ownership{
		{ID: "bli-1", Kind: "backlog_item", Status: "active"},
		{ID: "bli-2", Kind: "backlog_item", Status: "in_progress"},
		{ID: "bli-3", Kind: "backlog_item", Status: "active"},
	}

	bound, parked := b.BindOrPark(reqs, blis)
	if len(bound) != 3 {
		t.Fatalf("expected 3 bound, got %d", len(bound))
	}
	if len(parked) != 0 {
		t.Fatalf("expected 0 parked, got %d", len(parked))
	}

	for i, r := range bound {
		if r.ID != reqs[i].ID {
			t.Errorf("bound item at %d: expected %s, got %s", i, reqs[i].ID, r.ID)
		}
		if r.Status != "active" {
			t.Errorf("bound item at %d: expected active status, got %s", i, r.Status)
		}
	}
}

func TestBindOrPark_MoreRequirementsThanBLIs(t *testing.T) {
	t.Parallel()

	b := NewBinder()
	reqs := []Ownership{
		{ID: "req-1", Kind: "req", Status: "active"},
		{ID: "req-2", Kind: "req", Status: "active"},
		{ID: "req-3", Kind: "req", Status: "active"},
		{ID: "req-4", Kind: "req", Status: "active"},
		{ID: "req-5", Kind: "req", Status: "active"},
	}
	blis := []Ownership{
		{ID: "bli-1", Kind: "backlog_item", Status: "active"},
		{ID: "bli-2", Kind: "backlog_item", Status: "active"},
	}

	bound, parked := b.BindOrPark(reqs, blis)
	if len(bound) != 2 {
		t.Fatalf("expected 2 bound, got %d", len(bound))
	}
	if len(parked) != 3 {
		t.Fatalf("expected 3 parked, got %d", len(parked))
	}

	// Verify bound items
	if bound[0].ID != "req-1" || bound[1].ID != "req-2" {
		t.Errorf("unexpected bound items: %v", bound)
	}

	// Verify parked items
	expectedParkedIDs := []string{"req-3", "req-4", "req-5"}
	for i, p := range parked {
		if p.ID != expectedParkedIDs[i] {
			t.Errorf("parked item %d: expected %s, got %s", i, expectedParkedIDs[i], p.ID)
		}
		if p.Status != "parked" {
			t.Errorf("parked item %d: expected status 'parked', got %s", i, p.Status)
		}
		if p.ParkReason != "no_covering_bli" {
			t.Errorf("parked item %d: expected reason 'no_covering_bli', got %s", i, p.ParkReason)
		}
	}
}

func TestBindOrPark_FewerRequirementsThanBLIs(t *testing.T) {
	t.Parallel()

	b := NewBinder()
	reqs := []Ownership{
		{ID: "req-1", Kind: "req", Status: "active"},
		{ID: "req-2", Kind: "req", Status: "active"},
	}
	blis := []Ownership{
		{ID: "bli-1", Kind: "backlog_item", Status: "active"},
		{ID: "bli-2", Kind: "backlog_item", Status: "active"},
		{ID: "bli-3", Kind: "backlog_item", Status: "active"},
		{ID: "bli-4", Kind: "backlog_item", Status: "active"},
	}

	bound, parked := b.BindOrPark(reqs, blis)
	if len(bound) != 2 {
		t.Fatalf("expected 2 bound, got %d", len(bound))
	}
	if len(parked) != 0 {
		t.Fatalf("expected 0 parked, got %d", len(parked))
	}
}

func TestBindOrPark_ZeroBLIs(t *testing.T) {
	t.Parallel()

	b := NewBinder()
	reqs := []Ownership{
		{ID: "req-1", Kind: "req", Status: "active"},
		{ID: "req-2", Kind: "req", Status: "active"},
		{ID: "req-3", Kind: "req", Status: "active"},
	}

	bound, parked := b.BindOrPark(reqs, nil)
	if len(bound) != 0 {
		t.Fatalf("expected 0 bound, got %d", len(bound))
	}
	if len(parked) != 3 {
		t.Fatalf("expected 3 parked, got %d", len(parked))
	}
	for i, p := range parked {
		if p.Status != "parked" {
			t.Errorf("parked[%d] status: expected parked, got %s", i, p.Status)
		}
		if p.ParkReason != "no_covering_bli" {
			t.Errorf("parked[%d] park reason: expected no_covering_bli, got %s", i, p.ParkReason)
		}
	}
}

func TestBindOrPark_InterleavedStatuses(t *testing.T) {
	t.Parallel()

	b := NewBinder()
	reqs := []Ownership{
		{ID: "req-0", Kind: "req", Status: "draft"},
		{ID: "req-1", Kind: "req", Status: "active"},
		{ID: "req-2", Kind: "req", Status: "completed"},
		{ID: "req-3", Kind: "req", Status: "active"},
		{ID: "req-4", Kind: "req", Status: "parked"},
		{ID: "req-5", Kind: "req", Status: "active"},
	}

	// 4 BLIs: indices 0, 1, 2, 3 are within len(blis).
	// req-0 is draft -> skipped
	// req-1 is active (i=1 < 4) -> bound
	// req-2 is completed -> skipped
	// req-3 is active (i=3 < 4) -> bound
	// req-4 is parked -> skipped
	// req-5 is active (i=5 >= 4) -> parked
	blis := []Ownership{
		{ID: "bli-0", Kind: "backlog_item", Status: "active"},
		{ID: "bli-1", Kind: "backlog_item", Status: "active"},
		{ID: "bli-2", Kind: "backlog_item", Status: "active"},
		{ID: "bli-3", Kind: "backlog_item", Status: "active"},
	}

	bound, parked := b.BindOrPark(reqs, blis)
	if len(bound) != 2 {
		t.Fatalf("expected 2 bound, got %d", len(bound))
	}
	if bound[0].ID != "req-1" || bound[1].ID != "req-3" {
		t.Errorf("unexpected bound items: %v", bound)
	}

	if len(parked) != 1 {
		t.Fatalf("expected 1 parked, got %d", len(parked))
	}
	if parked[0].ID != "req-5" {
		t.Errorf("expected parked item req-5, got %s", parked[0].ID)
	}
	if parked[0].Status != "parked" || parked[0].ParkReason != "no_covering_bli" {
		t.Errorf("unexpected parked item state: %+v", parked[0])
	}
}

func TestBindOrPark_LargeScale(t *testing.T) {
	t.Parallel()

	b := NewBinder()
	const count = 10000

	reqs := make([]Ownership, count)
	blis := make([]Ownership, count)
	for i := 0; i < count; i++ {
		reqs[i] = Ownership{
			ID:     fmt.Sprintf("req-%d", i),
			Kind:   "req",
			Status: "active",
		}
		blis[i] = Ownership{
			ID:     fmt.Sprintf("bli-%d", i),
			Kind:   "backlog_item",
			Status: "active",
		}
	}

	bound, parked := b.BindOrPark(reqs, blis)
	if len(bound) != count {
		t.Fatalf("expected %d bound, got %d", count, len(bound))
	}
	if len(parked) != 0 {
		t.Fatalf("expected 0 parked, got %d", len(parked))
	}

	// Test half BLIs
	halfBLIs := blis[:count/2]
	boundHalf, parkedHalf := b.BindOrPark(reqs, halfBLIs)
	if len(boundHalf) != count/2 {
		t.Fatalf("expected %d bound, got %d", count/2, len(boundHalf))
	}
	if len(parkedHalf) != count/2 {
		t.Fatalf("expected %d parked, got %d", count/2, len(parkedHalf))
	}
}

func TestBindOrPark_ConcurrencySafety(t *testing.T) {
	t.Parallel()

	sharedBinder := NewBinder()
	const workers = 20
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("tracing_concurrency_test", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				runConcurrentBinderWorker(t, sharedBinder, workerID, iterations)
			})
	}

	wg.Wait()
}

func runConcurrentBinderWorker(t *testing.T, shared *Binder, workerID, iterations int) {
	localBinder := NewBinder()

	for iter := 0; iter < iterations; iter++ {
		reqs := []Ownership{
			{ID: fmt.Sprintf("req-%d-%d-0", workerID, iter), Kind: "req", Status: "active"},
			{ID: fmt.Sprintf("req-%d-%d-1", workerID, iter), Kind: "req", Status: "draft"},
			{ID: fmt.Sprintf("req-%d-%d-2", workerID, iter), Kind: "req", Status: "active"},
		}
		blis := []Ownership{
			{ID: fmt.Sprintf("bli-%d-%d-0", workerID, iter), Kind: "backlog_item", Status: "active"},
		}

		// Test on shared binder
		bound, parked := shared.BindOrPark(reqs, blis)
		if len(bound) != 1 || len(parked) != 1 {
			t.Errorf("worker %d iter %d shared: expected 1 bound, 1 parked, got %d bound, %d parked",
				workerID, iter, len(bound), len(parked))
		}

		// Test on local binder
		boundLoc, parkedLoc := localBinder.BindOrPark(reqs, blis)
		if len(boundLoc) != 1 || len(parkedLoc) != 1 {
			t.Errorf("worker %d iter %d local: expected 1 bound, 1 parked, got %d bound, %d parked",
				workerID, iter, len(boundLoc), len(parkedLoc))
		}
	}
}
