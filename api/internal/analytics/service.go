package analytics

import (
	"context"
	"net/mail"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"go.uber.org/zap"
)

func assert(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}

// SaveApplicationFeedback persists feedback for an active installation. It
// panics when called with data that has not passed the handler's validation.
func (as *analyticsservice) SaveApplicationFeedback(ctx context.Context, log *zap.Logger, userId int, r rating) error {
	assert(ctx != nil, "")
	assert(log != nil, "")
	assert(func() bool {
		ok, err := as.db.ValidateUserId(ctx, int64(userId))
		if err != nil {
			log.Error("db query returned error", zap.Error(err))
			return false
		}
		return ok
	}(), "")
	assert(r.Rating == -1 || r.Rating == 1, "")
	assert(len(r.Text) <= 128, "")

	err := as.db.SaveApplicationFeedback(ctx, queries.SaveApplicationFeedbackParams{
		InstallationID: int64(userId),
		Rating:         int16(r.Rating),
		FeedbackText:   r.Text,
	})
	if err != nil {
		log.Error("db query returned error", zap.Error(err))
		return err
	}

	return nil
}

// SaveFeedbackLead persists a syntactically valid plain email address for an
// active installation. It panics if its caller violates those preconditions.
func (as *analyticsservice) SaveFeedbackLead(ctx context.Context, log *zap.Logger, userId int, email string) error {
	assert(ctx != nil, "")
	assert(log != nil, "")
	assert(func() bool {
		ok, err := as.db.ValidateUserId(ctx, int64(userId))
		if err != nil {
			log.Error("db query returned error", zap.Error(err))
			return false
		}
		return ok
	}(), "")
	assert(func() bool {
		address, err := mail.ParseAddress(email)
		return err == nil && address.Address == email
	}(), "")

	err := as.db.SaveFeedbackLead(ctx, queries.SaveFeedbackLeadParams{
		InstallationID: int64(userId),
		Email:          email,
	})
	if err != nil {
		log.Error("db query returned error", zap.Error(err))
		return err
	}

	return nil
}
