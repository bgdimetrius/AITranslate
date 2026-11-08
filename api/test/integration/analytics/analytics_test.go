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
	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

const integrationJWTSecret = "01234567890123456789012345678901"
const translationID = "8bdf29a7-54d2-48ee-bd2b-c0ae79af6831"

type routeFixture struct {
	echo         *echo.Echo
	conn         *pgx.Conn
	userID       int64
	accessCookie *http.Cookie
	telegram     *integrationTelegramSender
}

type integrationTelegramSender struct {
	messages []tgbotapi.Chattable
}

func (sender *integrationTelegramSender) Send(message tgbotapi.Chattable) (tgbotapi.Message, error) {
	sender.messages = append(sender.messages, message)
	return tgbotapi.Message{}, nil
}

// TestAnalyticsRoutes verifies authenticated feedback validation and
// persistence through real service and PostgreSQL components.
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

	t.Run("persists a normalized lead once and notifies only for the insertion", func(t *testing.T) {
		fixture := newRouteFixture(t)
		for _, body := range []string{`{"email":"  reader@example.com  "}`, `{"email":"reader@example.com"}`} {
			response := fixture.request(http.MethodPut, "/analytics/feedback/order", body, true)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusNoContent)
			}
		}

		fixture.requireRowCount(t, "feedback_leads", 1)
		var email string
		if err := fixture.conn.QueryRow(context.Background(), "SELECT email FROM feedback_leads WHERE installation_id = $1", fixture.userID).Scan(&email); err != nil {
			t.Fatalf("read stored lead: %v", err)
		}
		if email != "reader@example.com" {
			t.Fatalf("stored email = %q; want %q", email, "reader@example.com")
		}
		if len(fixture.telegram.messages) != 1 {
			t.Fatalf("Telegram messages = %d; want 1", len(fixture.telegram.messages))
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

	t.Run("persists normalized translation feedback", func(t *testing.T) {
		fixture := newRouteFixture(t)
		body := `{"translation_id":"` + translationID + `","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"like"}`
		response := fixture.request(http.MethodPut, "/analytics/feedback/translation", body, true)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusNoContent)
		}

		var storedID, provider, sourceLang, targetLang, sourceText, translationText, assessment string
		var revision int32
		err := fixture.conn.QueryRow(context.Background(), `
			SELECT translation_id, revision, provider, source_lang, target_lang, source_text, translation_text, assessment
			FROM translation_feedback
			WHERE installation_id = $1
		`, fixture.userID).Scan(&storedID, &revision, &provider, &sourceLang, &targetLang, &sourceText, &translationText, &assessment)
		if err != nil {
			t.Fatalf("read stored translation feedback: %v", err)
		}
		if storedID != translationID || revision != 1 || provider != "chatgpt" || sourceLang != "English" || targetLang != "Spanish" || sourceText != "hello" || translationText != "hola" || assessment != "like" {
			t.Fatalf("stored translation feedback = (%q, %q, %q, %q, %q, %q); want normalized values", provider, sourceLang, targetLang, sourceText, translationText, assessment)
		}
	})

	t.Run("keeps one row and ignores stale feedback revisions", func(t *testing.T) {
		fixture := newRouteFixture(t)
		requests := []string{
			`{"translation_id":"` + translationID + `","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"like"}`,
			`{"translation_id":"` + translationID + `","revision":2,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"dislike"}`,
			`{"translation_id":"` + translationID + `","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"like"}`,
		}
		for _, body := range requests {
			response := fixture.request(http.MethodPut, "/analytics/feedback/translation", body, true)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusNoContent)
			}
		}

		fixture.requireRowCount(t, "translation_feedback", 1)
		var revision int32
		var assessment string
		if err := fixture.conn.QueryRow(context.Background(), `
			SELECT revision, assessment
			FROM translation_feedback
			WHERE installation_id = $1 AND translation_id = $2
		`, fixture.userID, translationID).Scan(&revision, &assessment); err != nil {
			t.Fatalf("read upserted translation feedback: %v", err)
		}
		if revision != 2 || assessment != "dislike" {
			t.Fatalf("stored revision and assessment = (%d, %q); want (2, %q)", revision, assessment, "dislike")
		}
	})

	t.Run("rejects invalid translation feedback", func(t *testing.T) {
		fixture := newRouteFixture(t)
		body := `{"translation_id":"` + translationID + `","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"neutral"}`
		response := fixture.request(http.MethodPut, "/analytics/feedback/translation", body, true)
		requireErrorResponse(t, response, http.StatusBadRequest, pkg.InvalidInput)
	})

	t.Run("requires authentication for translation feedback", func(t *testing.T) {
		fixture := newRouteFixture(t)
		body := `{"translation_id":"` + translationID + `","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"like"}`
		response := fixture.request(http.MethodPut, "/analytics/feedback/translation", body, false)
		requireErrorResponse(t, response, http.StatusUnauthorized, "unauthorized")
	})
}

func newRouteFixture(t *testing.T) *routeFixture {
	t.Helper()
	conn := startPostgres(t)
	db := queries.New(conn)
	telegram := &integrationTelegramSender{}
	service := analytics.NewAnalyticsService(db, -1001234567890, telegram)
	catalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {
			Source:     []string{"English"},
			Target:     []string{"Spanish"},
			AutoDetect: true,
		},
	})
	if err != nil {
		t.Fatalf("create language catalog: %v", err)
	}
	handler := analytics.NewAnalyticsHandlerWithValidation(service, catalog, 5)

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

	return &routeFixture{echo: e, conn: conn, userID: userID, accessCookie: accessCookie, telegram: telegram}
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
			filepath.Join(migrations, "000003_add_translation_feedback.up.sql"),
			filepath.Join(migrations, "000004_make_feedback_leads_unique.up.sql"),
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
	case "translation_feedback":
		query = "SELECT count(*) FROM translation_feedback WHERE installation_id = $1"
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
