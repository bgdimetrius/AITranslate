package translate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type translatorStub struct {
	output  string
	usage   pkg.Usage
	err     error
	request *translationReq
}

func (s *translatorStub) Do(_ context.Context, _ *zap.Logger, writer io.Writer, request *translationReq) (pkg.Usage, error) {
	requestCopy := *request
	s.request = &requestCopy
	if s.output != "" {
		_, _ = io.WriteString(writer, s.output)
	}
	return s.usage, s.err
}

type translationDBStub struct {
	records []queries.RecordDailyUsageParams
}

func (db *translationDBStub) ValidateUserId(context.Context, int64) (bool, error) {
	return true, nil
}

func (db *translationDBStub) RecordDailyUsage(_ context.Context, params queries.RecordDailyUsageParams) error {
	db.records = append(db.records, params)
	return nil
}

func newTranslationContext(provider, source, target, text string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	values := url.Values{
		"translator":  {provider},
		"source_lang": {source},
		"target_lang": {target},
		"text":        {text},
	}
	request := httptest.NewRequest(http.MethodGet, "/translate?"+values.Encode(), nil)
	c := e.NewContext(request, recorder)
	c.Set("logger", zap.NewNop())
	c.Set("user_id", 42)
	return c, recorder
}

// TestTranslationHandlerTranslate covers provider delegation, usage accounting,
// partial output, and documented provider failure statuses.
func TestTranslationHandlerTranslate(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		providerErr error
		wantCode    int
		wantMessage string
	}{
		{name: "successful translation", output: "hola", wantCode: http.StatusOK},
		{name: "provider failure after partial output", output: "hol", providerErr: errors.New("stream interrupted"), wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW},
		{name: "provider timeout", providerErr: context.DeadlineExceeded, wantCode: http.StatusGatewayTimeout, wantMessage: pkg.RequestTimedOut},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			translator := &translatorStub{
				output: test.output,
				usage:  pkg.Usage{InputTokens: 4, OutputTokens: 2},
				err:    test.providerErr,
			}
			db := new(translationDBStub)
			handler := &trslhandler{
				gpt:               translator,
				deepl:             new(translatorStub),
				allowedTextLength: 100,
				db:                db,
				validator:         translationCatalog(t),
			}
			c, recorder := newTranslationContext("chatgpt", "English", "Spanish", "hé")

			err := handler.Translate(c)
			if test.providerErr != nil {
				requireTranslationHTTPError(t, err, test.wantCode, test.wantMessage)
			} else if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if recorder.Body.String() != test.output {
				t.Fatalf("response body = %q; want %q", recorder.Body.String(), test.output)
			}

			wantRequest := &translationReq{
				SourceLang:    "English",
				TargetLang:    "Spanish",
				UserId:        42,
				Text:          "hé",
				withSource:    true,
				allowedLength: 100,
			}
			if !reflect.DeepEqual(translator.request, wantRequest) {
				t.Fatalf("provider request = %#v; want %#v", translator.request, wantRequest)
			}
			if len(db.records) != 1 {
				t.Fatalf("usage records = %d; want 1", len(db.records))
			}
			wantUsage := queries.RecordDailyUsageParams{
				InstallationID:  42,
				InputCharacters: 2,
				InputTokens:     4,
				OutputTokens:    2,
			}
			if test.providerErr == nil {
				wantUsage.TranslationRequests = 1
			} else {
				wantUsage.FailedRequests = 1
			}
			if db.records[0] != wantUsage {
				t.Fatalf("recorded usage = %#v; want %#v", db.records[0], wantUsage)
			}
		})
	}
}

// TestTranslationHandlerValidatesInput verifies that invalid public request
// values never invoke a provider or record usage.
func TestTranslationHandlerValidatesInput(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		source   string
		target   string
		text     string
		userID   any
		wantCode int
	}{
		{name: "unsupported provider", provider: "unknown", source: "English", target: "Spanish", text: "hello", userID: 42, wantCode: http.StatusBadRequest},
		{name: "unsupported source", provider: "chatgpt", source: "German", target: "Spanish", text: "hello", userID: 42, wantCode: http.StatusBadRequest},
		{name: "missing target", provider: "chatgpt", source: "English", text: "hello", userID: 42, wantCode: http.StatusBadRequest},
		{name: "missing text", provider: "chatgpt", source: "English", target: "Spanish", userID: 42, wantCode: http.StatusBadRequest},
		{name: "text over byte limit", provider: "chatgpt", source: "English", target: "Spanish", text: "ééé", userID: 42, wantCode: http.StatusBadRequest},
		{name: "missing authenticated user", provider: "chatgpt", source: "English", target: "Spanish", text: "hello", wantCode: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gpt := new(translatorStub)
			deepl := new(translatorStub)
			db := new(translationDBStub)
			handler := &trslhandler{
				gpt:               gpt,
				deepl:             deepl,
				allowedTextLength: 5,
				db:                db,
				validator:         translationCatalog(t),
			}
			c, _ := newTranslationContext(test.provider, test.source, test.target, test.text)
			c.Set(pkg.UserId, test.userID)

			err := handler.Translate(c)
			wantMessage := pkg.InvalidInput
			if test.wantCode == http.StatusInternalServerError {
				wantMessage = pkg.SmthWW
			}
			requireTranslationHTTPError(t, err, test.wantCode, wantMessage)
			if gpt.request != nil || deepl.request != nil {
				t.Fatal("provider was invoked for invalid input")
			}
			if len(db.records) != 0 {
				t.Fatalf("usage records = %d; want 0", len(db.records))
			}
		})
	}
}

// TestTranslationHandlerSelectsDeepL verifies the documented provider switch
// uses normalized language values from the shared catalog.
func TestTranslationHandlerSelectsDeepL(t *testing.T) {
	gpt := new(translatorStub)
	deepl := &translatorStub{output: "hola"}
	db := new(translationDBStub)
	handler := &trslhandler{
		gpt:               gpt,
		deepl:             deepl,
		allowedTextLength: 100,
		db:                db,
		validator:         translationCatalog(t),
	}
	c, recorder := newTranslationContext(" DEEPL ", " en ", " es ", "hello")

	if err := handler.Translate(c); err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if gpt.request != nil {
		t.Fatal("ChatGPT provider was invoked for a DeepL request")
	}
	want := &translationReq{SourceLang: "EN", TargetLang: "ES", UserId: 42, Text: "hello", withSource: true, allowedLength: 100}
	if !reflect.DeepEqual(deepl.request, want) {
		t.Fatalf("DeepL request = %#v; want %#v", deepl.request, want)
	}
	if recorder.Body.String() != "hola" {
		t.Fatalf("response body = %q; want %q", recorder.Body.String(), "hola")
	}
}

func translationCatalog(t *testing.T) *validation.Catalog {
	t.Helper()
	catalog, err := validation.NewCatalog(map[validation.Provider]validation.Capabilities{
		validation.ProviderChatGPT: {Source: []string{"English"}, Target: []string{"Spanish"}, AutoDetect: true},
		validation.ProviderDeepL:   {Source: []string{"EN"}, Target: []string{"ES"}, AutoDetect: true},
	})
	if err != nil {
		t.Fatalf("create test catalog: %v", err)
	}
	return catalog
}

func requireTranslationHTTPError(t *testing.T, err error, code int, message string) {
	t.Helper()
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %v; want *echo.HTTPError", err)
	}
	if httpErr.Code != code {
		t.Fatalf("HTTP error code = %d; want %d", httpErr.Code, code)
	}
	if httpErr.Message != message {
		t.Fatalf("HTTP error message = %v; want %q", httpErr.Message, message)
	}
}
