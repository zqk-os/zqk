// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testkit_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestEventually(t *testing.T) {
	var count int32
	go func() {
		time.Sleep(10 * time.Millisecond)
		atomic.StoreInt32(&count, 5)
	}()

	testkit.Eventually(t, 200*time.Millisecond, 5*time.Millisecond, func() bool {
		return atomic.LoadInt32(&count) == 5
	})
}

func TestEventuallyWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var counter int32
	go func() {
		time.Sleep(10 * time.Millisecond)
		atomic.StoreInt32(&counter, 10)
	}()

	err := testkit.EventuallyWithContext(ctx, 5*time.Millisecond, func(ctx context.Context) (bool, error) {
		return atomic.LoadInt32(&counter) == 10, nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
