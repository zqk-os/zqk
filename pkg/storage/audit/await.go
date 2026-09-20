package audit

import (
	"context"
	"time"
)

// AwaitCreate waits for an async Create to publish an id or error.
// timeoutErr is returned when timeout elapses (storage supplies the wrap).
func AwaitCreate(ctx context.Context, timeout time.Duration, timeoutErr error, idCh <-chan string, errCh <-chan error) (string, error) {
	select {
	case id := <-idCh:
		return id, nil
	case err := <-errCh:
		return "", err
	case <-time.After(timeout):
		return "", timeoutErr
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
