// Package translate exposes translation through the configured OpenAI and
// DeepL providers.
package translate

import (
	"net/http"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// Translate validates query input, writes the selected provider's response,
// and records usage after the provider attempt.
// @Summary Translate text
// @Description Authenticates the installation, validates both language codes, selects the requested provider, and enforces the configured text-size limit before calling the provider.
// @Description chatgpt streams raw model output and may have already written partial output if the provider fails. deepl returns a JSON object containing source_lang and text.
// @ID translateText
// @Tags translation
// @Produce plain,json
// @Security CookieAuth
// @Param X-Request-ID header string false "Caller-supplied correlation ID; generated when omitted"
// @Param source_lang query string false "Supported source language code; omitted for automatic detection"
// @Param target_lang query string true "Supported target language code"
// @Param translator query string true "Translation provider" Enums(chatgpt,deepl)
// @Param text query string true "Non-empty text within the server-configured byte limit"
// @Success 200 {string} string "chatgpt: streamed text; deepl: JSON-encoded object with source_lang and text"
// @Failure 400 {object} pkg.ErrorResponse "Unsupported language or provider, missing text, or text over the configured limit"
// @Failure 401 {object} pkg.ErrorResponse "Missing, expired, or invalid access_token cookie"
// @Failure 413 {object} pkg.ErrorResponse "Request body exceeds the configured global limit"
// @Failure 429 {object} pkg.ErrorResponse "Per-IP rate limit exceeded"
// @Failure 500 {object} pkg.ErrorResponse "Session validation, translation provider, or response encoding failed"
// @Failure 504 {object} pkg.ErrorResponse "Translation exceeded the request deadline"
// @Header all {string} X-Request-ID "Request correlation ID"
// @Header all {integer} X-RateLimit-Limit "Per-IP burst limit"
// @Header all {integer} X-RateLimit-Remaining "Requests remaining in the current limiter state"
// @Header 429 {integer} Retry-After "Seconds until another request may be attempted"
// @Router /translate [get]
func (th *trslhandler) Translate(c *echo.Context) error {
	log := helper.GetLogger(c)
	input, err := validation.ValidateTranslation(th.validator, validation.TranslationInput{
		Provider:   c.QueryParam("translator"),
		SourceLang: c.QueryParam("source_lang"),
		TargetLang: c.QueryParam("target_lang"),
		Text:       c.QueryParam("text"),
	}, th.allowedTextLength)
	if err != nil {
		log.Warn("translation input is invalid", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput).Wrap(err)
	}

	userID, ok := c.Get(pkg.UserId).(int)
	if !ok || userID <= 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	tr := &translationReq{
		SourceLang:    input.Languages.Source,
		TargetLang:    input.Languages.Target,
		UserId:        userID,
		Text:          input.Text,
		withSource:    input.Languages.Source != "",
		allowedLength: th.allowedTextLength,
	}

	var translator Translator
	switch input.Languages.Provider {
	case validation.ProviderChatGPT:
		translator = th.gpt
	case validation.ProviderDeepL:
		translator = th.deepl
	default:
		return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
	}

	usage, err := translator.Do(c.Request().Context(), log, c.Response(), tr)
	usage.InputCharacters = int64(len([]rune(tr.Text)))

	defer helper.RecordDailyUsage(c.Request().Context(), log, th.db, tr.UserId, &usage)

	if err != nil {
		usage.FailedRequests = 1
		if helper.IsTimeout(c.Request().Context(), err) {
			return echo.NewHTTPError(http.StatusGatewayTimeout, pkg.RequestTimedOut).Wrap(err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	usage.TranslationRequests = 1
	return nil
}

// RegisterRoutes installs the authenticated translation endpoint.
func (th *trslhandler) RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc) {
	g.GET("/translate", th.Translate, authenticate)
}
