// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

// Package testpoll provides eventual consistency polling utilities to replace time.Sleep in test suites (K:F-L-TESTING-001).
package testkit

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Eventually polls condition every interval until it returns true, context expires, or timeout is reached.
func Eventually(t *testing.T, timeout, interval time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v timeout", timeout)
			return
		}
		time.Sleep(interval)
	}
}

// EventuallyWithContext polls condition with a cancellable context.
func EventuallyWithContext(ctx context.Context, interval time.Duration, condition func(ctx context.Context) (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		ok, err := condition(ctx)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled before condition satisfied: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
