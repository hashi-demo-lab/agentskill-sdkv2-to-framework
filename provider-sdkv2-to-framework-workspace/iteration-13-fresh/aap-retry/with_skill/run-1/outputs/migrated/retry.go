//go:generate mockgen -source=retry.go -destination=mock_provider/mock_retry.go
package provider

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// RetryOperationFunc defines the signature for operations that can be retried.
type RetryOperationFunc func() ([]byte, diag.Diagnostics, int)

// RetryOperation interface for testing with mocks.
type RetryOperation interface {
	Execute() ([]byte, diag.Diagnostics, int)
}

// WrapRetryOperation converts a RetryOperation interface to a RetryOperationFunc.
func WrapRetryOperation(op RetryOperation) RetryOperationFunc {
	return func() ([]byte, diag.Diagnostics, int) {
		return op.Execute()
	}
}

// RetryStateRefreshFunc is the framework-side replacement for
// helper/retry.StateRefreshFunc. The signature is identical, so any helpers
// previously typed against retry.StateRefreshFunc can be ported by changing
// only the imported name (or, by Go's type-identity rules, by passing them
// directly as func() (any, string, error) values).
type RetryStateRefreshFunc func() (any, string, error)

// stateChangeConf is an in-package replacement for helper/retry.StateChangeConf.
// It carries the same fields and is consumed by waitForState below. Keeping the
// configuration as a struct preserves the existing public API of this package
// (CreateRetryConfig + RetryWithConfig) so call sites compile unchanged.
type stateChangeConf struct {
	Pending    []string
	Target     []string
	Refresh    RetryStateRefreshFunc
	Timeout    time.Duration
	MinTimeout time.Duration // poll interval between refreshes
	Delay      time.Duration // delay before the first refresh
}

// waitForState replaces retry.StateChangeConf{Pending, Target, Refresh}.WaitForStateContext(ctx).
// It runs `refresh` on a context-aware ticker until the returned state is in
// `target` (success), is not in `pending` (unexpected — fail), or `timeout`
// elapses. Returns the final value (so the caller can extract API response
// details) or an error.
func waitForState(
	ctx context.Context,
	refresh func() (any, string, error),
	pending, target []string,
	pollInterval, timeout time.Duration,
) (any, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		v, state, err := refresh()
		if err != nil {
			return v, err
		}
		if slices.Contains(target, state) {
			return v, nil
		}
		if !slices.Contains(pending, state) {
			return v, fmt.Errorf("unexpected state %q (pending=%v, target=%v)", state, pending, target)
		}
		if time.Now().After(deadline) {
			return v, fmt.Errorf("timeout after %s waiting for %v (last state=%q)", timeout, target, state)
		}
		select {
		case <-ctx.Done():
			return v, ctx.Err()
		case <-ticker.C:
		}
	}
}

// RetryConfig contains configuration for retrying host operations.
type RetryConfig struct {
	stateConf          *stateChangeConf
	operationName      string
	operation          RetryOperationFunc
	successStatusCodes []int
	ctx                context.Context
}

// RetryResult contains the result of a retry operation.
type RetryResult struct {
	Body  []byte
	Diags diag.Diagnostics
	State string
}

const (
	// DefaultRetryTimeout is the overall timeout for retry operations (seconds) Default: 30min.
	DefaultRetryTimeout = 1800

	// DefaultRetryDelay is the time to wait between retries (seconds).
	DefaultRetryDelay = 5

	// DefaultRetryInitialDelay is the initial delay before first retry (seconds).
	DefaultRetryInitialDelay = 2

	// RetryStateError represents an error state in retry operations.
	RetryStateError = "error"
	// RetryStateRetrying represents a retrying state in retry operations.
	RetryStateRetrying = "retrying"
	// RetryStateSuccess represents a success state in retry operations.
	RetryStateSuccess = "success"
)

var (
	// DefaultRetrySuccessStatusCodes contains success status codes for retry operations.
	DefaultRetrySuccessStatusCodes = []int{http.StatusAccepted, http.StatusNoContent}

	// DefaultRetryableStatusCodes contains retryable status codes for retry operations
	// Common retryable scenarios based on RFC 7231 and industry standards:
	// - HTTP 409: Resource conflict (host in use by running jobs)
	// - HTTP 408: Request timeout
	// - HTTP 429: Too many requests (rate limiting)
	// - HTTP 500: Internal server error
	// - HTTP 502: Bad gateway
	// - HTTP 503: Service unavailable
	// - HTTP 504: Gateway timeout
	//
	// HTTP 403: Forbidden
	// Acceptance tests run against AAP 2.4 almost always receives a
	// 403 upon first host deletion attempt. This is likely an invalid
	// response code from AAP 2.4 and the real error is not known.
	DefaultRetryableStatusCodes = []int{http.StatusConflict, http.StatusRequestTimeout,
		http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusForbidden}
)

// SafeDurationFromSeconds safely converts seconds to time.Duration, checking for overflow.
func SafeDurationFromSeconds(seconds int64) (time.Duration, error) {
	// Maximum duration in seconds for int64 is roughly 292 years
	const maxDurationSeconds = math.MaxInt64 / int64(time.Second)
	if seconds < 0 {
		return 0, fmt.Errorf("duration must be non-negative, got: %d seconds", seconds)
	}
	if seconds > maxDurationSeconds {
		return 0, fmt.Errorf("duration overflow: %d seconds exceeds maximum allowed duration", seconds)
	}

	return time.Duration(seconds) * time.Second, nil
}

