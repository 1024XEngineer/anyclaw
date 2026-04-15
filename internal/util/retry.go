package util

import "context"

// Retry calls fn once for now and leaves room for future backoff policies.
func Retry(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
