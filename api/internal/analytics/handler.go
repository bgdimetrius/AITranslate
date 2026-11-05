// Package analytics accepts product feedback and prospective-customer leads.
package analytics

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/bytedance/sonic"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// ApplicationFeedback accepts only positive or negative ratings and limits the
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
	if (r.Rating != -1 && r.Rating != 1) || len(r.Text) > 128 {
		log.Warn("input data is invalid", zap.Any("r", r))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
	}

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

	e.Email = strings.TrimSpace(e.Email)
	address, err := mail.ParseAddress(e.Email)
	if err != nil || address.Address != e.Email {
		log.Warn("can't recognize string as an email address", zap.Error(err), zap.String("email", e.Email))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
	}

	err = ah.service.SaveFeedbackLead(c.Request().Context(), log, userId, e.Email)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	return c.NoContent(http.StatusNoContent)
}

// RegisterRoutes installs the feedback endpoints behind authentication.
func (ah *analyticshandler) RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc) {
	g.PUT("/analytics/feedback", ah.ApplicationFeedback, authenticate)
	g.PUT("/analytics/feedback/order", ah.RegisterFeedbackLead, authenticate)
}
