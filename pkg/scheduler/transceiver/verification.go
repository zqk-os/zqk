package transceiver

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// MessageVerification tracks message delivery verification
type MessageVerification struct {
	mu            sync.RWMutex
	pending       map[string]*VerificationRequest
	verifications map[string]*VerificationResult
	logger        logging.Logger
	timeout       time.Duration
}

// VerificationRequest represents a pending verification
type VerificationRequest struct {
	MessageID    string
	EventType    string
	Timestamp    time.Time
	ExpectedURLs []string // URLs that should receive this message
	Timeout      time.Duration
}

// VerificationResult represents the result of verification
type VerificationResult struct {
	MessageID    string
	Success      bool
	VerifiedAt   time.Time
	VerifiedURLs []string // URLs that confirmed receipt
	FailedURLs   []string // URLs that failed or didn't respond
	Error        error
}

// NewMessageVerification creates a new message verification tracker
func NewMessageVerification(logger logging.Logger, timeout time.Duration) *MessageVerification {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	if timeout == 0 {
		timeout = 30 * time.Second // Default 30 second timeout
	}
	return &MessageVerification{
		pending:       make(map[string]*VerificationRequest),
		verifications: make(map[string]*VerificationResult),
		logger:        logger,
		timeout:       timeout,
	}
}

// RegisterMessage registers a message for verification
func (mv *MessageVerification) RegisterMessage(messageID, eventType string, expectedURLs []string) {
	_ = concurrency.RunInLockWithLogger(
		&mv.mu,
		LockNameMessageVerificationRegister,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			mv.pending[messageID] = &VerificationRequest{
				MessageID:    messageID,
				EventType:    eventType,
				Timestamp:    time.Now(),
				ExpectedURLs: expectedURLs,
				Timeout:      mv.timeout,
			}

			logging.Fluent(mv.logger).Debug(LogEventSchedulerTransceiverVerificationRegistered).
				MessageID(messageID).
				EventType(eventType).
				ExpectedURLCount(len(expectedURLs)).
				Log()
			return nil
		},
	)
}

// VerifyDelivery verifies that a message was delivered to an endpoint
// This is called by adapters after successful delivery
func (mv *MessageVerification) VerifyDelivery(messageID, url string, success bool, err error) {
	_ = concurrency.RunInLockWithLogger(
		&mv.mu,
		LockNameMessageVerificationVerify,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			request, exists := mv.pending[messageID]
			if !exists {
				// Message not registered for verification
				return nil
			}

			// Get or create verification result
			result, exists := mv.verifications[messageID]
			if !exists {
				result = &VerificationResult{
					MessageID:    messageID,
					VerifiedAt:   time.Now(),
					VerifiedURLs: []string{},
					FailedURLs:   []string{},
				}
				mv.verifications[messageID] = result
			}

			if success {
				result.VerifiedURLs = append(result.VerifiedURLs, url)
				logging.Fluent(mv.logger).Debug(LogEventSchedulerTransceiverVerificationDeliveryVerified).
					MessageID(messageID).
					URL(url).
					Log()
			} else {
				result.FailedURLs = append(result.FailedURLs, url)
				if err != nil {
					result.Error = err
				}
				logging.Fluent(mv.logger).Warn(LogEventSchedulerTransceiverVerificationDeliveryFailed).
					MessageID(messageID).
					URL(url).
					WithError(err).
					Log()
			}

			// Check if all expected URLs have been verified
			if len(result.VerifiedURLs)+len(result.FailedURLs) >= len(request.ExpectedURLs) {
				// All URLs verified (successfully or not)
				result.Success = len(result.FailedURLs) == 0
				delete(mv.pending, messageID)
				logging.Fluent(mv.logger).Info(LogEventSchedulerTransceiverVerificationComplete).
					MessageID(messageID).
					Success(result.Success).
					VerifiedURLCount(len(result.VerifiedURLs)).
					FailedURLCount(len(result.FailedURLs)).
					Log()
			}
			return nil
		},
	)
}

// GetVerificationResult returns the verification result for a message
func (mv *MessageVerification) GetVerificationResult(messageID string) (*VerificationResult, bool) {
	var result *VerificationResult
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&mv.mu,
		LockNameMessageVerificationGetResult,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			result, exists = mv.verifications[messageID]
			return nil
		},
	)
	return result, exists
}

// CleanupExpired removes expired verification requests
func (mv *MessageVerification) CleanupExpired() {
	_ = concurrency.RunInLockWithLogger(
		&mv.mu,
		LockNameMessageVerificationCleanup,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			now := time.Now()
			for messageID, request := range mv.pending {
				if now.Sub(request.Timestamp) > request.Timeout {
					// Request expired - mark as failed
					result := &VerificationResult{
						MessageID:    messageID,
						Success:      false,
						VerifiedAt:   now,
						VerifiedURLs: []string{},
						FailedURLs:   request.ExpectedURLs,
						Error:        errfmt.Errorf("verification timeout after %v", request.Timeout),
					}
					mv.verifications[messageID] = result
					delete(mv.pending, messageID)

					logging.Fluent(mv.logger).Warn(LogEventSchedulerTransceiverVerificationExpired).
						MessageID(messageID).
						Timeout(request.Timeout.String()).
						Log()
				}
			}
			return nil
		},
	)
}

// StartCleanup starts a background goroutine to cleanup expired verifications
func (mv *MessageVerification) StartCleanup(ctx context.Context) {
	bud := goroutinelabels.DefaultBudget()
	cleanupBuilder := goroutinelabels.NewGoroutine("message_verification_cleanup", "cleaning up expired message verifications")
	if bud != nil {
		cleanupBuilder = cleanupBuilder.WithBudget(bud)
	}
	cleanupBuilder.StartWithContext(ctx, func(ctx context.Context) error {
		ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				mv.CleanupExpired()
			}
		}
	})
}
