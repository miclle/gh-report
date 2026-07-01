package github

import (
	"context"
	"errors"
	"testing"
)

func TestWithRetryRetriesTemporaryStreamCancel(t *testing.T) {
	oldDelay := retryBaseDelay
	retryBaseDelay = 0
	t.Cleanup(func() { retryBaseDelay = oldDelay })

	attempts := 0

	got, err := withRetry(context.Background(), func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("stream error: stream ID 75; CANCEL; received from peer")
		}
		return "ok", nil
	})

	if err != nil {
		t.Fatalf("withRetry returned error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("withRetry returned %q, want ok", got)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestWithRetryStopsOnPermanentError(t *testing.T) {
	attempts := 0
	wantErr := errors.New("bad credentials")

	_, err := withRetry(context.Background(), func() (string, error) {
		attempts++
		return "", wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("withRetry error = %v, want %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
