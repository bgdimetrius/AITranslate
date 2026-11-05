// Package languages exposes and validates provider-specific language
// capabilities.
package languages

import (
	"net/http"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const providerContextKey = "translation_provider"

// SetProvider records the selected translation provider for downstream
// language validation middleware.
func SetProvider(c *echo.Context, provider Provider) {
	c.Set(providerContextKey, provider)
}

// UseProvider establishes a fixed provider for routes that do not offer a
// provider selector, such as OpenAI-backed interpretation.
func UseProvider(provider Provider) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			SetProvider(c, provider)
			return next(c)
		}
	}
}

func requestProvider(c *echo.Context) (Provider, bool) {
	provider, ok := c.Get(providerContextKey).(Provider)
	return provider, ok
}

// ValidateSourceLang validates source_lang against the provider selected by an
// earlier middleware and stores the provider-normalized code in the context.
func (catalog *Catalog) ValidateSourceLang(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := helper.GetLogger(c)
		provider, ok := requestProvider(c)
		if !ok {
			log.Error("translation provider is missing before language validation")
			return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
		}
		if c.QueryParam("source_lang") == "" && catalog.SupportsAutoDetect(provider) {
			return next(c)
		}

		sourceLang, ok := catalog.ResolveSource(provider, c.QueryParam("source_lang"))
		if !ok {
			log.Warn("source lang is invalid",
				zap.String("provider", string(provider)),
				zap.String("source_lang", c.QueryParam("source_lang")),
			)
			return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
		}

		c.Set("source_lang", sourceLang)
		return next(c)
	}
}

// ValidateTargetLang validates target_lang against the provider selected by an
// earlier middleware and stores the provider-normalized code in the context.
func (catalog *Catalog) ValidateTargetLang(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := helper.GetLogger(c)
		provider, ok := requestProvider(c)
		if !ok {
			log.Error("translation provider is missing before language validation")
			return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
		}

		targetLang, ok := catalog.ResolveTarget(provider, c.QueryParam("target_lang"))
		if !ok {
			log.Warn("target lang is invalid",
				zap.String("provider", string(provider)),
				zap.String("target_lang", c.QueryParam("target_lang")),
			)
			return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
		}

		c.Set("target_lang", targetLang)
		return next(c)
	}
}

// List returns all configured provider capabilities.
// @Summary List translation languages
// @Description Returns source and target language codes separately for every configured translation provider.
// @ID listTranslationLanguages
// @Tags translation
// @Produce json
// @Success 200 {object} map[string]Capabilities
// @Router /languages [get]
func (catalog *Catalog) List(c *echo.Context) error {
	return c.JSON(http.StatusOK, catalog.Snapshot())
}

// RegisterRoutes exposes the public language catalog.
func (catalog *Catalog) RegisterRoutes(g *echo.Group) {
	g.GET("/languages", catalog.List)
}
