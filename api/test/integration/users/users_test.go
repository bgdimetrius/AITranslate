//go:build integration

package users_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/bgdimetrius/AITranslate/internal/auth"
	"github.com/bgdimetrius/AITranslate/internal/users"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

const usersIntegrationJWTSecret = "01234567890123456789012345678901"

type usersRouteFixture struct {
	echo *echo.Echo
	conn *pgx.Conn
}

// TestUserRoutes verifies installation provisioning, persisted session state,
// and database failures through real HTTP and PostgreSQL components.
func TestUserRoutes(t *testing.T) {
	t.Run("creates an installation with its initial session", func(t *testing.T) {
		fixture := newUsersRouteFixture(t)
		response := fixture.request()
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusCreated)
		}
		if response.Body.Len() != 0 {
			t.Fatalf("body = %q; want empty body", response.Body.String())
		}

		cookies := usersCookiesByName(response.Result().Cookies())
		accessCookie := cookies["access_token"]
		refreshCookie := cookies["refresh_token"]
		if accessCookie == nil || refreshCookie == nil {
			t.Fatalf("cookies = %#v; want access and refresh cookies", cookies)
		}
		if !accessCookie.HttpOnly || !refreshCookie.HttpOnly {
			t.Fatal("initial session cookies are not HttpOnly")
		}

		var installationID int64
		var storedRefresh string
		err := fixture.conn.QueryRow(context.Background(), `
			SELECT installations.id, auth.refresh_token
			FROM installations
			JOIN auth ON auth.installation_id = installations.id
		`).Scan(&installationID, &storedRefresh)
		if err != nil {
			t.Fatalf("read provisioned installation: %v", err)
		}
		if installationID <= 0 {
			t.Fatalf("installation ID = %d; want a generated positive ID", installationID)
		}
		if storedRefresh != hashUsersToken(refreshCookie.Value) {
			t.Fatal("stored refresh hash does not match the returned refresh token")
		}
		if storedRefresh == refreshCookie.Value {
			t.Fatal("database contains the raw refresh token")
		}
	})

	t.Run("returns an internal error without cookies when persistence is unavailable", func(t *testing.T) {
		fixture := newUsersRouteFixture(t)
		if err := fixture.conn.Close(context.Background()); err != nil {
			t.Fatalf("close PostgreSQL connection: %v", err)
		}

		response := fixture.request()
		requireUsersErrorResponse(t, response, http.StatusInternalServerError, pkg.SmthWW)
		if cookies := response.Result().Cookies(); len(cookies) != 0 {
			t.Fatalf("cookies = %#v; want none", cookies)
		}
	})
}

func newUsersRouteFixture(t *testing.T) *usersRouteFixture {
	t.Helper()
	conn := startUsersPostgres(t)
	db := queries.New(conn)
	auth.Init(usersIntegrationJWTSecret, false)

	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("logger", zap.NewNop())
			return next(c)
		}
	})
	users.NewUserHandler(db).RegisterRoutes(e.Group(""))
	return &usersRouteFixture{echo: e, conn: conn}
}

func startUsersPostgres(t *testing.T) *pgx.Conn {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	migrations, err := filepath.Abs(filepath.Join("..", "..", "..", "docs", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations path: %v", err)
	}
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ai_translate_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithOrderedInitScripts(
			filepath.Join(migrations, "000001_initial_schema.up.sql"),
			filepath.Join(migrations, "000002_add_access_token_id.up.sql"),
		),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { testcontainers.TerminateContainer(container) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get postgres connection string: %v", err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("close postgres connection: %v", err)
		}
	})
	return conn
}

func (fixture *usersRouteFixture) request() *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	fixture.echo.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/users/new", nil))
	return response
}

func usersCookiesByName(cookies []*http.Cookie) map[string]*http.Cookie {
	result := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}

func hashUsersToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func requireUsersErrorResponse(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), status)
	}
	var body pkg.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode JSON %q: %v", response.Body.Bytes(), err)
	}
	if body.Message != message {
		t.Fatalf("error message = %q; want %q", body.Message, message)
	}
}
