package analytics

import (
	"context"
	"errors"
	"fmt"
	"html"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// SaveApplicationFeedback persists validated feedback for an active
// installation. It panics if its preconditions fail or the installation cannot
// be validated; persistence errors are returned.
func (as *analyticsservice) SaveApplicationFeedback(ctx context.Context, log *zap.Logger, userId int, r rating) error {
	validation.MustNotNil("context", ctx)
	validation.MustNotNil("logger", log)
	validation.MustPositiveInt("user ID", userId)
	ok, err := as.db.ValidateUserId(ctx, int64(userId))
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Error("db query returned error", zap.Error(err))
		}
		panic(fmt.Errorf("validate installation for application feedback: %w", err))
	}
	if !ok {
		panic("application feedback requires an active installation")
	}
	if _, err := validation.ValidateApplicationFeedback(r.Rating, r.Text); err != nil {
		panic(err)
	}

	err = as.db.SaveApplicationFeedback(ctx, queries.SaveApplicationFeedbackParams{
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

func formatFeedbackLeadNotification(userID int, email string) string {
	return fmt.Sprintf(
		"📬 <b>New feedback lead</b>\n\n"+
			"<b>Email:</b> <code>%s</code>\n"+
			"<b>Installation:</b> <code>%d</code>",
		html.EscapeString(email),
		userID,
	)
}

// SaveFeedbackLead persists a syntactically valid plain email address for an
// active installation. A newly inserted lead triggers one Telegram notification;
// submitting the same lead again is a successful no-op. It panics if its
// preconditions fail, the installation cannot be validated, or Telegram delivery
// fails; persistence errors are returned.
func (as *analyticsservice) SaveFeedbackLead(ctx context.Context, log *zap.Logger, userId int, email string) error {
	validation.MustNotNil("context", ctx)
	validation.MustNotNil("logger", log)
	validation.MustPositiveInt("user ID", userId)
	ok, err := as.db.ValidateUserId(ctx, int64(userId))
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Error("db query returned error", zap.Error(err))
		}
		panic(fmt.Errorf("validate installation for feedback lead: %w", err))
	}
	if !ok {
		panic("feedback lead requires an active installation")
	}
	validatedEmail, err := validation.ValidateEmail(email)
	if err != nil {
		panic(err)
	}

	ok, err = as.db.SaveFeedbackLead(ctx, queries.SaveFeedbackLeadParams{
		InstallationID: int64(userId),
		Email:          validatedEmail,
	})
	if err != nil {
		log.Error("db query returned error", zap.Error(err))
		return err
	}

	if ok {
		message := tgbotapi.NewMessage(as.chatId, formatFeedbackLeadNotification(userId, validatedEmail))
		message.ParseMode = tgbotapi.ModeHTML
		message.DisableWebPagePreview = true
		_, err = as.bot.Send(message)
		if err != nil {
			log.Error("telegram returned error", zap.Error(err))
			panic(err)
		}
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
	ok, err := as.db.ValidateUserId(ctx, int64(userID))
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Error("db query returned error", zap.Error(err))
		}
		panic(fmt.Errorf("validate installation for translation feedback: %w", err))
	}
	if !ok {
		panic("translation feedback requires an active installation")
	}

	err = as.db.SaveTranslationFeedback(ctx, queries.SaveTranslationFeedbackParams{
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
