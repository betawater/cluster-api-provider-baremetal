/*
Copyright 2024 The CAPBM Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package retry

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Config defines retry configuration.
type Config struct {
	// MaxRetries is the maximum number of retries.
	MaxRetries int
	
	// Backoff defines the backoff configuration.
	Backoff BackoffConfig
	
	// RetryableErrors is the list of error patterns that are retryable.
	RetryableErrors []string
}

// BackoffConfig defines backoff configuration.
type BackoffConfig struct {
	// Initial is the initial backoff duration.
	Initial time.Duration
	
	// Max is the maximum backoff duration.
	Max time.Duration
	
	// Factor is the backoff multiplication factor.
	Factor float64
}

// DefaultConfig returns a default retry configuration.
func DefaultConfig() Config {
	return Config{
		MaxRetries: 3,
		Backoff: BackoffConfig{
			Initial: 10 * time.Second,
			Max:     5 * time.Minute,
			Factor:  2.0,
		},
		RetryableErrors: []string{
			"connection refused",
			"timeout",
			"etcd server not ready",
			"i/o timeout",
			"no such host",
		},
	}
}

// IsRetryableError checks if an error is retryable based on the error message.
func IsRetryableError(err error, retryableErrors []string) bool {
	if err == nil {
		return false
	}
	
	errMsg := strings.ToLower(err.Error())
	for _, pattern := range retryableErrors {
		if strings.Contains(errMsg, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// WithBackoff executes a function with exponential backoff retry.
func WithBackoff(ctx context.Context, config Config, fn func(ctx context.Context) error) error {
	backoff := config.Backoff.Initial
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		
		// Check if error is retryable
		if !IsRetryableError(err, config.RetryableErrors) {
			return fmt.Errorf("non-retryable error: %w", err)
		}
		
		// Check if max retries exceeded
		if attempt == config.MaxRetries {
			return fmt.Errorf("max retries (%d) exceeded: %w", config.MaxRetries, err)
		}
		
		// Wait with backoff
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(backoff):
			backoff = time.Duration(float64(backoff) * config.Backoff.Factor)
			if backoff > config.Backoff.Max {
				backoff = config.Backoff.Max
			}
		}
	}
	
	return nil
}

// WithFixedInterval executes a function with fixed interval retry.
func WithFixedInterval(ctx context.Context, maxRetries int, interval time.Duration, fn func(ctx context.Context) error) error {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		
		if attempt == maxRetries {
			return fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, err)
		}
		
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
	
	return nil
}
