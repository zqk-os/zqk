package observability

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestBuilder_Success(t *testing.T) {
	builder := NewBuilder("storage_flush").
		WithDuration(150*time.Millisecond).
		WithField("cell_id", "HTS-001").
		WithField("batch_size", 42).
		WithTags("storage", "wal")

	data, err := builder.Build()
	if err != nil {
		t.Fatalf("unexpected error building metric: %v", err)
	}

	if data.Operation != "storage_flush" {
		t.Errorf("expected operation storage_flush, got %s", data.Operation)
	}
	if data.Duration != 150*time.Millisecond {
		t.Errorf("expected duration 150ms, got %v", data.Duration)
	}
	if !data.Success {
		t.Errorf("expected Success=true, got %v", data.Success)
	}
	if data.Error != "" {
		t.Errorf("expected empty error, got %s", data.Error)
	}
	if len(data.Tags) != 2 || data.Tags[0] != "storage" || data.Tags[1] != "wal" {
		t.Errorf("expected tags [storage wal], got %v", data.Tags)
	}
	if data.Fields["cell_id"] != "HTS-001" || data.Fields["batch_size"] != 42 {
		t.Errorf("unexpected fields: %v", data.Fields)
	}
	if data.Timestamp.IsZero() {
		t.Errorf("expected non-zero timestamp")
	}
}

func TestBuilder_WithError(t *testing.T) {
	expectedErr := errors.New("connection reset by peer")
	builder := NewBuilder("network_dial").
		WithDuration(50 * time.Millisecond).
		WithError(expectedErr)

	data, err := builder.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.Success {
		t.Errorf("expected Success=false on error")
	}
	if data.Error != expectedErr.Error() {
		t.Errorf("expected error %q, got %q", expectedErr.Error(), data.Error)
	}
}

func TestBuilder_MissingOperation(t *testing.T) {
	builder := NewBuilder("")
	_, err := builder.Build()
	if err == nil {
		t.Fatal("expected error when operation is empty, got nil")
	}
}

func TestNoOpRecorder(t *testing.T) {
	noop := GetNoOpRecorder()
	if noop.IsEnabled() {
		t.Errorf("expected no-op recorder to be disabled")
	}

	builder := NewBuilder("test_op")
	if err := noop.Record("test_op", builder); err != nil {
		t.Errorf("unexpected error from no-op Record: %v", err)
	}
}

func TestInMemoryRecorder_Lifecycle(t *testing.T) {
	rec := NewInMemoryRecorder(true)
	if !rec.IsEnabled() {
		t.Fatalf("expected recorder to be enabled")
	}

	// Record success
	err := rec.Record("op_1", NewBuilder("op_1").WithDuration(10*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected record error: %v", err)
	}

	// Record failure
	err = rec.Record("op_2", NewBuilder("op_2").WithError(errors.New("fail")))
	if err != nil {
		t.Fatalf("unexpected record error: %v", err)
	}

	records := rec.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Operation != "op_1" || !records[0].Success {
		t.Errorf("unexpected first record: %+v", records[0])
	}
	if records[1].Operation != "op_2" || records[1].Success {
		t.Errorf("unexpected second record: %+v", records[1])
	}

	// Disable and record
	rec.SetEnabled(false)
	if rec.IsEnabled() {
		t.Errorf("expected recorder to be disabled")
	}
	err = rec.Record("op_3", NewBuilder("op_3"))
	if err != nil {
		t.Fatalf("unexpected record error when disabled: %v", err)
	}
	if len(rec.Records()) != 2 {
		t.Errorf("expected still 2 records when recording while disabled")
	}

	// Clear
	rec.Clear()
	if len(rec.Records()) != 0 {
		t.Errorf("expected 0 records after Clear()")
	}
}

func TestInMemoryRecorder_InvalidBuilder(t *testing.T) {
	rec := NewInMemoryRecorder(true)
	invalidBuilder := NewBuilder("") // empty operation fails validation
	err := rec.Record("", invalidBuilder)
	if err == nil {
		t.Fatal("expected Record to return error on invalid builder, got nil")
	}
}

func TestFactory_Operations(t *testing.T) {
	// Disabled factory
	disabledFactory := NewFactory(false)
	rec := disabledFactory.CreateRecorder("auth")
	if rec.IsEnabled() {
		t.Errorf("expected recorder from disabled factory to be disabled")
	}
	if disabledFactory.GetRecorder("nonexistent").IsEnabled() {
		t.Errorf("expected GetRecorder for missing component to return disabled noop")
	}

	// Enabled factory
	factory := NewFactory(true)
	authRec := factory.CreateRecorder("auth")
	if !authRec.IsEnabled() {
		t.Errorf("expected recorder from enabled factory to be enabled")
	}

	// Get existing
	fetchedRec := factory.GetRecorder("auth")
	if fetchedRec != authRec {
		t.Errorf("expected GetRecorder to return created recorder")
	}

	// Custom registered recorder
	customRec := NewInMemoryRecorder(true)
	factory.RegisterRecorder("custom", customRec)
	if factory.GetRecorder("custom") != customRec {
		t.Errorf("expected custom recorder to be returned")
	}
}

func TestObservability_Concurrency(t *testing.T) {
	factory := NewFactory(true)
	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("observability_worker_%d", workerID), "concurrent observability factory test").
			WithWaitGroup(&wg).
			StartSimple(func() {
				compName := fmt.Sprintf("comp_%d", workerID%5)
				rec := factory.CreateRecorder(compName)

				for j := 0; j < iterations; j++ {
					opName := fmt.Sprintf("op_%d_%d", workerID, j)
					builder := NewBuilder(opName).
						WithDuration(time.Duration(j)*time.Millisecond).
						WithField("worker", workerID).
						WithField("iter", j).
						WithTags("concurrency", "stress")

					_ = rec.Record(opName, builder)
					_ = rec.IsEnabled()
					_ = factory.GetRecorder(compName)
				}
			})
	}

	wg.Wait()
}
