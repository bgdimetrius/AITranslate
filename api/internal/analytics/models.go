package analytics

import (
	"context"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// AnalyticsHandler registers the authenticated feedback endpoints.
type AnalyticsHandler interface {
	RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc)
	ApplicationFeedback(c *echo.Context) error
	RegisterFeedbackLead(c *echo.Context) error
}

// AnalyticsService validates and persists feedback associated with an
// installation.
type AnalyticsService interface {
	SaveApplicationFeedback(ctx context.Context, log *zap.Logger, userId int, r rating) error
	SaveFeedbackLead(ctx context.Context, log *zap.Logger, userId int, email string) error
}

// DbHandler is the persistence contract required by the analytics service.
type DbHandler interface {
	ValidateUserId(ctx context.Context, userID int64) (bool, error)
	SaveApplicationFeedback(ctx context.Context, params queries.SaveApplicationFeedbackParams) error
	SaveFeedbackLead(ctx context.Context, params queries.SaveFeedbackLeadParams) error
}

type analyticshandler struct {
	service AnalyticsService
}

type analyticsservice struct {
	db DbHandler
}

// NewAnalyticsHandler returns an HTTP handler backed by service.
func NewAnalyticsHandler(service AnalyticsService) AnalyticsHandler {
	return &analyticshandler{service: service}
}

// NewAnalyticsService returns a feedback service backed by db.
func NewAnalyticsService(db DbHandler) AnalyticsService {
	return &analyticsservice{db: db}
}

type rating struct {
	Rating int    `json:"rating" enums:"-1,1" example:"1"`
	Text   string `json:"text,omitempty" example:"The translation was accurate."`
}

type email struct {
	Email string `json:"email" format:"email" example:"reader@example.com"`
}
