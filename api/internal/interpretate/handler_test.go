package interpretate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type interpretationServiceStub struct {
	interpretation interpretation
	suitable       bool
	usage          pkg.Usage
	err            error
	request        *interpReq
	text           string
}

func (s *interpretationServiceStub) Interpretate(_ context.Context, _ *zap.Logger, request *interpReq) (interpretation, pkg.Usage, error) {
	requestCopy := *request
	s.request = &requestCopy
	return s.interpretation, s.usage, s.err
}

func (s *interpretationServiceStub) AnalyzeText(_ context.Context, _ *zap.Logger, text string) (bool, pkg.Usage, error) {
	s.text = text
	return s.suitable, s.usage, s.err
}

type interpretationDBStub struct {
	records []queries.RecordDailyUsageParams
}

func (db *interpretationDBStub) ValidateUserId(context.Context, int64) (bool, error) {
	return true, nil
}

func (db *interpretationDBStub) RecordDailyUsage(_ context.Context, params queries.RecordDailyUsageParams) error {
	db.records = append(db.records, params)
	return nil
}

func newInterpretationContext(method, target, text, targetLang string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)
	c := e.NewContext(request, recorder)
	c.Set("logger", zap.NewNop())
	c.Set("user_id", 42)
	c.Set("text", text)
	if targetLang != "" {
		c.Set("target_lang", targetLang)
	}
	return c, recorder
}

func requireHTTPError(t *testing.T, err error, code int, message string) {
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

// TestInterpretationHandlerInterpretate covers successful JSON responses and documented provider failure statuses.
func TestInterpretationHandlerInterpretate(t *testing.T) {
	tests := []struct {
		name        string
		serviceErr  error
		wantCode    int
		wantMessage string
	}{
		{name: "successful interpretation", wantCode: http.StatusOK},
		{name: "provider failure", serviceErr: errors.New("provider unavailable"), wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW},
		{name: "provider timeout", serviceErr: context.DeadlineExceeded, wantCode: http.StatusGatewayTimeout, wantMessage: pkg.RequestTimedOut},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantInterpretation := interpretation{
				PartOfSpeech: "verb",
				MeaningCards: []*meaningCard{{Context: "greeting", Translation: "hola", Example: "hello there"}},
			}
			service := &interpretationServiceStub{
				interpretation: wantInterpretation,
				usage:          pkg.Usage{InputTokens: 7, OutputTokens: 11},
				err:            test.serviceErr,
			}
			db := new(interpretationDBStub)
			handler := &interphandler{service: service, db: db}
			c, recorder := newInterpretationContext(http.MethodGet, "/interpretate", "héllo", "Spanish")

			err := handler.Interpretate(c)
			if test.serviceErr != nil {
				requireHTTPError(t, err, test.wantCode, test.wantMessage)
			} else {
				if err != nil {
					t.Fatalf("Interpretate() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
				var got interpretation
				if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if !reflect.DeepEqual(got, wantInterpretation) {
					t.Fatalf("response = %#v; want %#v", got, wantInterpretation)
				}
			}

			wantRequest := &interpReq{TargetLang: "Spanish", UserId: 42, Text: "héllo"}
			if !reflect.DeepEqual(service.request, wantRequest) {
				t.Fatalf("service request = %#v; want %#v", service.request, wantRequest)
			}
			if len(db.records) != 1 {
				t.Fatalf("usage records = %d; want 1", len(db.records))
			}
			wantUsage := queries.RecordDailyUsageParams{
				InstallationID:  42,
				InputCharacters: 5,
				InputTokens:     7,
				OutputTokens:    11,
			}
			if test.serviceErr == nil {
				wantUsage.InterpretationRequests = 1
			} else {
				wantUsage.FailedRequests = 1
			}
			if db.records[0] != wantUsage {
				t.Fatalf("recorded usage = %#v; want %#v", db.records[0], wantUsage)
			}
		})
	}
}

// TestInterpretationHandlerAnalyzeText covers suitable, unsuitable, failed, and timed-out analysis outcomes.
func TestInterpretationHandlerAnalyzeText(t *testing.T) {
	tests := []struct {
		name        string
		suitable    bool
		serviceErr  error
		wantCode    int
		wantMessage string
	}{
		{name: "suitable text", suitable: true, wantCode: http.StatusOK},
		{name: "unsuitable text", wantCode: http.StatusNotAcceptable},
		{name: "provider failure", serviceErr: errors.New("provider unavailable"), wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW},
		{name: "provider timeout", serviceErr: context.DeadlineExceeded, wantCode: http.StatusGatewayTimeout, wantMessage: pkg.RequestTimedOut},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &interpretationServiceStub{
				suitable: test.suitable,
				usage:    pkg.Usage{InputTokens: 3, OutputTokens: 1},
				err:      test.serviceErr,
			}
			db := new(interpretationDBStub)
			handler := &interphandler{service: service, db: db}
			c, recorder := newInterpretationContext(http.MethodPost, "/interpretate/analyze", "break the ice", "")

			err := handler.AnalyzeText(c)
			if test.serviceErr != nil {
				requireHTTPError(t, err, test.wantCode, test.wantMessage)
			} else {
				if err != nil {
					t.Fatalf("AnalyzeText() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
			}
			if service.text != "break the ice" {
				t.Fatalf("analyzed text = %q; want %q", service.text, "break the ice")
			}
			if len(db.records) != 1 {
				t.Fatalf("usage records = %d; want 1", len(db.records))
			}
			wantUsage := queries.RecordDailyUsageParams{
				InstallationID:  42,
				InputCharacters: 13,
				InputTokens:     3,
				OutputTokens:    1,
			}
			if test.serviceErr == nil {
				wantUsage.InterpretationRequests = 1
			} else {
				wantUsage.FailedRequests = 1
			}
			if db.records[0] != wantUsage {
				t.Fatalf("recorded usage = %#v; want %#v", db.records[0], wantUsage)
			}
		})
	}
}
