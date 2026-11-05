package translate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
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

func newTranslationContext(translator Translator) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/translate", nil)
	c := e.NewContext(request, recorder)
	c.Set("logger", zap.NewNop())
	c.Set("user_id", 42)
	c.Set("source_lang", "English")
	c.Set("target_lang", "Spanish")
	c.Set("text", "hé")
	c.Set("allowed_length", 100)
	c.Set("translator", translator)
	return c, recorder
}

// TestTranslationHandlerTranslate covers provider delegation, usage accounting, and documented provider failure statuses.
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
			handler := &trslhandler{db: db}
			c, recorder := newTranslationContext(translator)

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
