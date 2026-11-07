package users

import (
	"context"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/labstack/echo/v5"
)

// UserHandler registers the installation-provisioning endpoint.
type UserHandler interface {
	RegisterRoutes(g *echo.Group)
	NewUser(c *echo.Context) error
}

type userhandler struct {
	db DbHandler
}

// NewUserHandler panics if db is nil.
func NewUserHandler(db DbHandler) UserHandler {
	validation.MustNotNil("users database", db)
	return &userhandler{db: db}
}

// DbHandler combines installation creation with the operations needed to issue
// its initial session.
type DbHandler interface {
	NewUser(ctx context.Context) (int64, error)

	ValidateUserId(ctx context.Context, userId int64) (bool, error)
	SaveSession(ctx context.Context, params queries.SaveSessionParams) error
}
