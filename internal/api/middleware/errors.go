package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse is the standard JSON error envelope returned by the API.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// AccountingError is returned when a business/accounting rule is violated.
// It maps to HTTP 422 Unprocessable Entity.
//
// Usage in a handler:
//
//	_ = c.Error(middleware.AccountingError{Message: "debit ≠ credit", Code: "UNBALANCED_ENTRY"})
type AccountingError struct {
	Message string
	Code    string
}

func (e AccountingError) Error() string { return e.Message }

// ErrorHandler is a gin middleware that intercepts errors attached via c.Error()
// and formats them as a consistent JSON ErrorResponse.
//
//   - AccountingError  → 422 Unprocessable Entity
//   - all other errors → 500 Internal Server Error
//
// Place this middleware AFTER the route handlers (or as a global middleware so
// that gin executes it during the post-handler phase).
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Only act when at least one error was attached to the context.
		if len(c.Errors) == 0 {
			return
		}

		// Use the last error attached — convention: handlers attach a single error.
		ginErr := c.Errors.Last()

		var acctErr AccountingError
		if errors.As(ginErr.Err, &acctErr) {
			// Business / accounting rule violation → 422
			c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
				Error: acctErr.Message,
				Code:  acctErr.Code,
			})
			return
		}

		// Generic server-side error → 500
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
		})
	}
}
