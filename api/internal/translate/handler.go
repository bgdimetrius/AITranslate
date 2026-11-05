// Package translate exposes translation through the configured OpenAI and
// DeepL providers.
package translate

import (
	"net/http"
	"strings"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

func (th *trslhandler) validateInputText(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := helper.GetLogger(c)
		text := c.QueryParam("text")

		if text == "" || len(text) > th.allowedTextLength {
			log.Warn("text is invalid", zap.String("text", text))
			return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
		}

		c.Set("text", text)
		c.Set("allowed_length", th.allowedTextLength)
		return next(c)
	}
}

func (th *trslhandler) validateTranslator(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var translator Translator
		log := helper.GetLogger(c)

		translatorName := strings.ToLower(c.QueryParam("translator"))
		switch translatorName {
		case "chatgpt":
			log.Debug("gpt is the translator", zap.String("name", translatorName))
			translator = th.gpt
			languages.SetProvider(c, languages.ProviderChatGPT)
		case "deepl":
			log.Debug("deepl is the translator", zap.String("name", translatorName))
			translator = th.deepl
			languages.SetProvider(c, languages.ProviderDeepL)
		default:
			log.Warn("unknown translator's name", zap.String("translator_name", translatorName))
			return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
		}

		c.Set("translator", translator)
		return next(c)
	}
}

// Translate delegates to the provider selected by middleware, streams its
// response, and records the operation's usage on a best-effort basis.
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
	tr := new(translationReq)
	// Route middleware establishes these typed context values before the
	// handler runs. Direct calls to Translate must preserve the same invariant.
	tr.UserId = c.Get("user_id").(int)
	sl := c.Get("source_lang")
	if sl != nil {
		tr.withSource = true
		tr.SourceLang = sl.(string)
	}
	tr.TargetLang = c.Get("target_lang").(string)
	tr.Text = c.Get("text").(string)
	tr.allowedLength = c.Get("allowed_length").(int)

	translator := c.Get("translator").(Translator)

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

// RegisterRoutes installs the authenticated translation endpoint and its input
// validation middleware.
func (th *trslhandler) RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc) {
	g.GET("/translate", th.Translate,
		authenticate, th.validateTranslator,
		th.languages.ValidateSourceLang, th.languages.ValidateTargetLang, th.validateInputText,
	)
}
