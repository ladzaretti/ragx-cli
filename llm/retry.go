package llm

import (
	"errors"
	"fmt"
	"net"
	"net/http"
)

// APIError represents an error returned by the LLM client.
type APIError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("API Error: Status=%d, Message='%s', OriginalErr=%v", e.StatusCode, e.Message, e.Err)
	}

	return fmt.Sprintf("API Error: Status=%d, Message='%s'", e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// DefaultIsRetryableError provides a default implementation
// based on common HTTP codes and network errors.
func DefaultIsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusConflict, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}
