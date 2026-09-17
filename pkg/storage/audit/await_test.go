package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAwaitCreateID(t *testing.T) {
	t.Parallel()
	idCh := make(chan string, 1)
	errCh := make(chan error, 1)
	idCh <- "CMD-1"
	got, err := AwaitCreate(context.Background(), time.Second, errors.New("timeout"), idCh, errCh)
	if err != nil || got != "CMD-1" {
		t.Fatalf("got=%s err=%v", got, err)
	}
}

func TestAwaitCreateError(t *testing.T) {
	t.Parallel()
	want := errors.New("create")
	idCh := make(chan string, 1)
	errCh := make(chan error, 1)
	errCh <- want
	_, err := AwaitCreate(context.Background(), time.Second, errors.New("timeout"), idCh, errCh)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}

func TestAwaitCreateTimeout(t *testing.T) {
	t.Parallel()
	timeoutErr := errors.New("timeout")
	got, err := AwaitCreate(context.Background(), time.Millisecond, timeoutErr, make(chan string), make(chan error))
	if !errors.Is(err, timeoutErr) || got != "" {
		t.Fatalf("got=%s err=%v", got, err)
	}
}

func TestAwaitCreateCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AwaitCreate(ctx, time.Second, errors.New("timeout"), make(chan string), make(chan error))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}
