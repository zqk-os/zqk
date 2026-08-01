package storage

import (
	"context"
	"time"
)

// CallbackWaiter is the exported name for [callbackWaiter] (same contract as pkg/testing.CallbackWaiter).
type CallbackWaiter = callbackWaiter

// EventCollector is the exported name for [eventCollector] (same contract as pkg/testing.EventCollector).
type EventCollector = eventCollector

// NewCallbackWaiterForTest constructs a callback waiter for external storage_test package tests.
func NewCallbackWaiterForTest() *CallbackWaiter {
	return newCallbackWaiter()
}

// NewEventCollectorForTest constructs an event collector for external storage_test package tests.
func NewEventCollectorForTest() *EventCollector {
	return newEventCollector()
}

// WaitForConditionForTest exposes [waitForCondition] for external storage_test package tests.
func WaitForConditionForTest(ctx context.Context, cond func() bool, pollInterval time.Duration) bool {
	return waitForCondition(ctx, cond, pollInterval)
}

// WaitForConditionWithTimeoutForTest exposes [waitForConditionWithTimeout] for external storage_test package tests.
func WaitForConditionWithTimeoutForTest(ctx context.Context, cond func() bool, timeout, interval time.Duration) bool {
	return waitForConditionWithTimeout(ctx, cond, timeout, interval)
}
