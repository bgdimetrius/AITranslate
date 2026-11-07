package users

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bgdimetrius/AITranslate/internal/auth"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const usersUnitJWTSecret = "01234567890123456789012345678901"

type usersDBStub struct {
	newUserID           int64
	newUserErr          error
	valid               bool
	validateErr         error
	saveErr             error
	created             bool
	sessionBeforeCreate bool
	newUserCalls        int
	validateCalls       int
	saveCalls           int
	saved               []queries.SaveSessionParams
}

func (db *usersDBStub) NewUser(context.Context) (int64, error) {
	db.newUserCalls++
	if db.newUserErr == nil {
		db.created = true
	}
	return db.newUserID, db.newUserErr
}

func (db *usersDBStub) ValidateUserId(context.Context, int64) (bool, error) {
	db.validateCalls++
	if !db.created {
		db.sessionBeforeCreate = true
	}
	return db.created && db.valid, db.validateErr
}

func (db *usersDBStub) SaveSession(_ context.Context, params queries.SaveSessionParams) error {
	db.saveCalls++
	if !db.created {
		db.sessionBeforeCreate = true
	}
	db.saved = append(db.saved, params)
	return db.saveErr
}

// TestUserHandlerNewUser covers installation persistence, initial session
// issuance, and documented creation or session failures.
func TestUserHandlerNewUser(t *testing.T) {
	tests := []struct {
		name           string
		newUserErr     error
		valid          bool
		validateErr    error
		saveErr        error
		wantCode       int
		wantValidation int
		wantSaveCount  int
	}{
		{
			name:           "creates installation before issuing initial session",
			valid:          true,
			wantCode:       http.StatusCreated,
			wantValidation: 1,
			wantSaveCount:  1,
		},
		{
			name:       "installation creation failure",
			newUserErr: errors.New("database unavailable"),
			wantCode:   http.StatusInternalServerError,
		},
		{
			name:           "session validation failure",
			valid:          true,
			validateErr:    errors.New("database unavailable"),
			wantCode:       http.StatusInternalServerError,
			wantValidation: 1,
		},
		{
			name:           "session persistence failure",
			valid:          true,
			saveErr:        errors.New("database unavailable"),
			wantCode:       http.StatusInternalServerError,
			wantValidation: 1,
			wantSaveCount:  1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			auth.Init(usersUnitJWTSecret, false)
			db := &usersDBStub{
				newUserID:   73,
				newUserErr:  test.newUserErr,
				valid:       test.valid,
				validateErr: test.validateErr,
				saveErr:     test.saveErr,
			}
			handler := NewUserHandler(db)
			c, recorder := newUsersContext()

			err := handler.NewUser(c)
			if test.wantCode == http.StatusCreated {
				if err != nil {
					t.Fatalf("NewUser() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
				cookies := usersCookiesByName(recorder.Result().Cookies())
				accessCookie := cookies["access_token"]
				refreshCookie := cookies["refresh_token"]
				if accessCookie == nil || refreshCookie == nil {
					t.Fatalf("cookies = %#v; want access and refresh cookies", cookies)
				}
				if !accessCookie.HttpOnly || !refreshCookie.HttpOnly {
					t.Fatal("initial session cookies are not HttpOnly")
				}
				if db.saved[0].InstallationID != 73 {
					t.Fatalf("saved installation ID = %d; want 73", db.saved[0].InstallationID)
				}
				if db.saved[0].RefreshToken != hashUsersToken(refreshCookie.Value) {
					t.Fatal("stored refresh hash does not match the returned refresh token")
				}
				if db.saved[0].AccessTokenID == "" {
					t.Fatal("stored session does not contain the active access token ID")
				}
			} else {
				requireUsersHTTPError(t, err, test.wantCode, pkg.SmthWW)
				if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
					t.Fatalf("cookies = %#v; want none on failure", cookies)
				}
			}
			if db.newUserCalls != 1 {
				t.Fatalf("installation creation calls = %d; want 1", db.newUserCalls)
			}
			if db.validateCalls != test.wantValidation {
				t.Fatalf("session validation calls = %d; want %d", db.validateCalls, test.wantValidation)
			}
			if db.sessionBeforeCreate {
				t.Fatal("initial session was attempted before installation persistence")
			}
			if db.saveCalls != test.wantSaveCount || len(db.saved) != test.wantSaveCount {
				t.Fatalf("saved sessions = (%d calls, %d values); want %d", db.saveCalls, len(db.saved), test.wantSaveCount)
			}
		})
	}
}

func newUsersContext() (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/users/new", nil), recorder)
	c.Set("logger", zap.NewNop())
	return c, recorder
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

func requireUsersHTTPError(t *testing.T, err error, code int, message string) {
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
