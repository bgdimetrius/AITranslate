package auth

import (
	"context"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
)

type claims struct {
	jwt.RegisteredClaims
}

// AuthHandler registers the session-refresh endpoint.
type AuthHandler interface {
	RegisterRoutes(g *echo.Group)
	Handshake(c *echo.Context) error
}

type authhandler struct {
	db DbHandler
}

// NewAuthHandler panics if db is nil.
func NewAuthHandler(db DbHandler) AuthHandler {
	validation.MustNotNil("authentication database", db)
	return &authhandler{db: db}
}

// DbHandler is the persistence contract required by the refresh flow.
type DbHandler interface {
	RotateSession(ctx context.Context, params queries.RotateSessionParams) (int64, error)
}

// StartSessionDbHandler is the narrower persistence contract used when a new
// session is issued.
type StartSessionDbHandler interface {
	ValidateUserId(ctx context.Context, userId int64) (bool, error)
	SaveSession(ctx context.Context, params queries.SaveSessionParams) error
}

// AccessTokenDbHandler supplies the active token ID used to reject access
// tokens superseded by a later session rotation.
type AccessTokenDbHandler interface {
	GetAccessTokenID(ctx context.Context, userId int64) (string, error)
}
