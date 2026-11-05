//go:build integration

package analytics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bgdimetrius/AITranslate/internal/analytics"
	"github.com/bgdimetrius/AITranslate/internal/auth"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

const integrationJWTSecret = "01234567890123456789012345678901"

type routeFixture struct {
	echo         *echo.Echo
	conn         *pgx.Conn
	userID       int64
	accessCookie *http.Cookie
}

// TestAnalyticsRoutes verifies authenticated feedback validation and persistence through real service and PostgreSQL components.
func TestAnalyticsRoutes(t *testing.T) {
	t.Run("persists feedback at the 128-byte boundary", func(t *testing.T) {
		fixture := newRouteFixture(t)
		text := strings.Repeat("é", 64)
		response := fixture.request(http.MethodPut, "/analytics/feedback", `{"rating":1,"text":"`+text+`"}`, true)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusNoContent)
		}

		var rating int16
		var storedText string
		err := fixture.conn.QueryRow(context.Background(), `
			SELECT rating, text
			FROM application_feedback
			WHERE installation_id = $1
		`, fixture.userID).Scan(&rating, &storedText)
		if err != nil {
			t.Fatalf("read stored feedback: %v", err)
		}
		if rating != 1 || storedText != text {
			t.Fatalf("stored feedback = (%d, %q); want (1, %q)", rating, storedText, text)
		}
	})

	t.Run("trims and persists a plain email", func(t *testing.T) {
		fixture := newRouteFixture(t)
		response := fixture.request(http.MethodPut, "/analytics/feedback/order", `{"email":"  reader@example.com  "}`, true)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusNoContent)
		}

		var email string
		if err := fixture.conn.QueryRow(context.Background(), "SELECT email FROM feedback_leads WHERE installation_id = $1", fixture.userID).Scan(&email); err != nil {
			t.Fatalf("read stored lead: %v", err)
		}
		if email != "reader@example.com" {
			t.Fatalf("stored email = %q; want %q", email, "reader@example.com")
		}
	})

	t.Run("rejects feedback over 128 bytes", func(t *testing.T) {
		fixture := newRouteFixture(t)
		response := fixture.request(http.MethodPut, "/analytics/feedback", `{"rating":1,"text":"`+strings.Repeat("é", 65)+`"}`, true)
		requireErrorResponse(t, response, http.StatusBadRequest, pkg.InvalidInput)
		fixture.requireRowCount(t, "application_feedback", 0)
	})

	t.Run("rejects display-name email forms", func(t *testing.T) {
		fixture := newRouteFixture(t)
		response := fixture.request(http.MethodPut, "/analytics/feedback/order", `{"email":"Reader <reader@example.com>"}`, true)
		requireErrorResponse(t, response, http.StatusBadRequest, pkg.InvalidInput)
		fixture.requireRowCount(t, "feedback_leads", 0)
	})

	t.Run("requires authentication", func(t *testing.T) {
		fixture := newRouteFixture(t)
		response := fixture.request(http.MethodPut, "/analytics/feedback", `{"rating":-1}`, false)
		requireErrorResponse(t, response, http.StatusUnauthorized, "unauthorized")
		fixture.requireRowCount(t, "application_feedback", 0)
	})
}

func newRouteFixture(t *testing.T) *routeFixture {
	t.Helper()
	conn := startPostgres(t)
	db := queries.New(conn)
	service := analytics.NewAnalyticsService(db)
	handler := analytics.NewAnalyticsHandler(service)

	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("logger", zap.NewNop())
			return next(c)
		}
	})
	handler.RegisterRoutes(e.Group(""), auth.Authenticate(db))

	userID, err := db.NewUser(context.Background())
	if err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	auth.Init(integrationJWTSecret, false)
	accessCookie := startSession(t, e, db, userID)

	return &routeFixture{echo: e, conn: conn, userID: userID, accessCookie: accessCookie}
}

func startPostgres(t *testing.T) *pgx.Conn {
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

func startSession(t *testing.T, e *echo.Echo, db *queries.Queries, userID int64) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/session", nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)
	c.Set("logger", zap.NewNop())
	if err := auth.StartSession(c, db, int(userID)); err != nil {
		t.Fatalf("start authenticated session: %v", err)
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "access_token" {
			return cookie
		}
	}
	t.Fatal("session did not return an access_token cookie")
	return nil
}

func (fixture *routeFixture) request(method, target, body string, authenticated bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if authenticated {
		request.AddCookie(fixture.accessCookie)
	}
	recorder := httptest.NewRecorder()
	fixture.echo.ServeHTTP(recorder, request)
	return recorder
}

func (fixture *routeFixture) requireRowCount(t *testing.T, table string, want int) {
	t.Helper()
	var query string
	switch table {
	case "application_feedback":
		query = "SELECT count(*) FROM application_feedback WHERE installation_id = $1"
	case "feedback_leads":
		query = "SELECT count(*) FROM feedback_leads WHERE installation_id = $1"
	default:
		t.Fatalf("unsupported table %q", table)
	}
	var count int
	if err := fixture.conn.QueryRow(context.Background(), query, fixture.userID).Scan(&count); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if count != want {
		t.Fatalf("%s rows = %d; want %d", table, count, want)
	}
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
