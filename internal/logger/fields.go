package logger

import "log/slog"

// Standard field keys used across the application for structured logging.
const (
	KeyRequestID    = "request_id"
	KeyClientID     = "client_id"
	KeyAdminID      = "admin_id"
	KeyProviderID   = "provider_id"
	KeyModel        = "model"
	KeyStatusCode   = "status_code"
	KeyLatencyMs    = "latency_ms"
	KeyIPAddress    = "ip_address"
	KeyUserAgent    = "user_agent"
	KeyComponent    = "component"
	KeyError        = "error"
	KeyEventType    = "event_type"
	KeySeverity     = "severity"
	KeyAction       = "action"
	KeyResource     = "resource"
	KeyResourceID   = "resource_id"
	KeyNonce        = "nonce"
	KeyTokenType    = "token_type"
	KeyRateLimit    = "rate_limit"
	KeyDuration     = "duration"
	KeyEndpoint     = "endpoint"
	KeyMethod       = "method"
)

// Convenience wrappers for creating slog.Attr with standard keys.
func AttrRequestID(v string) slog.Attr   { return slog.String(KeyRequestID, v) }
func AttrClientID(v string) slog.Attr    { return slog.String(KeyClientID, v) }
func AttrAdminID(v string) slog.Attr     { return slog.String(KeyAdminID, v) }
func AttrProviderID(v string) slog.Attr  { return slog.String(KeyProviderID, v) }
func AttrModel(v string) slog.Attr       { return slog.String(KeyModel, v) }
func AttrStatusCode(v int) slog.Attr     { return slog.Int(KeyStatusCode, v) }
func AttrLatencyMs(v int) slog.Attr      { return slog.Int(KeyLatencyMs, v) }
func AttrIPAddress(v string) slog.Attr   { return slog.String(KeyIPAddress, v) }
func AttrError(v error) slog.Attr        { return slog.Any(KeyError, v) }
func AttrComponent(v string) slog.Attr   { return slog.String(KeyComponent, v) }
func AttrEndpoint(v string) slog.Attr    { return slog.String(KeyEndpoint, v) }
func AttrMethod(v string) slog.Attr      { return slog.String(KeyMethod, v) }
func AttrDuration(v string) slog.Attr    { return slog.String(KeyDuration, v) }
