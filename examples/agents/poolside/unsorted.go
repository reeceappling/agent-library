package poolside

import (
	"context"
	"errors"
	"net"
)

// sanitizeHTTPError sanitizes HTTP client errors to prevent leaking sensitive information.
// It checks for context deadline/cancellation errors and returns generic timeout messages
// instead of potentially exposing request details, headers, or other sensitive data.
func sanitizeHTTPError(err error) error {
	if err == nil {
		return nil
	}

	// Check for context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("request timeout: API call exceeded deadline")
	}

	// Check for context cancellation
	if errors.Is(err, context.Canceled) {
		return errors.New("request cancelled")
	}

	// Check for network timeout errors
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errors.New("request timeout: network operation exceeded timeout")
	}

	// For other network errors, provide generic message without exposing details
	if _, ok := err.(net.Error); ok {
		return errors.New("network error: failed to reach API server")
	}

	// Return original error if it's not a sensitive type
	return err
}
