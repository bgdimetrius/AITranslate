package users

import (
	"context"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
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

// NewUserHandler returns a provisioning handler backed by db.
func NewUserHandler(db DbHandler) UserHandler {
	return &userhandler{db: db}
}

// DbHandler combines installation creation with the operations needed to issue
// its initial session.
type DbHandler interface {
	NewUser(ctx context.Context) (int64, error)

	ValidateUserId(ctx context.Context, userId int64) (bool, error)
	SaveSession(ctx context.Context, params queries.SaveSessionParams) error
}
