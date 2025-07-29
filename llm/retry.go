package llm

import (
	"errors"
	"fmt"
	"net"
	"net/http"
)

// APIError wraps an HTTP error returned by the LLM provider.
type APIError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("API Error: status=%d, message=%q, cause=%v", e.StatusCode, e.Message, e.Err)
	}

	return fmt.Sprintf("API Error: status=%d, message=%q", e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// DefaultIsRetryableError returns true if the error is retryable.
// It handles common HTTP codes and network timeouts.
func DefaultIsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusConflict,
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}
