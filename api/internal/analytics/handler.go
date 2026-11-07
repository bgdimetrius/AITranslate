// Package analytics accepts product feedback and prospective-customer leads.
package analytics

import (
	"net/http"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/bytedance/sonic"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// ApplicationFeedback accepts only +1 or -1 ratings and limits the
// optional explanation to 128 bytes before persistence.
// @Summary Record application feedback
// @Description Stores a +1 or -1 rating for the authenticated installation. The optional text is limited to 128 bytes.
// @ID saveApplicationFeedback
// @Tags analytics
// @Accept json
// @Produce json
// @Security CookieAuth
// @Param X-Request-ID header string false "Caller-supplied correlation ID; generated when omitted"
// @Param request body rating true "Rating and optional feedback text"
// @Success 204 "Feedback recorded"
// @Failure 400 {object} pkg.ErrorResponse "Malformed JSON, unsupported rating, or feedback text over 128 bytes"
// @Failure 401 {object} pkg.ErrorResponse "Missing, expired, or invalid access_token cookie"
// @Failure 413 {object} pkg.ErrorResponse "Request body exceeds the configured global limit"
// @Failure 429 {object} pkg.ErrorResponse "Per-IP rate limit exceeded"
// @Failure 500 {object} pkg.ErrorResponse "Session validation or feedback persistence failed"
// @Header all {string} X-Request-ID "Request correlation ID"
// @Header all {integer} X-RateLimit-Limit "Per-IP burst limit"
// @Header all {integer} X-RateLimit-Remaining "Requests remaining in the current limiter state"
// @Header 429 {integer} Retry-After "Seconds until another request may be attempted"
// @Router /analytics/feedback [put]
func (ah *analyticshandler) ApplicationFeedback(c *echo.Context) error {
	log := helper.GetLogger(c)
	userId := c.Get("user_id").(int)

	var r rating
	err := sonic.ConfigDefault.NewDecoder(c.Request().Body).Decode(&r)
	if err != nil {
		log.Warn("couldn't unmarshal input json", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
	}
	validated, err := validation.ValidateApplicationFeedback(r.Rating, r.Text)
	if err != nil {
		log.Warn("input data is invalid", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput).Wrap(err)
	}
	r = rating{Rating: validated.Rating, Text: validated.Text}

	err = ah.service.SaveApplicationFeedback(c.Request().Context(), log, userId, r)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	return c.NoContent(http.StatusNoContent)
}

// RegisterFeedbackLead stores a plain email address after trimming whitespace;
// display-name forms such as "Name <user@example.com>" are rejected.
// @Summary Register a feedback lead
// @Description Stores a plain email address for the authenticated installation. Surrounding whitespace is trimmed; display-name address forms are rejected.
// @ID registerFeedbackLead
// @Tags analytics
// @Accept json
// @Produce json
// @Security CookieAuth
// @Param X-Request-ID header string false "Caller-supplied correlation ID; generated when omitted"
// @Param request body email true "Email address to register"
// @Success 204 "Email address recorded"
// @Failure 400 {object} pkg.ErrorResponse "Malformed JSON or invalid email address"
// @Failure 401 {object} pkg.ErrorResponse "Missing, expired, or invalid access_token cookie"
// @Failure 413 {object} pkg.ErrorResponse "Request body exceeds the configured global limit"
// @Failure 429 {object} pkg.ErrorResponse "Per-IP rate limit exceeded"
// @Failure 500 {object} pkg.ErrorResponse "Session validation or lead persistence failed"
// @Header all {string} X-Request-ID "Request correlation ID"
// @Header all {integer} X-RateLimit-Limit "Per-IP burst limit"
// @Header all {integer} X-RateLimit-Remaining "Requests remaining in the current limiter state"
// @Header 429 {integer} Retry-After "Seconds until another request may be attempted"
// @Router /analytics/feedback/order [put]
func (ah *analyticshandler) RegisterFeedbackLead(c *echo.Context) error {
	log := helper.GetLogger(c)
	userId := c.Get("user_id").(int)

	var e email
	err := sonic.ConfigDefault.NewDecoder(c.Request().Body).Decode(&e)
	if err != nil {
		log.Warn("couldn't unmarshal input json", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
	}

	e.Email, err = validation.ValidateEmail(e.Email)
	if err != nil {
		log.Warn("can't recognize string as an email address", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput).Wrap(err)
	}

	err = ah.service.SaveFeedbackLead(c.Request().Context(), log, userId, e.Email)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	return c.NoContent(http.StatusNoContent)
}

// TranslationFeedback validates and persists feedback about a completed
// translation.
// @Summary Record translation feedback
// @Description Upserts one current assessment per installation and translation ID. A higher revision replaces an older assessment; duplicate or stale revisions are accepted without changing stored feedback.
// @ID saveTranslationFeedback
// @Tags analytics
// @Accept json
// @Produce json
// @Security CookieAuth
// @Param X-Request-ID header string false "Caller-supplied correlation ID; generated when omitted"
// @Param request body translationfeedback true "Feedback about a completed translation"
// @Success 204 "Translation feedback recorded"
// @Failure 400 {object} pkg.ErrorResponse "Malformed JSON, invalid translation ID or revision, unsupported provider or language, invalid assessment, or text over the configured limit"
// @Failure 401 {object} pkg.ErrorResponse "Missing, expired, or invalid access_token cookie"
// @Failure 413 {object} pkg.ErrorResponse "Request body exceeds the configured global limit"
// @Failure 429 {object} pkg.ErrorResponse "Per-IP rate limit exceeded"
// @Failure 500 {object} pkg.ErrorResponse "Authenticated installation is missing from request context or persistence failed"
// @Header all {string} X-Request-ID "Request correlation ID"
// @Header all {integer} X-RateLimit-Limit "Per-IP burst limit"
// @Header all {integer} X-RateLimit-Remaining "Requests remaining in the current limiter state"
// @Header 429 {integer} Retry-After "Seconds until another request may be attempted"
// @Router /analytics/feedback/translation [put]
func (ah *analyticshandler) TranslationFeedback(c *echo.Context) error {
	log := helper.GetLogger(c)
	var request translationfeedback
	if err := sonic.ConfigDefault.NewDecoder(c.Request().Body).Decode(&request); err != nil {
		log.Warn("couldn't unmarshal input json", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput).Wrap(err)
	}
	if ah.validator == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	validated, err := validation.ValidateTranslationFeedback(ah.validator, validation.TranslationFeedbackInput{
		TranslationID:   request.TranslationID,
		Revision:        request.Revision,
		Provider:        request.Translator,
		SourceLang:      request.SourceLang,
		TargetLang:      request.TargetLang,
		SourceText:      request.SourceText,
		TranslationText: request.TranslationText,
		Assessment:      request.Assessment,
	}, ah.maxTextBytes)
	if err != nil {
		log.Warn("translation feedback is invalid", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput).Wrap(err)
	}
	userID, ok := c.Get(pkg.UserId).(int)
	if !ok || userID <= 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	if err := ah.service.SaveTranslationFeedback(c.Request().Context(), log, userID, validated); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	return c.NoContent(http.StatusNoContent)
}

// RegisterRoutes installs the feedback endpoints behind authentication.
func (ah *analyticshandler) RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc) {
	g.PUT("/analytics/feedback", ah.ApplicationFeedback, authenticate)
	g.PUT("/analytics/feedback/order", ah.RegisterFeedbackLead, authenticate)
	if ah.validator != nil {
		g.PUT("/analytics/feedback/translation", ah.TranslationFeedback, authenticate)
	}
}
