// Package users provisions anonymous installations and their initial sessions.
package users

import (
	"net/http"

	"github.com/bgdimetrius/AITranslate/internal/auth"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// NewUser creates an anonymous installation and issues its initial session only
// after the installation has been persisted.
// @Summary Create an anonymous installation
// @Description Creates an installation record and starts its first session. Both tokens are returned as HttpOnly Set-Cookie headers.
// @ID createInstallation
// @Tags users
// @Produce json
// @Param X-Request-ID header string false "Caller-supplied correlation ID; generated when omitted"
// @Success 201 "Installation created; no response body"
// @Failure 413 {object} pkg.ErrorResponse "Request body exceeds the configured global limit"
// @Failure 429 {object} pkg.ErrorResponse "Per-IP rate limit exceeded"
// @Failure 500 {object} pkg.ErrorResponse "Installation creation or initial session creation failed"
// @Header 201 {string} Set-Cookie "New access_token and refresh_token cookies"
// @Header all {string} X-Request-ID "Request correlation ID"
// @Header all {integer} X-RateLimit-Limit "Per-IP burst limit"
// @Header all {integer} X-RateLimit-Remaining "Requests remaining in the current limiter state"
// @Header 429 {integer} Retry-After "Seconds until another request may be attempted"
// @Router /users/new [post]
func (uh *userhandler) NewUser(c *echo.Context) error {
	log := helper.GetLogger(c)
	userId, err := uh.db.NewUser(c.Request().Context())
	if err != nil {
		log.Error("db error", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	err = auth.StartSession(c, uh.db, int(userId))
	if err != nil {
		log.Error("could not create session", zap.Error(err))
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			pkg.SmthWW,
		)
	}

	return c.NoContent(http.StatusCreated)
}

// RegisterRoutes installs the unauthenticated installation-provisioning
// endpoint.
func (uh *userhandler) RegisterRoutes(g *echo.Group) {
	g.POST("/users/new", uh.NewUser)
}
