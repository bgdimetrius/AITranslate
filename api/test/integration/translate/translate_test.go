//go:build integration

package translate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/bgdimetrius/AITranslate/internal/auth"
	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/internal/translate"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/cluttrdev/deepl-go/deepl"
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

// TestTranslationRoutes verifies both real provider adapters through the authenticated HTTP route and database accounting.
func TestTranslationRoutes(t *testing.T) {
	t.Run("streams a ChatGPT translation with source auto-detection", func(t *testing.T) {
		fixture := newRouteFixture(t, 100)
		values := url.Values{
			"target_lang": {"spanish"},
			"translator":  {"chatgpt"},
			"text":        {"héllo"},
		}

		response := fixture.request(t, "/translate?"+values.Encode())
		if response.Code != http.StatusOK || response.Body.String() != "hola" {
			t.Fatalf("status/body = %d/%q; want %d/%q", response.Code, response.Body.String(), http.StatusOK, "hola")
		}

		wantUsage := storedUsage{translationRequests: 1, inputCharacters: 5, inputTokens: 4, outputTokens: 2}
		if got := fixture.usage(t); got != wantUsage {
			t.Fatalf("stored usage = %#v; want %#v", got, wantUsage)
		}
	})

	t.Run("returns a buffered DeepL translation", func(t *testing.T) {
		fixture := newRouteFixture(t, 100)
		values := url.Values{
			"source_lang": {"en"},
			"target_lang": {"es"},
			"translator":  {"deepl"},
			"text":        {"hello"},
		}

		response := fixture.request(t, "/translate?"+values.Encode())
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusOK)
		}
		var body struct {
			SourceLang string `json:"source_lang"`
			Text       string `json:"text"`
		}
		decodeJSON(t, response.Body.Bytes(), &body)
		if body.SourceLang != "EN" || body.Text != "hola" {
			t.Fatalf("response = %#v; want source_lang EN and text hola", body)
		}

		wantUsage := storedUsage{translationRequests: 1, inputCharacters: 5}
		if got := fixture.usage(t); got != wantUsage {
			t.Fatalf("stored usage = %#v; want %#v", got, wantUsage)
		}
	})

	t.Run("rejects an unsupported provider", func(t *testing.T) {
		fixture := newRouteFixture(t, 100)
		values := url.Values{
			"target_lang": {"spanish"},
			"translator":  {"unknown"},
			"text":        {"hello"},
		}

		response := fixture.request(t, "/translate?"+values.Encode())
		requireErrorResponse(t, response, http.StatusBadRequest, pkg.InvalidInput)
		fixture.requireNoUsage(t)
	})

	t.Run("rejects text over the configured byte limit", func(t *testing.T) {
		fixture := newRouteFixture(t, 4)
		values := url.Values{
			"target_lang": {"spanish"},
			"translator":  {"chatgpt"},
			"text":        {"ééx"},
		}

		response := fixture.request(t, "/translate?"+values.Encode())
		requireErrorResponse(t, response, http.StatusBadRequest, pkg.InvalidInput)
		fixture.requireNoUsage(t)
	})
}

func newRouteFixture(t *testing.T, allowedTextLength int) *routeFixture {
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
	deepLClient, err := deepl.NewTranslator(
		"integration-test-key:fx",
		deepl.WithServerURL(provider.URL),
		deepl.WithHTTPClient(provider.Client()),
	)
	if err != nil {
		t.Fatalf("create DeepL client: %v", err)
	}
	catalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {
			Source:     []string{"English", "Spanish"},
			Target:     []string{"English", "Spanish"},
			AutoDetect: true,
		},
		languages.ProviderDeepL: {
			Source:     []string{"EN", "ES"},
			Target:     []string{"EN", "ES"},
			AutoDetect: true,
		},
	})
	if err != nil {
		t.Fatalf("create language catalog: %v", err)
	}

	gptTranslator := translate.NewGptTranslator(openAIClient, shared.ChatModel("integration-test-model"), "translate prompt")
	deepLTranslator := translate.NewDeepLTranslator([]*deepl.Translator{deepLClient})
	handler := translate.NewTranslationHandler(gptTranslator, deepLTranslator, allowedTextLength, db, catalog)

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
		switch request.URL.Path {
		case "/responses":
			var body struct {
				Instructions string `json:"instructions"`
				Stream       bool   `json:"stream"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if body.Instructions != "translate prompt" || !body.Stream {
				http.Error(w, "streaming translation request required", http.StatusBadRequest)
				return
			}
			writeOpenAIStream(w, "hola", 4, 2)
		case "/v2/translate":
			w.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			_, _ = io.WriteString(w, `{"translations":[{"detected_source_language":"EN","text":"hola"}]}`)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeOpenAIStream(w http.ResponseWriter, text string, inputTokens, outputTokens int64) {
	w.Header().Set(echo.HeaderContentType, "text/event-stream")
	encodedText, _ := json.Marshal(text)
	_, _ = fmt.Fprintf(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":%s,\"item_id\":\"msg_test\",\"output_index\":0,\"content_index\":0,\"sequence_number\":1}\n\n", encodedText)
	_, _ = fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_test\",\"object\":\"response\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":%d,\"output_tokens\":%d,\"total_tokens\":%d}}}\n\n", inputTokens, outputTokens, inputTokens+outputTokens)
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

func (fixture *routeFixture) request(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.AddCookie(fixture.accessCookie)
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
