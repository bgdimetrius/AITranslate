// Package interpretate analyzes short text and produces contextual meanings
// through the configured language model.
package interpretate

import (
	"net/http"
	"strings"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/bytedance/sonic"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const maxWords = 21

func validateInputText(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := helper.GetLogger(c)
		text := c.QueryParam("text")
		if text == "" {
			log.Debug("text query param is empty")
			var textr textReq
			err := sonic.ConfigDefault.NewDecoder(c.Request().Body).Decode(&textr)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
			}
			text = textr.Text
		}

		words := len(strings.Fields(text))
		if words == 0 || words > maxWords {
			log.Warn("text param is too big", zap.String("text", text))
			return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
		}

		c.Set("text", text)
		return next(c)
	}
}

// Interpretate returns structured meanings for middleware-validated text and
// records provider usage on a best-effort basis.
// @Summary Interpret short text
// @Description Authenticates the installation and validates target_lang before invoking the model. Text may contain 1-21 whitespace-delimited words.
// @Description A non-empty text query parameter takes precedence; otherwise text is decoded from the optional JSON body.
// @ID interpretText
// @Tags interpretation
// @Accept json
// @Produce json
// @Security CookieAuth
// @Param X-Request-ID header string false "Caller-supplied correlation ID; generated when omitted"
// @Param target_lang query string true "Supported target language code"
// @Param text query string false "Text to interpret; required when the JSON body is omitted"
// @Param request body textReq false "Alternative text source; ignored when the text query parameter is non-empty"
// @Success 200 {object} interpretation "Structured interpretation"
// @Failure 400 {object} pkg.ErrorResponse "Unsupported target language, malformed body, or text outside the 1-21 word limit"
// @Failure 401 {object} pkg.ErrorResponse "Missing, expired, or invalid access_token cookie"
// @Failure 413 {object} pkg.ErrorResponse "Request body exceeds the configured global limit"
// @Failure 429 {object} pkg.ErrorResponse "Per-IP rate limit exceeded"
// @Failure 500 {object} pkg.ErrorResponse "Session validation failed, or the provider failed or returned an invalid interpretation"
// @Failure 504 {object} pkg.ErrorResponse "Interpretation exceeded the request deadline"
// @Header all {string} X-Request-ID "Request correlation ID"
// @Header all {integer} X-RateLimit-Limit "Per-IP burst limit"
// @Header all {integer} X-RateLimit-Remaining "Requests remaining in the current limiter state"
// @Header 429 {integer} Retry-After "Seconds until another request may be attempted"
// @Router /interpretate [get]
func (inth *interphandler) Interpretate(c *echo.Context) error {
	log := helper.GetLogger(c)
	inr := new(interpReq)
	// Route middleware establishes these typed context values before the
	// handler runs. Direct calls must preserve the same invariant.
	inr.Text = c.Get("text").(string)
	inr.UserId = c.Get("user_id").(int)
	inr.TargetLang = c.Get("target_lang").(string)

	interp, usage, err := inth.service.Interpretate(c.Request().Context(), log, inr)
	usage.InputCharacters = int64(len([]rune(inr.Text)))

	defer helper.RecordDailyUsage(c.Request().Context(), log, inth.db, inr.UserId, &usage)

	if err != nil {
		usage.FailedRequests = 1
		if helper.IsTimeout(c.Request().Context(), err) {
			return echo.NewHTTPError(http.StatusGatewayTimeout, pkg.RequestTimedOut).Wrap(err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	usage.InterpretationRequests = 1

	return c.JSON(http.StatusOK, interp)
}

// AnalyzeText reports suitability through status codes: 200 for suitable text
// and 406 for text the model rejects.
// @Summary Analyze text suitability
// @Description Authenticates the installation, accepts 1-21 whitespace-delimited words, and asks the model whether the text is suitable for interpretation.
// @Description A non-empty text query parameter takes precedence; otherwise text is decoded from the optional JSON body.
// @ID analyzeText
// @Tags interpretation
// @Accept json
// @Produce json
// @Security CookieAuth
// @Param X-Request-ID header string false "Caller-supplied correlation ID; generated when omitted"
// @Param text query string false "Text to analyze; required when the JSON body is omitted"
// @Param request body textReq false "Alternative text source; ignored when the text query parameter is non-empty"
// @Success 200 "Text is suitable; no response body"
// @Failure 400 {object} pkg.ErrorResponse "Malformed body or text outside the 1-21 word limit"
// @Failure 401 {object} pkg.ErrorResponse "Missing, expired, or invalid access_token cookie"
// @Failure 406 "Text is not suitable; no response body"
// @Failure 413 {object} pkg.ErrorResponse "Request body exceeds the configured global limit"
// @Failure 429 {object} pkg.ErrorResponse "Per-IP rate limit exceeded"
// @Failure 500 {object} pkg.ErrorResponse "Session validation failed, or the provider failed or returned a value other than true or false"
// @Failure 504 {object} pkg.ErrorResponse "Analysis exceeded the request deadline"
// @Header all {string} X-Request-ID "Request correlation ID"
// @Header all {integer} X-RateLimit-Limit "Per-IP burst limit"
// @Header all {integer} X-RateLimit-Remaining "Requests remaining in the current limiter state"
// @Header 429 {integer} Retry-After "Seconds until another request may be attempted"
// @Router /interpretate/analyze [post]
func (inth *interphandler) AnalyzeText(c *echo.Context) error {
	log := helper.GetLogger(c)
	text := c.Get("text").(string)
	userId := c.Get("user_id").(int)

	ok, usage, err := inth.service.AnalyzeText(c.Request().Context(), log, text)
	usage.InputCharacters = int64(len([]rune(text)))

	defer helper.RecordDailyUsage(c.Request().Context(), log, inth.db, userId, &usage)

	if err != nil {
		usage.FailedRequests = 1
		if helper.IsTimeout(c.Request().Context(), err) {
			return echo.NewHTTPError(http.StatusGatewayTimeout, pkg.RequestTimedOut).Wrap(err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}
	usage.InterpretationRequests = 1

	if !ok {
		return c.NoContent(http.StatusNotAcceptable)
	}

	return c.NoContent(http.StatusOK)
}

// RegisterRoutes installs the authenticated interpretation and analysis
// endpoints with their language and text validation middleware.
func (inth *interphandler) RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc) {
	g.GET("/interpretate", inth.Interpretate,
		authenticate,
		languages.UseProvider(languages.ProviderChatGPT),
		inth.languages.ValidateTargetLang, validateInputText,
	)
	g.POST("/interpretate/analyze", inth.AnalyzeText, authenticate, validateInputText)
}
