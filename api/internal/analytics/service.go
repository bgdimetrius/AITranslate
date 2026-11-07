package analytics

import (
	"context"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"go.uber.org/zap"
)

func assert(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}

// SaveApplicationFeedback persists validated feedback for an active
// installation. It panics if its preconditions fail or the installation cannot
// be validated; persistence errors are returned.
func (as *analyticsservice) SaveApplicationFeedback(ctx context.Context, log *zap.Logger, userId int, r rating) error {
	validation.MustNotNil("context", ctx)
	validation.MustNotNil("logger", log)
	validation.MustPositiveInt("user ID", userId)
	assert(func() bool {
		ok, err := as.db.ValidateUserId(ctx, int64(userId))
		if err != nil {
			log.Error("db query returned error", zap.Error(err))
			return false
		}
		return ok
	}(), "")
	if _, err := validation.ValidateApplicationFeedback(r.Rating, r.Text); err != nil {
		panic(err)
	}

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
// active installation. It panics if its preconditions fail or the installation
// cannot be validated; persistence errors are returned.
func (as *analyticsservice) SaveFeedbackLead(ctx context.Context, log *zap.Logger, userId int, email string) error {
	validation.MustNotNil("context", ctx)
	validation.MustNotNil("logger", log)
	validation.MustPositiveInt("user ID", userId)
	assert(func() bool {
		ok, err := as.db.ValidateUserId(ctx, int64(userId))
		if err != nil {
			log.Error("db query returned error", zap.Error(err))
			return false
		}
		return ok
	}(), "")
	validatedEmail, err := validation.ValidateEmail(email)
	if err != nil {
		panic(err)
	}

	err = as.db.SaveFeedbackLead(ctx, queries.SaveFeedbackLeadParams{
		InstallationID: int64(userId),
		Email:          validatedEmail,
	})
	if err != nil {
		log.Error("db query returned error", zap.Error(err))
		return err
	}

	return nil
}

// SaveTranslationFeedback persists already validated translation feedback for
// an active installation. It panics if its preconditions fail or the
// installation cannot be validated; persistence errors are returned.
func (as *analyticsservice) SaveTranslationFeedback(ctx context.Context, log *zap.Logger, userID int, feedback validation.TranslationFeedback) error {
	validation.MustNotNil("context", ctx)
	validation.MustNotNil("logger", log)
	validation.MustPositiveInt("user ID", userID)
	assert(func() bool {
		ok, err := as.db.ValidateUserId(ctx, int64(userID))
		if err != nil {
			log.Error("db query returned error", zap.Error(err))
			return false
		}
		return ok
	}(), "")

	err := as.db.SaveTranslationFeedback(ctx, queries.SaveTranslationFeedbackParams{
		InstallationID:  int64(userID),
		TranslationID:   feedback.TranslationID,
		Revision:        feedback.Revision,
		Provider:        string(feedback.Languages.Provider),
		SourceLang:      feedback.Languages.Source,
		TargetLang:      feedback.Languages.Target,
		SourceText:      feedback.SourceText,
		TranslationText: feedback.TranslationText,
		Assessment:      feedback.Assessment,
	})
	if err != nil {
		log.Error("db query returned error", zap.Error(err))
		return err
	}

	return nil
}
