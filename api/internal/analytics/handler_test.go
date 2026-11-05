package analytics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type analyticsServiceStub struct {
	feedbackErr error
	leadErr     error
	userID      int
	feedback    *rating
	leadEmail   string
}

func (service *analyticsServiceStub) SaveApplicationFeedback(_ context.Context, _ *zap.Logger, userID int, feedback rating) error {
	service.userID = userID
	feedbackCopy := feedback
	service.feedback = &feedbackCopy
	return service.feedbackErr
}

func (service *analyticsServiceStub) SaveFeedbackLead(_ context.Context, _ *zap.Logger, userID int, email string) error {
	service.userID = userID
	service.leadEmail = email
	return service.leadErr
}

func newAnalyticsContext(method, target, body string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(request, recorder)
	c.Set("logger", zap.NewNop())
	c.Set("user_id", 42)
	return c, recorder
}

// TestAnalyticsHandlerApplicationFeedback covers accepted ratings, the byte-length boundary, malformed input, and persistence failure.
func TestAnalyticsHandlerApplicationFeedback(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		serviceErr   error
		wantCode     int
		wantFeedback *rating
	}{
		{
			name:         "positive rating at 128-byte boundary",
			body:         `{"rating":1,"text":"` + strings.Repeat("é", 64) + `"}`,
			wantCode:     http.StatusNoContent,
			wantFeedback: &rating{Rating: 1, Text: strings.Repeat("é", 64)},
		},
		{
			name:         "negative rating",
			body:         `{"rating":-1}`,
			wantCode:     http.StatusNoContent,
			wantFeedback: &rating{Rating: -1},
		},
		{name: "malformed JSON", body: `{"rating":`, wantCode: http.StatusBadRequest},
		{name: "unsupported rating", body: `{"rating":0}`, wantCode: http.StatusBadRequest},
		{name: "feedback over 128 bytes", body: `{"rating":1,"text":"` + strings.Repeat("é", 65) + `"}`, wantCode: http.StatusBadRequest},
		{
			name:         "persistence failure",
			body:         `{"rating":1,"text":"useful"}`,
			serviceErr:   errors.New("database unavailable"),
			wantCode:     http.StatusInternalServerError,
			wantFeedback: &rating{Rating: 1, Text: "useful"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &analyticsServiceStub{feedbackErr: test.serviceErr}
			handler := NewAnalyticsHandler(service)
			c, recorder := newAnalyticsContext(http.MethodPut, "/analytics/feedback", test.body)

			err := handler.ApplicationFeedback(c)
			if test.wantCode == http.StatusNoContent {
				if err != nil {
					t.Fatalf("ApplicationFeedback() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
			} else {
				wantMessage := pkg.InvalidInput
				if test.wantCode == http.StatusInternalServerError {
					wantMessage = pkg.SmthWW
				}
				requireAnalyticsHTTPError(t, err, test.wantCode, wantMessage)
			}

			wantUserID := 0
			if test.wantFeedback != nil {
				wantUserID = 42
			}
			if service.userID != wantUserID {
				t.Fatalf("service user ID = %d; want %d", service.userID, wantUserID)
			}
			if !equalRating(service.feedback, test.wantFeedback) {
				t.Fatalf("service feedback = %#v; want %#v", service.feedback, test.wantFeedback)
			}
		})
	}
}

// TestAnalyticsHandlerRegisterFeedbackLead covers whitespace trimming, plain-address validation, malformed input, and persistence failure.
func TestAnalyticsHandlerRegisterFeedbackLead(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		serviceErr error
		wantCode   int
		wantEmail  string
	}{
		{name: "trimmed plain email", body: `{"email":"  reader@example.com  "}`, wantCode: http.StatusNoContent, wantEmail: "reader@example.com"},
		{name: "malformed JSON", body: `{"email":`, wantCode: http.StatusBadRequest},
		{name: "invalid email", body: `{"email":"not-an-email"}`, wantCode: http.StatusBadRequest},
		{name: "display-name address", body: `{"email":"Reader <reader@example.com>"}`, wantCode: http.StatusBadRequest},
		{
			name:       "persistence failure",
			body:       `{"email":"reader@example.com"}`,
			serviceErr: errors.New("database unavailable"),
			wantCode:   http.StatusInternalServerError,
			wantEmail:  "reader@example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &analyticsServiceStub{leadErr: test.serviceErr}
			handler := NewAnalyticsHandler(service)
			c, recorder := newAnalyticsContext(http.MethodPut, "/analytics/feedback/order", test.body)

			err := handler.RegisterFeedbackLead(c)
			if test.wantCode == http.StatusNoContent {
				if err != nil {
					t.Fatalf("RegisterFeedbackLead() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
			} else {
				wantMessage := pkg.InvalidInput
				if test.wantCode == http.StatusInternalServerError {
					wantMessage = pkg.SmthWW
				}
				requireAnalyticsHTTPError(t, err, test.wantCode, wantMessage)
			}

			wantUserID := 0
			if test.wantEmail != "" {
				wantUserID = 42
			}
			if service.userID != wantUserID {
				t.Fatalf("service user ID = %d; want %d", service.userID, wantUserID)
			}
			if service.leadEmail != test.wantEmail {
				t.Fatalf("service email = %q; want %q", service.leadEmail, test.wantEmail)
			}
		})
	}
}

func equalRating(got, want *rating) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}

func requireAnalyticsHTTPError(t *testing.T, err error, code int, message string) {
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
