package shared

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the standard envelope for all API responses.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

// ErrorBody carries structured error information.
type ErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// Meta holds pagination or other response metadata.
type Meta struct {
	Total  int `json:"total,omitempty"`
	Page   int `json:"page,omitempty"`
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// SendOK sends a 200 response with the given data.
func SendOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
	})
}

// SendCreated sends a 201 response with the given data.
func SendCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Success: true,
		Data:    data,
	})
}

// SendNoContent sends a 204 response with no body.
func SendNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// SendError sends an error response for an AppError.
func SendError(c *gin.Context, err *AppError) {
	c.JSON(err.Code, Response{
		Success: false,
		Error: &ErrorBody{
			Code:    err.Code,
			Message: err.Message,
			Detail:  err.Detail,
		},
	})
}

// SendValidationError sends a 422 validation error response.
func SendValidationError(c *gin.Context, message string) {
	c.JSON(http.StatusUnprocessableEntity, Response{
		Success: false,
		Error: &ErrorBody{
			Code:    http.StatusUnprocessableEntity,
			Message: "Validation failed",
			Detail:  message,
		},
	})
}

// SendPaginated sends a paginated response with data and metadata.
func SendPaginated(c *gin.Context, data interface{}, total, page, limit int) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
		Meta: &Meta{
			Total: total,
			Page:  page,
			Limit: limit,
		},
	})
}
