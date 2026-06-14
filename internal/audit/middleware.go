package audit

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-proxy/internal/logger"
)

// Middleware returns a Gin handler that automatically logs every API request
// as an audit event on completion. It captures request metadata, status,
// latency, client info, and error responses.
//
// Use this on any route group that should be audit-logged (proxy routes,
// auth endpoints, admin mutations).
//
// If a Broadcaster is provided, the audit event is also broadcast to connected
// clients (e.g., WebSocket hub) after being logged.
func Middleware(auditService *Service, broadcaster ...Broadcaster) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process the request
		c.Next()

		latencyMs := int(time.Since(start).Milliseconds())
		statusCode := c.Writer.Status()
		path := c.Request.URL.Path

		// Determine event type based on method + path
		eventType := deriveEventType(c.Request.Method, path)

		// Extract identifiers from request context
		clientID, _ := c.Get("client_id")
		clientIDStr, _ := clientID.(string)

		// Build audit event
		ev := &AuditEvent{
			EventType:  eventType,
			Severity:   severityFromStatus(statusCode),
			ClientID:   optionalStr(clientIDStr),
			ActorType:  ActorClient,
			Action:     deriveAction(c.Request.Method),
			Resource:   deriveResource(path),
			ResourceID: clientIDStr,
			StatusCode: &statusCode,
			LatencyMs:  &latencyMs,
			RequestID:  c.GetString(logger.KeyRequestID),
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			Timestamp:  start,
		}

		auditService.Log(ev)

		// Broadcast to connected WebSocket clients
		for _, b := range broadcaster {
			b.Broadcast(ev)
		}
	}
}

// severityFromStatus maps HTTP status codes to audit severity levels.
func severityFromStatus(status int) Severity {
	switch {
	case status >= 500:
		return SeverityError
	case status == 429:
		return SeverityWarning
	case status >= 400:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// deriveEventType maps HTTP method + path combinations to audit event types.
func deriveEventType(method, path string) EventType {
	switch {
	case method == http.MethodPost && path == "/api/v1/chat/completions":
		return EventAPIRequest
	case method == http.MethodPost && strings.Contains(path, "/clients"):
		return EventClientCreated
	case method == http.MethodPut && strings.Contains(path, "/clients") && strings.Contains(path, "/rotate"):
		return EventKeysRotated
	case method == http.MethodDelete && strings.Contains(path, "/clients"):
		return EventClientDeleted
	case method == http.MethodPost && strings.Contains(path, "/auth"):
		return EventAdminLogin
	default:
		return EventAPIRequest
	}
}

// deriveAction returns a readable action string from the HTTP method.
func deriveAction(method string) string {
	switch method {
	case http.MethodGet:
		return "read"
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return method
	}
}

// deriveResource extracts the resource name from the URL path.
func deriveResource(path string) string {
	switch {
	case strings.Contains(path, "/clients"):
		return "client"
	case strings.Contains(path, "/providers"):
		return "provider"
	case strings.Contains(path, "/chat/completions"):
		return "provider"
	case strings.Contains(path, "/auth"):
		return "admin"
	case strings.Contains(path, "/admin"):
		return "admin"
	default:
		return "api"
	}
}

// optionalStr returns nil for empty strings, which helps with DB nullable columns.
func optionalStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