// CreateRetryConfig creates a RetryConfig wrapping the framework-compatible
// stateChangeConf object. Behaviourally equivalent to the SDKv2 version that
// previously wrapped helper/retry.StateChangeConf.
func CreateRetryConfig(ctx context.Context, operationName string, operation RetryOperationFunc,
	successStatusCodes []int, retryableStatusCodes []int, retryTimeout int64, initialDelay int64,
	retryDelay int64) (*RetryConfig, diag.Diagnostics) {
	const unableRetryMsg = "Unable to retry"
	var diags diag.Diagnostics

	if operation == nil {
		diags.AddError(
			"Error configuring retry",
			"Retry function is not defined",
		)
		return nil, diags
	}

	if len(successStatusCodes) == 0 {
		successStatusCodes = DefaultRetrySuccessStatusCodes
	}
	if len(retryableStatusCodes) == 0 {
		retryableStatusCodes = DefaultRetryableStatusCodes
	}

	// Check for overflow when converting to time.Duration
	timeoutDuration, err := SafeDurationFromSeconds(retryTimeout)
	if err != nil {
		diags.AddError(
			unableRetryMsg,
			fmt.Sprintf("invalid retry timeout: %s", err.Error()),
		)
	}
	retryDelayDuration, err := SafeDurationFromSeconds(retryDelay)
	if err != nil {
		diags.AddError(
			unableRetryMsg,
			fmt.Sprintf("invalid retry delay: %s", err.Error()),
		)
	}
	initialDelayDuration, err := SafeDurationFromSeconds(initialDelay)
	if err != nil {
		diags.AddError(
			unableRetryMsg,
			fmt.Sprintf("invalid initial delay: %s", err.Error()))
	}
	if diags.HasError() {
		return nil, diags
	}

	result := &RetryResult{}
	stateConf := &stateChangeConf{
		Pending: []string{RetryStateRetrying},
		Target:  []string{RetryStateSuccess},
		Refresh: func() (any, string, error) {
			body, diags, statusCode := operation()
			result.Body = body

			if slices.Contains(retryableStatusCodes, statusCode) {
				result.State = RetryStateRetrying
				return result, RetryStateRetrying, nil // Keep retrying
			}
			if slices.Contains(successStatusCodes, statusCode) {
				result.State = RetryStateSuccess
				return result, RetryStateSuccess, nil
			}

			// If status code is not retryable append the error returned
			result.Diags.Append(diags...)
			return result, RetryStateError, fmt.Errorf("non-retryable HTTP status %d for %s", statusCode, operationName)
		},
		Timeout:    timeoutDuration,
		MinTimeout: retryDelayDuration,
		Delay:      initialDelayDuration,
	}

	return &RetryConfig{
		stateConf:          stateConf,
		operationName:      operationName,
		operation:          operation,
		successStatusCodes: successStatusCodes,
		ctx:                ctx,
	}, diags
}

// RetryWithConfig executes a retry operation with the provided configuration.
// Replaces the previous retry.StateChangeConf.WaitForStateContext call with the
// inline waitForState ticker loop documented in the migration skill.
func RetryWithConfig(retryConfig *RetryConfig) (*RetryResult, error) {
	if retryConfig == nil {
		return nil, fmt.Errorf("retry configuration cannot be nil")
	}
	if retryConfig.stateConf == nil {
		return nil, fmt.Errorf("retry operation '%s': state configuration is not initialized", retryConfig.operationName)
	}
	if retryConfig.ctx == nil {
		return nil, fmt.Errorf("retry operation '%s': context cannot be nil", retryConfig.operationName)
	}
	if !IsContextActive(retryConfig.ctx, retryConfig.operationName, nil) {
		return nil, fmt.Errorf("retry operation '%s': context is not active", retryConfig.operationName)
	}

	conf := retryConfig.stateConf

	// Honour Delay (initial delay before first refresh) the same way
	// retry.StateChangeConf used to. Cancel cleanly if ctx is done.
	if conf.Delay > 0 {
		timer := time.NewTimer(conf.Delay)
		select {
		case <-retryConfig.ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("retry operation '%s' failed: %w", retryConfig.operationName, retryConfig.ctx.Err())
		case <-timer.C:
		}
	}

	// MinTimeout maps onto the ticker's poll interval. Guard against zero so
	// time.NewTicker doesn't panic.
	pollInterval := conf.MinTimeout
	if pollInterval <= 0 {
		pollInterval = time.Second
	}

	result, err := waitForState(
		retryConfig.ctx,
		conf.Refresh,
		conf.Pending,
		conf.Target,
		pollInterval,
		conf.Timeout,
	)
	if err != nil {
		return nil, fmt.Errorf("retry operation '%s' failed: %w", retryConfig.operationName, err)
	}

	if retryresult, ok := result.(*RetryResult); ok {
		if retryresult.Diags.HasError() && retryresult.State != RetryStateError {
			return retryresult, fmt.Errorf("retry operation '%s' returned errors with retry state '%s'", retryConfig.operationName, retryresult.State)
		}
		return retryresult, nil
	}

	return nil, fmt.Errorf("retry operation '%s' returned unexpected result type: %T (expected *RetryResult)", retryConfig.operationName, result)
}
