package shared

import "net/http"

// AppError represents a structured application error with an HTTP status code.
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *AppError) Error() string {
	return e.Message
}

// Common application errors.
var (
	ErrNotFound       = NewAppError(http.StatusNotFound, "Resource not found")
	ErrUnauthorized   = NewAppError(http.StatusUnauthorized, "Unauthorized")
	ErrForbidden      = NewAppError(http.StatusForbidden, "Forbidden")
	ErrBadRequest     = NewAppError(http.StatusBadRequest, "Bad request")
	ErrConflict       = NewAppError(http.StatusConflict, "Resource already exists")
	ErrRateLimited    = NewAppError(http.StatusTooManyRequests, "Rate limit exceeded")
	ErrInternal       = NewAppError(http.StatusInternalServerError, "Internal server error")
	ErrValidation     = NewAppError(http.StatusUnprocessableEntity, "Validation failed")
	ErrNonceInvalid   = NewAppError(http.StatusUnauthorized, "Invalid or missing nonce")
	ErrNonceReplayed  = NewAppError(http.StatusUnauthorized, "Nonce already used")
	ErrTimestampInvalid = NewAppError(http.StatusBadRequest, "Invalid or expired timestamp")
	ErrClientSuspended = NewAppError(http.StatusForbidden, "Client is suspended")
)

// NewAppError creates a new AppError with the given status code and message.
func NewAppError(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// WithDetail adds detail to the error and returns it (fluent).
func (e *AppError) WithDetail(detail string) *AppError {
	e.Detail = detail
	return e
}
