package github

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	gh "github.com/google/go-github/v69/github"
)

const maxGitHubAttempts = 5

var retryBaseDelay = 500 * time.Millisecond

func withRetry[T any](ctx context.Context, op func() (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := 1; attempt <= maxGitHubAttempts; attempt++ {
		result, err := op()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt == maxGitHubAttempts || !isRetryableError(err) {
			return zero, err
		}

		delay := retryBaseDelay * time.Duration(1<<(attempt-1))
		if delay <= 0 {
			continue
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}

	return zero, lastErr
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	var respErr *gh.ErrorResponse
	if errors.As(err, &respErr) && respErr.Response != nil {
		switch respErr.Response.StatusCode {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	retryableFragments := []string{
		"stream error",
		"cancel",
		"connection reset",
		"unexpected eof",
		"server closed idle connection",
		"client connection lost",
		"timeout awaiting response headers",
	}
	for _, fragment := range retryableFragments {
		if strings.Contains(msg, fragment) {
			return true
		}
	}

	return false
}
