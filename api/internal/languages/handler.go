// Package languages exposes and validates provider-specific language
// capabilities.
package languages

import (
	"net/http"
	"strings"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// SetProvider records the selected translation provider for downstream
// language validation middleware.
func SetProvider(c *echo.Context, provider Provider) {
	validation.MustNotBlank("provider", string(provider))
	c.Set(pkg.Provider, provider)
}

func requestProvider(c *echo.Context) (Provider, bool) {
	provider, ok := c.Get(pkg.Provider).(Provider)
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

		sourceLangStr, ok := c.Get(pkg.SourceLang).(string)
		if !ok {
			sourceLangStr = c.QueryParam(pkg.SourceLang)
		}

		sourceLang, err := catalog.validator.ValidateSource(provider, sourceLangStr)
		if err != nil {
			log.Warn("source lang is invalid",
				zap.String(pkg.Provider, string(provider)),
				zap.String(pkg.SourceLang, sourceLangStr),
			)
			return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
		}

		if strings.TrimSpace(sourceLang) != "" {
			c.Set(pkg.SourceLang, sourceLang)
		}
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

		targetLangStr, ok := c.Get(pkg.TargetLang).(string)
		if !ok {
			targetLangStr = c.QueryParam(pkg.TargetLang)
		}
		targetLang, err := catalog.validator.ValidateTarget(provider, targetLangStr)
		if err != nil {
			log.Warn("target lang is invalid",
				zap.String(pkg.Provider, string(provider)),
				zap.String(pkg.TargetLang, targetLangStr),
			)
			return echo.NewHTTPError(http.StatusBadRequest, pkg.InvalidInput)
		}

		c.Set(pkg.TargetLang, targetLang)
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
