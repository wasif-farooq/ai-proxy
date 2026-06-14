package audit

import (
	"encoding/json"
	"time"
)

// Severity represents the importance level of an audit event.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// ValidSeverities contains all valid severity values.
var ValidSeverities = []Severity{SeverityInfo, SeverityWarning, SeverityError, SeverityCritical}

// EventType categorises audit events for filtering and compliance reporting.
type EventType string

const (
	EventClientCreated         EventType = "client_created"
	EventClientUpdated         EventType = "client_updated"
	EventClientDeleted         EventType = "client_deleted"
	EventKeysRotated           EventType = "keys_rotated"
	EventTokenIssued           EventType = "token_issued"
	EventTokenRevoked          EventType = "token_revoked"
	EventNonceFailed           EventType = "nonce_validation_failed"
	EventRateLimitExceeded     EventType = "rate_limit_exceeded"
	EventAPIRequest            EventType = "api_request"
	EventAdminLogin            EventType = "admin_login"
	EventAdminLogout           EventType = "admin_logout"
	EventClientAuthFailed      EventType = "client_auth_failed"
	EventProviderDisabled      EventType = "provider_disabled"
	EventProviderEnabled       EventType = "provider_enabled"
	EventProviderKeySet        EventType = "provider_key_set"
	EventProviderKeyDeleted    EventType = "provider_key_deleted"
)

// ActorType identifies who performed the action.
type ActorType string

const (
	ActorClient ActorType = "client"
	ActorAdmin  ActorType = "admin"
	ActorSystem ActorType = "system"
)

// Broadcaster is an interface for broadcasting audit events to connected clients
// (e.g., WebSocket hub). The audit middleware calls Broadcast after logging an event.
type Broadcaster interface {
	Broadcast(msg interface{})
}

// AuditEvent is the central structure for all security-relevant events.
// It maps directly to the audit_logs table in PostgreSQL.
type AuditEvent struct {
	ID         string    `json:"id"`
	EventType  EventType `json:"event_type"`
	Severity   Severity  `json:"severity"`

	ClientID *string   `json:"client_id,omitempty"`
	AdminID  *string   `json:"admin_id,omitempty"`
	ActorType ActorType `json:"actor_type"`

	RequestID string `json:"request_id,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`

	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id,omitempty"`

	ProviderID string `json:"provider_id,omitempty"`
	Model      string `json:"model,omitempty"`
	StatusCode *int   `json:"status_code,omitempty"`
	LatencyMs  *int   `json:"latency_ms,omitempty"`

	NonceValid *bool   `json:"nonce_valid,omitempty"`
	TokenType  string  `json:"token_type,omitempty"`

	BeforeState *json.RawMessage `json:"before_state,omitempty"`
	AfterState  *json.RawMessage `json:"after_state,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}

// AuditFilter carries optional filter fields for querying audit events.
type AuditFilter struct {
	EventType *EventType
	Severity  *Severity
	ClientID  *string
	AdminID   *string
	Limit     int
	Offset    int
}

// AuditList is the result of a paginated audit event query.
type AuditList struct {
	Events []AuditEvent `json:"events"`
	Total  int          `json:"total"`
}
