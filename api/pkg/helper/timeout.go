package helper

import (
	"context"
	"errors"
)

// IsTimeout reports whether either the returned error or the request context
// indicates that the operation exceeded its deadline.
func IsTimeout(ctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) ||
		(ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded))
}
