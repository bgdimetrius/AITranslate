//go:build integration

package interpretate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bgdimetrius/AITranslate/internal/auth"
	"github.com/bgdimetrius/AITranslate/internal/interpretate"
	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
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

type storedUsage struct {
	translationRequests    int32
	interpretationRequests int32
	failedRequests         int32
	inputCharacters        int64
	inputTokens            int64
	outputTokens           int64
}

// TestInterpretationRoutes verifies real authentication, request validation,
// provider decoding, and usage persistence.
func TestInterpretationRoutes(t *testing.T) {
	t.Run("interprets authenticated query text", func(t *testing.T) {
		fixture := newRouteFixture(t)
		values := url.Values{
			"target_lang": {"spanish"},
			"text":        {"break the ice"},
		}

		response := fixture.request(t, http.MethodGet, "/interpretate?"+values.Encode(), "", true)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusOK)
		}
		var body struct {
			PartOfSpeech string `json:"part_of_speech"`
			Meanings     []struct {
				Translation string `json:"translation"`
			} `json:"meanings"`
		}
		decodeJSON(t, response.Body.Bytes(), &body)
		if body.PartOfSpeech != "verb" || len(body.Meanings) != 1 || body.Meanings[0].Translation != "romper el hielo" {
			t.Fatalf("response = %#v; want the structured interpretation returned by the provider", body)
		}

		wantUsage := storedUsage{interpretationRequests: 1, inputCharacters: 13, inputTokens: 5, outputTokens: 7}
		if got := fixture.usage(t); got != wantUsage {
			t.Fatalf("stored usage = %#v; want %#v", got, wantUsage)
		}
	})

	t.Run("analyzes JSON body text", func(t *testing.T) {
		fixture := newRouteFixture(t)
		response := fixture.request(t, http.MethodPost, "/interpretate/analyze", `{"text":"break the ice"}`, true)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusOK)
		}
		if response.Body.Len() != 0 {
			t.Fatalf("response body = %q; want empty body", response.Body.String())
		}

		wantUsage := storedUsage{interpretationRequests: 1, inputCharacters: 13, inputTokens: 2, outputTokens: 1}
		if got := fixture.usage(t); got != wantUsage {
			t.Fatalf("stored usage = %#v; want %#v", got, wantUsage)
		}
	})

	t.Run("rejects text over the word limit", func(t *testing.T) {
		fixture := newRouteFixture(t)
		values := url.Values{
			"target_lang": {"spanish"},
			"text":        {strings.Repeat("word ", 22)},
		}

		response := fixture.request(t, http.MethodGet, "/interpretate?"+values.Encode(), "", true)
		requireErrorResponse(t, response, http.StatusBadRequest, pkg.InvalidInput)
		fixture.requireNoUsage(t)
	})

	t.Run("requires authentication", func(t *testing.T) {
		fixture := newRouteFixture(t)
		values := url.Values{
			"target_lang": {"spanish"},
			"text":        {"break the ice"},
		}

		response := fixture.request(t, http.MethodGet, "/interpretate?"+values.Encode(), "", false)
		requireErrorResponse(t, response, http.StatusUnauthorized, "unauthorized")
		fixture.requireNoUsage(t)
	})
}

func newRouteFixture(t *testing.T) *routeFixture {
	t.Helper()
	conn := startPostgres(t)
	db := queries.New(conn)
	provider := newProviderServer(t)

	openAIClient := openai.NewClient(
		option.WithAPIKey("integration-test-key"),
		option.WithBaseURL(provider.URL),
		option.WithUnsafeAllowHTTP(),
		option.WithHTTPClient(provider.Client()),
		option.WithMaxRetries(0),
	)
	catalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {
			Source:     []string{"English", "Spanish"},
			Target:     []string{"English", "Spanish"},
			AutoDetect: true,
		},
	})
	if err != nil {
		t.Fatalf("create language catalog: %v", err)
	}

	service := interpretate.NewInterpretationService(
		db,
		shared.ChatModel("integration-test-model"),
		openAIClient,
		"interpret prompt",
		"analyze prompt",
	)
	handler := interpretate.NewInterpretationHandler(service, db, catalog)

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

func newProviderServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			http.NotFound(w, request)
			return
		}
		var body struct {
			Instructions string `json:"instructions"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch body.Instructions {
		case "interpret prompt":
			text := `{"part_of_speech":"verb","meanings":[{"context":"idiom","translation":"romper el hielo","example":"She broke the ice."}]}`
			writeOpenAIResponse(w, text, 5, 7)
		case "analyze prompt":
			writeOpenAIResponse(w, "TRUE", 2, 1)
		default:
			http.Error(w, "unexpected instructions", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeOpenAIResponse(w http.ResponseWriter, text string, inputTokens, outputTokens int64) {
	w.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	encodedText, _ := json.Marshal(text)
	_, _ = fmt.Fprintf(w, `{"id":"resp_test","object":"response","status":"completed","output":[{"id":"msg_test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":%s,"annotations":[]}]}],"usage":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}}`, encodedText, inputTokens, outputTokens, inputTokens+outputTokens)
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

func (fixture *routeFixture) request(t *testing.T, method, target, body string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if authenticated {
		request.AddCookie(fixture.accessCookie)
	}
	recorder := httptest.NewRecorder()
	fixture.echo.ServeHTTP(recorder, request)
	return recorder
}

func (fixture *routeFixture) usage(t *testing.T) storedUsage {
	t.Helper()
	var usage storedUsage
	err := fixture.conn.QueryRow(context.Background(), `
		SELECT translation_requests, interpretation_requests, failed_requests,
		       input_characters, input_tokens, output_tokens
		FROM daily_usage
		WHERE installation_id = $1 AND usage_date = CURRENT_DATE
	`, fixture.userID).Scan(
		&usage.translationRequests,
		&usage.interpretationRequests,
		&usage.failedRequests,
		&usage.inputCharacters,
		&usage.inputTokens,
		&usage.outputTokens,
	)
	if err != nil {
		t.Fatalf("read stored usage: %v", err)
	}
	return usage
}

func (fixture *routeFixture) requireNoUsage(t *testing.T) {
	t.Helper()
	var count int
	if err := fixture.conn.QueryRow(context.Background(), "SELECT count(*) FROM daily_usage WHERE installation_id = $1", fixture.userID).Scan(&count); err != nil {
		t.Fatalf("count stored usage rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("stored usage rows = %d; want 0", count)
	}
}

func requireErrorResponse(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), status)
	}
	var body pkg.ErrorResponse
	decodeJSON(t, response.Body.Bytes(), &body)
	if body.Message != message {
		t.Fatalf("error message = %q; want %q", body.Message, message)
	}
}

func decodeJSON(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode JSON %q: %v", data, err)
	}
}
