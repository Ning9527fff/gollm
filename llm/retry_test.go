package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoWithRetry_Success(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	result, err := DoWithRetry(ctx, cfg, func() (string, error) {
		callCount++
		return "success", nil
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != "success" {
		t.Fatalf("expected 'success', got: %s", result)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got: %d", callCount)
	}
}

func TestDoWithRetry_RetryableError(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	retryableErr := &LLMError{
		Provider:   "test",
		StatusCode: 429,
		Message:    "rate limit",
		Retryable:  true,
	}

	result, err := DoWithRetry(ctx, cfg, func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", retryableErr
		}
		return "success", nil
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != "success" {
		t.Fatalf("expected 'success', got: %s", result)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got: %d", callCount)
	}
}

func TestDoWithRetry_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	nonRetryableErr := &LLMError{
		Provider:   "test",
		StatusCode: 400,
		Message:    "bad request",
		Retryable:  false,
	}

	result, err := DoWithRetry(ctx, cfg, func() (string, error) {
		callCount++
		return "", nonRetryableErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != "" {
		t.Fatalf("expected empty result, got: %s", result)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call (no retry), got: %d", callCount)
	}
}

func TestDoWithRetry_MaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	retryableErr := &LLMError{
		Provider:   "test",
		StatusCode: 500,
		Message:    "server error",
		Retryable:  true,
	}

	result, err := DoWithRetry(ctx, cfg, func() (string, error) {
		callCount++
		return "", retryableErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != "" {
		t.Fatalf("expected empty result, got: %s", result)
	}
	// MaxRetries=2 means total 3 attempts (1 initial + 2 retries)
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got: %d", callCount)
	}
}

func TestDoWithRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}

	callCount := 0
	retryableErr := &LLMError{
		Provider:   "test",
		StatusCode: 500,
		Message:    "server error",
		Retryable:  true,
	}

	// Cancel after first retry
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	result, err := DoWithRetry(ctx, cfg, func() (string, error) {
		callCount++
		return "", retryableErr
	})

	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty result, got: %s", result)
	}
	// Should have attempted at least 1, but not all retries
	if callCount < 1 || callCount > 3 {
		t.Fatalf("expected 1-3 calls, got: %d", callCount)
	}
}

func TestDoWithRetry_NoRetry(t *testing.T) {
	ctx := context.Background()
	cfg := NoRetry // MaxRetries = 0

	callCount := 0
	retryableErr := &LLMError{
		Provider:   "test",
		StatusCode: 500,
		Message:    "server error",
		Retryable:  true,
	}

	result, err := DoWithRetry(ctx, cfg, func() (string, error) {
		callCount++
		return "", retryableErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != "" {
		t.Fatalf("expected empty result, got: %s", result)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call (no retry), got: %d", callCount)
	}
}

func TestIsRetryableError_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"429 Too Many Requests", 429, true},
		{"500 Internal Server Error", 500, true},
		{"502 Bad Gateway", 502, true},
		{"503 Service Unavailable", 503, true},
		{"504 Gateway Timeout", 504, true},
		{"400 Bad Request", 400, false},
		{"401 Unauthorized", 401, false},
		{"403 Forbidden", 403, false},
		{"404 Not Found", 404, false},
		{"422 Unprocessable Entity", 422, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &LLMError{
				Provider:   "test",
				StatusCode: tt.statusCode,
				Message:    "test",
			}
			// Call NewLLMError to auto-detect retryability
			err = NewLLMError(err.Provider, err.StatusCode, err.Message, nil)
			if err.Retryable != tt.want {
				t.Errorf("statusCode %d: expected Retryable=%v, got %v", tt.statusCode, tt.want, err.Retryable)
			}
		})
	}
}

func TestIsRetryableError_NetworkErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"timeout error", errors.New("request timeout"), true},
		{"connection refused", errors.New("connection refused"), true},
		{"EOF", errors.New("EOF"), true},
		{"generic error", errors.New("unknown error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmErr := NewLLMError("test", 0, "test", tt.err)
			if llmErr.Retryable != tt.want {
				t.Errorf("error '%s': expected Retryable=%v, got %v", tt.err.Error(), tt.want, llmErr.Retryable)
			}
		})
	}
}
