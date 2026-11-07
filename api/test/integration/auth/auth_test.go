//go:build integration

package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/bgdimetrius/AITranslate/internal/auth"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

const integrationJWTSecret = "01234567890123456789012345678901"

type routeFixture struct {
	echo          *echo.Echo
	db            *queries.Queries
	userID        int64
	accessCookie  *http.Cookie
	refreshCookie *http.Cookie
}

// TestAuthRoutes verifies atomic rotation, immediate access revocation, replay
// rejection, and documented failures through real JWT, HTTP, and PostgreSQL components.
func TestAuthRoutes(t *testing.T) {
	t.Run("rotates a session with an expired access token", func(t *testing.T) {
		fixture := newRouteFixture(t)
		expiredAccess := &http.Cookie{
			Name:  "access_token",
			Value: signedExpiredAccessToken(t, fixture.userID),
		}

		response := fixture.handshake(expiredAccess, fixture.refreshCookie)
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusCreated)
		}
		cookies := cookiesByName(response.Result().Cookies())
		newAccess := cookies["access_token"]
		newRefresh := cookies["refresh_token"]
		if newAccess == nil || newRefresh == nil {
			t.Fatalf("cookies = %#v; want rotated access and refresh cookies", cookies)
		}
		if newAccess.Value == expiredAccess.Value || newRefresh.Value == fixture.refreshCookie.Value {
			t.Fatal("successful handshake did not rotate both tokens")
		}
		if !newAccess.HttpOnly || !newRefresh.HttpOnly {
			t.Fatal("rotated session cookies are not HttpOnly")
		}

		storedHash, err := fixture.db.GetRefreshToken(context.Background(), fixture.userID)
		if err != nil {
			t.Fatalf("read rotated refresh hash: %v", err)
		}
		if storedHash != hashToken(newRefresh.Value) {
			t.Fatal("stored refresh hash does not match the rotated refresh token")
		}

		oldSessionResponse := fixture.handshake(expiredAccess, fixture.refreshCookie)
		requireErrorResponse(t, oldSessionResponse, http.StatusUnauthorized, "unauthorized")
	})

	t.Run("immediately rejects the superseded access token", func(t *testing.T) {
		fixture := newRouteFixture(t)
		response := fixture.handshake(fixture.accessCookie, fixture.refreshCookie)
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusCreated)
		}
		rotated := cookiesByName(response.Result().Cookies())
		newAccess := rotated["access_token"]
		newRefresh := rotated["refresh_token"]
		if newAccess == nil || newRefresh == nil {
			t.Fatalf("cookies = %#v; want rotated access and refresh cookies", rotated)
		}

		oldAccessResponse := fixture.protected(fixture.accessCookie)
		requireErrorResponse(t, oldAccessResponse, http.StatusUnauthorized, "session expired")
		if currentAccessResponse := fixture.protected(newAccess); currentAccessResponse.Code != http.StatusNoContent {
			t.Fatalf("current access status = %d, body = %s; want %d", currentAccessResponse.Code, currentAccessResponse.Body.String(), http.StatusNoContent)
		}

		replayedRefresh := fixture.handshake(fixture.accessCookie, fixture.refreshCookie)
		requireErrorResponse(t, replayedRefresh, http.StatusUnauthorized, "unauthorized")
	})

	t.Run("accepts only one concurrent refresh", func(t *testing.T) {
		fixture := newRouteFixture(t)
		start := make(chan struct{})
		responses := make(chan *httptest.ResponseRecorder, 2)
		for range 2 {
			go func() {
				<-start
				responses <- fixture.handshake(fixture.accessCookie, fixture.refreshCookie)
			}()
		}
		close(start)

		statusCounts := make(map[int]int, 2)
		for range 2 {
			response := <-responses
			statusCounts[response.Code]++
		}
		if statusCounts[http.StatusCreated] != 1 || statusCounts[http.StatusUnauthorized] != 1 {
			t.Fatalf("concurrent refresh statuses = %#v; want one 201 and one 401", statusCounts)
		}
	})

	t.Run("requires both session cookies", func(t *testing.T) {
		fixture := newRouteFixture(t)
		missingAccess := fixture.handshake(nil, fixture.refreshCookie)
		requireErrorResponse(t, missingAccess, http.StatusBadRequest, "access token is missing")

		missingRefresh := fixture.handshake(fixture.accessCookie, nil)
		requireErrorResponse(t, missingRefresh, http.StatusBadRequest, "refresh token is missing")
	})

	t.Run("rejects a mismatched refresh token", func(t *testing.T) {
		fixture := newRouteFixture(t)
		wrongRefresh := &http.Cookie{Name: "refresh_token", Value: "wrong-refresh-token"}
		response := fixture.handshake(fixture.accessCookie, wrongRefresh)
		requireErrorResponse(t, response, http.StatusUnauthorized, "unauthorized")
		fixture.requireStoredRefresh(t, fixture.refreshCookie.Value)
	})

	t.Run("rejects an invalid access signature", func(t *testing.T) {
		fixture := newRouteFixture(t)
		invalidAccess := &http.Cookie{
			Name:  "access_token",
			Value: signedExpiredAccessTokenWithSecret(t, fixture.userID, "different-signing-secret-12345678"),
		}
		response := fixture.handshake(invalidAccess, fixture.refreshCookie)
		requireErrorResponse(t, response, http.StatusForbidden, "invalid access token")
		fixture.requireStoredRefresh(t, fixture.refreshCookie.Value)
	})
}

