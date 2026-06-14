package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"ai-proxy/internal/logger"
)

const (
	defaultBufferSize = 1000
	defaultFlushInterval = 5 * time.Second
	defaultBatchSize    = 100
	maxRetries          = 3
)

// Service provides async batch writing of audit events with retry logic.
// Events are buffered in memory and flushed to the repository periodically
// or when the buffer reaches a threshold.
type Service struct {
	repo    Repository
	log     *slog.Logger
	buffer  chan *AuditEvent
	flushInterval time.Duration
	batchSize     int
	done    chan struct{}

	// Stats
	mu           sync.Mutex
	droppedCount int
	flushedCount int
	retryCount   int
}

// NewService creates an audit service and starts the background flush loop.
func NewService(repo Repository) *Service {
	s := &Service{
		repo:          repo,
		log:           logger.Default().With(slog.String("component", "audit")),
		buffer:        make(chan *AuditEvent, defaultBufferSize),
		flushInterval: defaultFlushInterval,
		batchSize:     defaultBatchSize,
		done:          make(chan struct{}),
	}
	go s.processBuffer()
	return s
}

// Stop gracefully shuts down the flush loop, flushing any remaining events.
func (s *Service) Stop() {
	close(s.done)
}

// Log enqueues an audit event for async batch writing.
// If the buffer is full, the event is dropped and a warning is logged.
func (s *Service) Log(event *AuditEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	select {
	case s.buffer <- event:
	default:
		s.mu.Lock()
		s.droppedCount++
		s.mu.Unlock()
		s.log.Warn("audit buffer full, dropping event",
			slog.String("event_type", string(event.EventType)),
			slog.Int("buffer_size", defaultBufferSize),
		)
	}
}

// LogAPIRequest is a convenience method for logging API proxy requests.
func (s *Service) LogAPIRequest(clientID, providerID, model, ip, requestID string, statusCode, latencyMs int, nonceValid bool) {
	ev := &AuditEvent{
		EventType:  EventAPIRequest,
		Severity:   SeverityInfo,
		ClientID:   strPtr(clientID),
		ActorType:  ActorClient,
		Action:     "api_request",
		Resource:   "provider",
		ProviderID: providerID,
		Model:      model,
		StatusCode: intPtr(statusCode),
		LatencyMs:  intPtr(latencyMs),
		NonceValid: boolPtr(nonceValid),
		IPAddress:  ip,
		RequestID:  requestID,
	}

	// Set status-based severity
	if statusCode >= 500 {
		ev.Severity = SeverityError
	} else if statusCode == 429 {
		ev.Severity = SeverityWarning
	}

	s.Log(ev)
}

// LogClientCreated logs a client creation event.
func (s *Service) LogClientCreated(clientID, adminID, ip, requestID string, beforeState, afterState map[string]string) {
	var before, after *json.RawMessage
	if beforeState != nil {
		b, _ := json.Marshal(beforeState)
		raw := json.RawMessage(b)
		before = &raw
	}
	if afterState != nil {
		a, _ := json.Marshal(afterState)
		raw := json.RawMessage(a)
		after = &raw
	}

	s.Log(&AuditEvent{
		EventType:   EventClientCreated,
		Severity:    SeverityInfo,
		AdminID:     strPtr(adminID),
		ActorType:   ActorAdmin,
		Action:      "create_client",
		Resource:    "client",
		ResourceID:  clientID,
		BeforeState: before,
		AfterState:  after,
		IPAddress:   ip,
		RequestID:   requestID,
	})
}

// LogKeysRotated logs a key rotation event.
func (s *Service) LogKeysRotated(clientID, adminID, ip, requestID string) {
	s.Log(&AuditEvent{
		EventType:  EventKeysRotated,
		Severity:   SeverityWarning,
		ClientID:   strPtr(clientID),
		AdminID:    strPtr(adminID),
		ActorType:  ActorAdmin,
		Action:     "rotate_keys",
		Resource:   "client",
		ResourceID: clientID,
		IPAddress:  ip,
		RequestID:  requestID,
	})
}

// LogNonceFailed logs a nonce validation failure.
func (s *Service) LogNonceFailed(clientID *string, ip, requestID, reason string) {
	s.Log(&AuditEvent{
		EventType: EventNonceFailed,
		Severity:  SeverityWarning,
		ClientID:  clientID,
		ActorType: ActorClient,
		Action:    "nonce_validation_failed",
		Resource:  "api",
		IPAddress: ip,
		RequestID: requestID,
	})
}

// LogRateLimitExceeded logs a rate limit event.
func (s *Service) LogRateLimitExceeded(clientID, ip, requestID string) {
	s.Log(&AuditEvent{
		EventType:  EventRateLimitExceeded,
		Severity:   SeverityWarning,
		ClientID:   strPtr(clientID),
		ActorType:  ActorClient,
		Action:     "rate_limit_exceeded",
		Resource:   "api",
		IPAddress:  ip,
		RequestID:  requestID,
	})
}

// LogClientAuthFailed logs an authentication failure.
func (s *Service) LogClientAuthFailed(clientID, ip, requestID, reason string) {
	s.Log(&AuditEvent{
		EventType: EventClientAuthFailed,
		Severity:  SeverityWarning,
		ClientID:  strPtr(clientID),
		ActorType: ActorClient,
		Action:    "client_auth_failed",
		Resource:  "api",
		IPAddress: ip,
		RequestID: requestID,
	})
}

// Stats returns current service metrics.
func (s *Service) Stats() (flushed, dropped, retried int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushedCount, s.droppedCount, s.retryCount
}

/* ─── Internal ──────────────────────────────────────────── */

// processBuffer runs the flush loop: it collects events from the buffer
// and flushes them in batches when either the batch size is reached or
// the flush interval elapses.
func (s *Service) processBuffer() {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	var batch []*AuditEvent

	for {
		select {
		case event := <-s.buffer:
			batch = append(batch, event)
			if len(batch) >= s.batchSize {
				batch = s.flush(batch)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				batch = s.flush(batch)
			}

		case <-s.done:
			// Final flush of remaining events
			if len(batch) > 0 {
				batch = s.flush(batch)
			}
			return
		}
	}
}

// flush writes a batch of events to the repository with retry logic.
// Events that fail after maxRetries are discarded. Returns a fresh empty batch.
func (s *Service) flush(batch []*AuditEvent) []*AuditEvent {
	if len(batch) == 0 {
		return batch[:0]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			s.mu.Lock()
			s.retryCount++
			s.mu.Unlock()
			time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond) // exponential backoff
		}

		err = s.repo.InsertBatch(ctx, batch)
		if err == nil {
			s.mu.Lock()
			s.flushedCount += len(batch)
			s.mu.Unlock()
			s.log.Debug("audit batch flushed",
				slog.Int("count", len(batch)),
			)
			return batch[:0]
		}

		s.log.Error("audit flush failed",
			slog.Int("attempt", attempt+1),
			slog.String("error", err.Error()),
		)
	}

	// Events discarded after exhausting retries
	s.mu.Lock()
	s.droppedCount += len(batch)
	s.mu.Unlock()
	s.log.Error("audit events discarded after retries",
		slog.Int("count", len(batch)),
		slog.String("error", err.Error()),
	)
	return batch[:0]
}

/* ─── Helpers ───────────────────────────────────────────── */

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
func boolPtr(b bool) *bool    { return &b }
