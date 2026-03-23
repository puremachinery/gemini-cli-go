package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/logging"
)

const (
	maxRetryAttempts = 3
	retryBaseDelay   = 200 * time.Millisecond
	retryMaxDelay    = 2 * time.Second
)

func doRequestWithRetry(ctx context.Context, client *http.Client, buildReq func() (*http.Request, error)) (*http.Response, error) {
	if client == nil {
		return nil, errors.New("http client is nil")
	}
	var lastErr error
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err == nil {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return resp, nil
			}
			if isRetryableStatus(resp.StatusCode) && attempt < maxRetryAttempts-1 {
				logging.Logger().Debug("retrying request after status",
					"attempt", attempt+1,
					"status", resp.StatusCode,
				)
				closeResponseBody(resp)
				if err := sleepWithContext(ctx, retryDelay(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return resp, nil
		}
		lastErr = err
		if isRetryableError(err) && attempt < maxRetryAttempts-1 {
			logging.Logger().Debug("retrying request after error",
				"attempt", attempt+1,
				"error", err,
			)
			if err := sleepWithContext(ctx, retryDelay(attempt)); err != nil {
				return nil, err
			}
			continue
		}
		return nil, err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("request retries exhausted")
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func retryDelay(attempt int) time.Duration {
	delay := retryBaseDelay * (1 << attempt)
	if delay > retryMaxDelay {
		return retryMaxDelay
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		_ = err
	}
}

func classifyStatus(code int) string {
	if isRetryableStatus(code) {
		return "transient"
	}
	return "permanent"
}

func formatHTTPError(prefix, status string, statusCode int, payload []byte) error {
	kind := classifyStatus(statusCode)
	message := strings.TrimSpace(string(payload))
	if message == "" {
		return fmt.Errorf("%s (%s): %s", prefix, kind, status)
	}
	return fmt.Errorf("%s (%s): %s: %s", prefix, kind, status, message)
}