func newRouteFixture(t *testing.T) *routeFixture {
	t.Helper()
	pool := startPostgres(t)
	db := queries.New(pool)
	auth.Init(integrationJWTSecret, false)

	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("logger", zap.NewNop())
			return next(c)
		}
	})
	auth.NewAuthHandler(db).RegisterRoutes(e.Group(""))
	e.GET("/protected", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, auth.Authenticate(db))

	userID, err := db.NewUser(context.Background())
	if err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	accessCookie, refreshCookie := startSession(t, e, db, userID)
	return &routeFixture{
		echo:          e,
		db:            db,
		userID:        userID,
		accessCookie:  accessCookie,
		refreshCookie: refreshCookie,
	}
}

func startPostgres(t *testing.T) *pgxpool.Pool {
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
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping PostgreSQL pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})
	return pool
}

func startSession(t *testing.T, e *echo.Echo, db *queries.Queries, userID int64) (*http.Cookie, *http.Cookie) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/session", nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)
	c.Set("logger", zap.NewNop())
	if err := auth.StartSession(c, db, int(userID)); err != nil {
		t.Fatalf("start authenticated session: %v", err)
	}
	cookies := cookiesByName(recorder.Result().Cookies())
	accessCookie := cookies["access_token"]
	refreshCookie := cookies["refresh_token"]
	if accessCookie == nil || refreshCookie == nil {
		t.Fatalf("cookies = %#v; want access and refresh cookies", cookies)
	}
	return accessCookie, refreshCookie
}

func (fixture *routeFixture) handshake(accessCookie, refreshCookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/auth/handshake", nil)
	if accessCookie != nil {
		request.AddCookie(accessCookie)
	}
	if refreshCookie != nil {
		request.AddCookie(refreshCookie)
	}
	recorder := httptest.NewRecorder()
	fixture.echo.ServeHTTP(recorder, request)
	return recorder
}

func (fixture *routeFixture) protected(accessCookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if accessCookie != nil {
		request.AddCookie(accessCookie)
	}
	recorder := httptest.NewRecorder()
	fixture.echo.ServeHTTP(recorder, request)
	return recorder
}

func (fixture *routeFixture) requireStoredRefresh(t *testing.T, rawToken string) {
	t.Helper()
	storedHash, err := fixture.db.GetRefreshToken(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("read stored refresh hash: %v", err)
	}
	if storedHash != hashToken(rawToken) {
		t.Fatal("stored refresh token changed after a rejected handshake")
	}
}

func signedExpiredAccessToken(t *testing.T, userID int64) string {
	t.Helper()
	return signedExpiredAccessTokenWithSecret(t, userID, integrationJWTSecret)
}

func signedExpiredAccessTokenWithSecret(t *testing.T, userID int64, secret string) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "ai_translate_api",
		Audience:  jwt.ClaimStrings{"ai_translate_extension"},
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
		IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Hour)),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign expired access token: %v", err)
	}
	return signed
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func cookiesByName(cookies []*http.Cookie) map[string]*http.Cookie {
	result := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}

func requireErrorResponse(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
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
