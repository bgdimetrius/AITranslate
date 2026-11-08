package analytics

import (
	"context"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/bgdimetrius/AITranslate/pkg/tgbot"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// AnalyticsHandler registers the authenticated feedback endpoints.
type AnalyticsHandler interface {
	RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc)
	ApplicationFeedback(c *echo.Context) error
	RegisterFeedbackLead(c *echo.Context) error
	TranslationFeedback(c *echo.Context) error
}

// AnalyticsService validates and persists feedback associated with an
// installation.
type AnalyticsService interface {
	SaveApplicationFeedback(ctx context.Context, log *zap.Logger, userId int, r rating) error
	SaveFeedbackLead(ctx context.Context, log *zap.Logger, userId int, email string) error
	SaveTranslationFeedback(ctx context.Context, log *zap.Logger, userID int, feedback validation.TranslationFeedback) error
}

// DbHandler is the persistence contract required by the analytics service.
type DbHandler interface {
	ValidateUserId(ctx context.Context, userID int64) (bool, error)
	SaveApplicationFeedback(ctx context.Context, params queries.SaveApplicationFeedbackParams) error
	// SaveFeedbackLead reports whether the lead was inserted. Existing leads are
	// successful no-ops so callers can avoid sending duplicate notifications.
	SaveFeedbackLead(ctx context.Context, params queries.SaveFeedbackLeadParams) (bool, error)
	SaveTranslationFeedback(ctx context.Context, params queries.SaveTranslationFeedbackParams) error
}

type analyticshandler struct {
	service      AnalyticsService
	validator    *validation.Catalog
	maxTextBytes int
}

type analyticsservice struct {
	chatId int64
	bot    tgbot.TelegramSender
	db     DbHandler
}

// NewAnalyticsHandler configures the application-feedback and lead routes. Use
// NewAnalyticsHandlerWithValidation to also register translation feedback.
func NewAnalyticsHandler(service AnalyticsService) AnalyticsHandler {
	validation.MustNotNil("analytics service", service)
	return &analyticshandler{service: service}
}

// NewAnalyticsHandlerWithValidation registers the persisted translation-feedback
// flow in addition to the application-feedback and lead flows.
func NewAnalyticsHandlerWithValidation(service AnalyticsService, catalog *languages.Catalog, maxTextBytes int) AnalyticsHandler {
	validation.MustNotNil("analytics service", service)
	validation.MustNotNil("language catalog", catalog)
	validation.MustNotNil("language validator", catalog.Validator())
	validation.MustPositiveInt("analytics translation text limit", maxTextBytes)
	return &analyticshandler{
		service:      service,
		validator:    catalog.Validator(),
		maxTextBytes: maxTextBytes,
	}
}

// NewAnalyticsService panics if db is nil.
func NewAnalyticsService(db DbHandler, chatId int64, bot tgbot.TelegramSender) AnalyticsService {
	validation.MustNotNil("analytics database", db)
	return &analyticsservice{
		db:     db,
		chatId: chatId,
		bot:    bot,
	}
}

type rating struct {
	Rating int    `json:"rating" enums:"-1,1" example:"1"`
	Text   string `json:"text,omitempty" example:"The translation was accurate."`
}

type email struct {
	Email string `json:"email" format:"email" example:"reader@example.com"`
}

type translationfeedback struct {
	TranslationID   string `json:"translation_id" format:"uuid"`
	Revision        int32  `json:"revision" minimum:"1"`
	Translator      string `json:"translator"`
	SourceLang      string `json:"source_lang"`
	TargetLang      string `json:"target_lang"`
	SourceText      string `json:"source_text"`
	TranslationText string `json:"translation_text"`
	Assessment      string `json:"assessment"`
}
