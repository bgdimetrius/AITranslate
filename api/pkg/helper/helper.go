// Package helper contains request-scoped infrastructure shared by API features.
package helper

import (
	"context"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

func assert(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}

// DbHandler is the persistence contract required for usage accounting.
type DbHandler interface {
	RecordDailyUsage(ctx context.Context, params queries.RecordDailyUsageParams) error
	ValidateUserId(ctx context.Context, userId int64) (bool, error)
}

// GetLogger returns the request-scoped logger installed by the request-ID
// middleware. It panics when the middleware invariant has not been established.
func GetLogger(c *echo.Context) *zap.Logger {
	switch log := c.Get("logger").(type) {
	case *zap.Logger:
		return log
	default:
		panic("no logger has been provide into the request context")
	}
}

// RecordDailyUsage adds usage to the installation's current-day counters.
// Accounting is best effort: failures are logged but are not returned to the
// request handler, so they never replace the provider response.
func RecordDailyUsage(ctx context.Context, log *zap.Logger, db DbHandler, userId int, usage *pkg.Usage) {
	assert(ctx != nil, "")
	assert(log != nil, "")
	assert(db != nil, "")
	assert(func() bool {
		ok, err := db.ValidateUserId(ctx, int64(userId))
		if err != nil {
			log.Error("db query returned error", zap.Error(err))
			return false
		}
		return ok
	}(), "")
	assert(usage != nil, "")
	err := db.RecordDailyUsage(ctx, queries.RecordDailyUsageParams{
		InstallationID:         int64(userId),
		TranslationRequests:    usage.TranslationRequests,
		InterpretationRequests: usage.InterpretationRequests,
		FailedRequests:         usage.FailedRequests,
		InputCharacters:        usage.InputCharacters,
		InputTokens:            usage.InputTokens,
		OutputTokens:           usage.OutputTokens,
	})
	if err != nil {
		log.Error("couldn't record daily usage", zap.Error(err))
	}
}
