// Package auth creates and validates installation sessions for protected API
// routes.
package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const (
	accessTokenCookie  = "access_token"
	refreshTokenCookie = "refresh_token"
)

var (
	jwtSecret  string
	production bool
)

func setTokenCookies(c *echo.Context, accessToken, refreshToken string) {
	sameSite := http.SameSiteLaxMode
	if production {
		sameSite = http.SameSiteNoneMode
	}

	// The access JWT expires after 15 minutes, but its cookie stays available
	// for the refresh lifetime. Handshake verifies the expired JWT signature and
	// uses its subject and refresh token to conditionally rotate the session.
	maxAge := int(refreshTokenTTL.Seconds())
	expires := time.Now().Add(refreshTokenTTL)

	c.SetCookie(&http.Cookie{
		Name:     accessTokenCookie,
		Value:    accessToken,
		Path:     "/",
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   production,
		SameSite: sameSite,
	})
	c.SetCookie(&http.Cookie{
		Name:     refreshTokenCookie,
		Value:    refreshToken,
		Path:     "/",
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   production,
		SameSite: sameSite,
	})
}

// StartSession validates the installation and persists both credentials that
// define its active session before exposing the token pair as HTTP-only cookies.
func StartSession(c *echo.Context, db StartSessionDbHandler, userId int) error {
	validation.MustNotNil("session database", db)
	validation.MustPositiveInt("user ID", userId)
	log := helper.GetLogger(c)

	ok, err := db.ValidateUserId(c.Request().Context(), int64(userId))
	if err != nil {
		log.Error("db query returned error", zap.Error(err))
		return err
	}
	if !ok {
		return errors.New("unknown user id")
	}

	pair, err := generatePair(log, userId, jwtSecret)
	if err != nil {
		return err
	}

	params := queries.SaveSessionParams{
		InstallationID: int64(userId),
		RefreshToken:   hashRefreshToken(pair.refreshToken),
		AccessTokenID:  pair.accessTokenID,
	}
	err = db.SaveSession(c.Request().Context(), params)
	if err != nil {
		log.Error("couldn't save session", zap.Error(err))
		return err
	}

	setTokenCookies(c, pair.accessToken, pair.refreshToken)
	return nil
}

// Authenticate returns middleware that accepts only the access token currently
// recorded for its installation. Rotating a session therefore invalidates the
// previous signed JWT immediately instead of waiting for its expiry.
func Authenticate(db AccessTokenDbHandler) echo.MiddlewareFunc {
	validation.MustNotNil("access token database", db)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			log := helper.GetLogger(c)
			cookie, err := c.Cookie(accessTokenCookie)
			if err != nil {
				log.Warn("access token cookie is missing", zap.Error(err))
				return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
			}

			accessClaims, err := decodeAccessToken(log, cookie.Value, jwtSecret, true)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "session expired")
			}

			userID, err := strconv.Atoi(accessClaims.Subject)
			if err != nil {
				log.Warn("access token contains invalid session claims", zap.Error(err))
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
			}
			if accessClaims.ID == "" {
				log.Warn("access token is missing its session identifier")
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
			}

			activeTokenID, err := db.GetAccessTokenID(c.Request().Context(), int64(userID))
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return echo.NewHTTPError(http.StatusUnauthorized, "session expired")
				}
				log.Error("couldn't load active access token id", zap.Error(err))
				return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
			}
			if subtle.ConstantTimeCompare([]byte(accessClaims.ID), []byte(activeTokenID)) != 1 {
				return echo.NewHTTPError(http.StatusUnauthorized, "session expired")
			}

			c.Set("user_id", userID)
			return next(c)
		}
	}
}

// Handshake atomically rotates both session credentials when the signed access
// token identifies the installation and the presented refresh token is still
// current. The conditional update prevents concurrent refresh-token replay.
// @Summary Refresh an installation session
// @Description Requires access_token and refresh_token cookies. The access token may be expired, but its signature and subject must be valid; the refresh token must match the stored hash.
// @Description On success both cookies are rotated and returned as HttpOnly Set-Cookie headers. The prior refresh token cannot be replayed, and the prior access token immediately stops authorizing protected endpoints.
// @ID refreshSession
// @Tags auth
// @Produce json
// @Security CookieAuth
// @Param X-Request-ID header string false "Caller-supplied correlation ID; generated when omitted"
// @Success 201 "Session refreshed; no response body"
// @Failure 400 {object} pkg.ErrorResponse "One or both session cookies are missing"
// @Failure 401 {object} pkg.ErrorResponse "Refresh token does not match the stored session"
// @Failure 403 {object} pkg.ErrorResponse "Access token signature or subject is invalid"
// @Failure 413 {object} pkg.ErrorResponse "Request body exceeds the configured global limit"
// @Failure 429 {object} pkg.ErrorResponse "Per-IP rate limit exceeded"
// @Failure 500 {object} pkg.ErrorResponse "Token generation or atomic session rotation failed"
// @Header 201 {string} Set-Cookie "Rotated access_token and refresh_token cookies"
// @Header all {string} X-Request-ID "Request correlation ID"
// @Header all {integer} X-RateLimit-Limit "Per-IP burst limit"
// @Header all {integer} X-RateLimit-Remaining "Requests remaining in the current limiter state"
// @Header 429 {integer} Retry-After "Seconds until another request may be attempted"
// @Router /auth/handshake [post]
func (ah *authhandler) Handshake(c *echo.Context) error {
	log := helper.GetLogger(c)

	accessCookie, err := c.Cookie(accessTokenCookie)
	if err != nil {
		log.Warn("access token cookie is missing", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, "access token is missing")
	}

	refreshCookie, err := c.Cookie(refreshTokenCookie)
	if err != nil {
		log.Warn("refresh token cookie is missing", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, "refresh token is missing")
	}

	accessClaims, err := decodeAccessToken(log, accessCookie.Value, jwtSecret, false)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "invalid access token")
	}

	userID, err := strconv.Atoi(accessClaims.Subject)
	if err != nil {
		log.Warn("access token contains an invalid user id", zap.Error(err))
		return echo.NewHTTPError(http.StatusForbidden, "invalid access token")
	}

	pair, err := generatePair(log, userID, jwtSecret)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}

	rowsAffected, err := ah.db.RotateSession(c.Request().Context(), queries.RotateSessionParams{
		NewRefreshToken:      hashRefreshToken(pair.refreshToken),
		NewAccessTokenID:     pair.accessTokenID,
		InstallationID:       int64(userID),
		PreviousRefreshToken: hashRefreshToken(refreshCookie.Value),
	})
	if err != nil {
		log.Error("couldn't rotate refresh token", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW)
	}
	if rowsAffected != 1 {
		log.Warn("refresh token is invalid", zap.Int("user_id", userID))
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	setTokenCookies(c, pair.accessToken, pair.refreshToken)
	return c.NoContent(http.StatusCreated)
}

// RegisterRoutes installs the session-refresh endpoint.
func (ah *authhandler) RegisterRoutes(g *echo.Group) {
	g.POST("/auth/handshake", ah.Handshake)
}

// Init configures process-wide token signing and cookie security. It must be
// called before any session is created or authenticated.
func Init(secret string, isProduction bool) {
	secret = validation.MustNotBlank("JWT secret", secret)
	validation.MustMinBytes("JWT secret", secret, 32)
	jwtSecret = secret
	production = isProduction
}
