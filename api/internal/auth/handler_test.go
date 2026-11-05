package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const unitJWTSecret = "01234567890123456789012345678901"

type authDBStub struct {
	valid         bool
	validateErr   error
	accessTokenID string
	getAccessErr  error
	saveErr       error
	rotateErr     error
	rotateRows    int64
	saved         []queries.SaveSessionParams
	rotated       []queries.RotateSessionParams
}

func (db *authDBStub) ValidateUserId(context.Context, int64) (bool, error) {
	return db.valid, db.validateErr
}

func (db *authDBStub) GetAccessTokenID(context.Context, int64) (string, error) {
	return db.accessTokenID, db.getAccessErr
}

func (db *authDBStub) SaveSession(_ context.Context, params queries.SaveSessionParams) error {
	db.saved = append(db.saved, params)
	return db.saveErr
}

func (db *authDBStub) RotateSession(_ context.Context, params queries.RotateSessionParams) (int64, error) {
	db.rotated = append(db.rotated, params)
	return db.rotateRows, db.rotateErr
}

// TestStartSession covers installation validation, complete active-session persistence, cookie issuance, and pre-cookie failures.
func TestStartSession(t *testing.T) {
	tests := []struct {
		name        string
		valid       bool
		validateErr error
		saveErr     error
		wantErr     bool
	}{
		{name: "issues session after persistence", valid: true},
		{name: "validation query failure", validateErr: errors.New("database unavailable"), wantErr: true},
		{name: "unknown installation", wantErr: true},
		{name: "refresh persistence failure", valid: true, saveErr: errors.New("database unavailable"), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			Init(unitJWTSecret, false)
			db := &authDBStub{valid: test.valid, validateErr: test.validateErr, saveErr: test.saveErr}
			c, recorder := newAuthContext(http.MethodPost, "/session")

			err := StartSession(c, db, 42)
			if test.wantErr {
				if err == nil {
					t.Fatal("StartSession() error = nil; want error")
				}
				if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
					t.Fatalf("cookies = %#v; want none", cookies)
				}
				return
			}
			if err != nil {
				t.Fatalf("StartSession() error = %v", err)
			}
			if len(db.saved) != 1 {
				t.Fatalf("saved sessions = %d; want 1", len(db.saved))
			}
			if db.saved[0].InstallationID != 42 {
				t.Fatalf("saved installation ID = %d; want 42", db.saved[0].InstallationID)
			}

			cookies := cookiesByName(recorder.Result().Cookies())
			accessCookie := cookies[accessTokenCookie]
			refreshCookie := cookies[refreshTokenCookie]
			if accessCookie == nil || refreshCookie == nil {
				t.Fatalf("cookies = %#v; want access and refresh cookies", cookies)
			}
			if db.saved[0].RefreshToken != hashRefreshToken(refreshCookie.Value) {
				t.Fatal("persisted refresh token is not the hash of the returned token")
			}
			if db.saved[0].RefreshToken == refreshCookie.Value {
				t.Fatal("persisted refresh token contains the raw returned token")
			}
			accessClaims, err := decodeAccessToken(zap.NewNop(), accessCookie.Value, unitJWTSecret, true)
			if err != nil {
				t.Fatalf("decode returned access token: %v", err)
			}
			if db.saved[0].AccessTokenID != accessClaims.ID {
				t.Fatalf("persisted access token ID = %q; want %q", db.saved[0].AccessTokenID, accessClaims.ID)
			}
			for name, cookie := range cookies {
				if !cookie.HttpOnly || cookie.Path != "/" || cookie.MaxAge != int(refreshTokenTTL.Seconds()) {
					t.Fatalf("%s cookie attributes = %#v; want HttpOnly root cookie with refresh lifetime", name, cookie)
				}
			}
		})
	}
}

// TestAuthenticate covers JWT validation and the active-session check that immediately rejects superseded access tokens.
func TestAuthenticate(t *testing.T) {
	Init(unitJWTSecret, false)
	validPair, err := generatePair(zap.NewNop(), 42, unitJWTSecret)
	if err != nil {
		t.Fatalf("generate valid access token: %v", err)
	}
	expiredAccess := signAccessToken(t, "42", unitJWTSecret, time.Now().Add(-time.Hour), "expired-token-id")
	invalidSubject := signAccessToken(t, "not-a-number", unitJWTSecret, time.Now().Add(time.Hour), "invalid-subject-id")
	missingID := signAccessToken(t, "42", unitJWTSecret, time.Now().Add(time.Hour), "")

	tests := []struct {
		name          string
		token         string
		activeTokenID string
		getAccessErr  error
		wantCode      int
		wantMessage   string
		wantNext      bool
	}{
		{name: "current access token", token: validPair.accessToken, activeTokenID: validPair.accessTokenID, wantNext: true},
		{name: "superseded access token", token: validPair.accessToken, activeTokenID: "different-token-id", wantCode: http.StatusUnauthorized, wantMessage: "session expired"},
		{name: "missing access token", wantCode: http.StatusUnauthorized, wantMessage: "unauthorized"},
		{name: "invalid access token", token: "not-a-token", wantCode: http.StatusUnauthorized, wantMessage: "session expired"},
		{name: "expired access token", token: expiredAccess, wantCode: http.StatusUnauthorized, wantMessage: "session expired"},
		{name: "non-numeric subject", token: invalidSubject, wantCode: http.StatusUnauthorized, wantMessage: "invalid session"},
		{name: "missing JWT ID", token: missingID, wantCode: http.StatusUnauthorized, wantMessage: "invalid session"},
		{name: "missing active session", token: validPair.accessToken, getAccessErr: pgx.ErrNoRows, wantCode: http.StatusUnauthorized, wantMessage: "session expired"},
		{name: "session lookup failure", token: validPair.accessToken, getAccessErr: errors.New("database unavailable"), wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := &authDBStub{accessTokenID: test.activeTokenID, getAccessErr: test.getAccessErr}
			c, _ := newAuthContext(http.MethodGet, "/protected")
			if test.token != "" {
				c.Request().AddCookie(&http.Cookie{Name: accessTokenCookie, Value: test.token})
			}
			nextCalled := false
			err := Authenticate(db)(func(c *echo.Context) error {
				nextCalled = true
				if userID := c.Get("user_id"); userID != 42 {
					t.Fatalf("context user_id = %#v; want 42", userID)
				}
				return nil
			})(c)

			if test.wantNext {
				if err != nil {
					t.Fatalf("Authenticate() error = %v", err)
				}
			} else {
				requireAuthHTTPError(t, err, test.wantCode, test.wantMessage)
			}
			if nextCalled != test.wantNext {
				t.Fatalf("next called = %t; want %t", nextCalled, test.wantNext)
			}
		})
	}
}

// TestAuthHandlerHandshake covers atomic credential rotation, refresh replay rejection, malformed credentials, generation failure, and persistence failure.
func TestAuthHandlerHandshake(t *testing.T) {
	validRefresh := "refresh-token"
	expiredAccess := signAccessToken(t, "42", unitJWTSecret, time.Now().Add(-time.Hour), "expired-token-id")
	emptySecretAccess := signAccessToken(t, "42", "", time.Now().Add(-time.Hour), "empty-secret-id")
	invalidSubject := signAccessToken(t, "not-a-number", unitJWTSecret, time.Now().Add(time.Hour), "invalid-subject-id")

	tests := []struct {
		name            string
		accessToken     string
		refreshToken    string
		rotateRows      int64
		rotateErr       error
		emptySecret     bool
		wantCode        int
		wantMessage     string
		wantRotateCount int
	}{
		{
			name:            "rotates an expired access session",
			accessToken:     expiredAccess,
			refreshToken:    validRefresh,
			rotateRows:      1,
			wantCode:        http.StatusCreated,
			wantRotateCount: 1,
		},
		{name: "missing access cookie", refreshToken: validRefresh, wantCode: http.StatusBadRequest, wantMessage: "access token is missing"},
		{name: "missing refresh cookie", accessToken: expiredAccess, wantCode: http.StatusBadRequest, wantMessage: "refresh token is missing"},
		{name: "invalid access token", accessToken: "not-a-token", refreshToken: validRefresh, wantCode: http.StatusForbidden, wantMessage: "invalid access token"},
		{name: "non-numeric subject", accessToken: invalidSubject, refreshToken: validRefresh, wantCode: http.StatusForbidden, wantMessage: "invalid access token"},
		{name: "mismatched refresh token", accessToken: expiredAccess, refreshToken: "wrong-refresh", wantCode: http.StatusUnauthorized, wantMessage: "unauthorized", wantRotateCount: 1},
		{name: "token generation failure", accessToken: emptySecretAccess, refreshToken: validRefresh, emptySecret: true, wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW},
		{name: "rotation persistence failure", accessToken: expiredAccess, refreshToken: validRefresh, rotateErr: errors.New("database unavailable"), wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW, wantRotateCount: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			secret := unitJWTSecret
			if test.emptySecret {
				secret = ""
			}
			Init(secret, false)
			db := &authDBStub{
				valid:      true,
				rotateRows: test.rotateRows,
				rotateErr:  test.rotateErr,
			}
			handler := NewAuthHandler(db)
			c, recorder := newAuthContext(http.MethodPost, "/auth/handshake")
			if test.accessToken != "" {
				c.Request().AddCookie(&http.Cookie{Name: accessTokenCookie, Value: test.accessToken})
			}
			if test.refreshToken != "" {
				c.Request().AddCookie(&http.Cookie{Name: refreshTokenCookie, Value: test.refreshToken})
			}

			err := handler.Handshake(c)
			if test.wantCode == http.StatusCreated {
				if err != nil {
					t.Fatalf("Handshake() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
				cookies := cookiesByName(recorder.Result().Cookies())
				if cookies[accessTokenCookie] == nil || cookies[refreshTokenCookie] == nil {
					t.Fatalf("cookies = %#v; want rotated access and refresh cookies", cookies)
				}
				if cookies[refreshTokenCookie].Value == validRefresh {
					t.Fatal("refresh token was not rotated")
				}
				if db.rotated[0].NewRefreshToken != hashRefreshToken(cookies[refreshTokenCookie].Value) {
					t.Fatal("stored refresh hash does not match the rotated refresh token")
				}
				rotatedClaims, err := decodeAccessToken(zap.NewNop(), cookies[accessTokenCookie].Value, unitJWTSecret, true)
				if err != nil {
					t.Fatalf("decode rotated access token: %v", err)
				}
				if db.rotated[0].NewAccessTokenID != rotatedClaims.ID {
					t.Fatal("stored access token ID does not match the rotated access token")
				}
				if db.rotated[0].PreviousRefreshToken != hashRefreshToken(validRefresh) {
					t.Fatal("rotation did not conditionally match the presented refresh token")
				}
			} else {
				requireAuthHTTPError(t, err, test.wantCode, test.wantMessage)
				if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
					t.Fatalf("cookies = %#v; want none on failure", cookies)
				}
			}
			if len(db.rotated) != test.wantRotateCount {
				t.Fatalf("session rotations = %d; want %d", len(db.rotated), test.wantRotateCount)
			}
		})
	}
}

func newAuthContext(method, target string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)
	c := e.NewContext(request, recorder)
	c.Set("logger", zap.NewNop())
	return c, recorder
}

func signAccessToken(t *testing.T, subject, secret string, expiresAt time.Time, tokenID string) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    tokenIssuer,
		Audience:  jwt.ClaimStrings{tokenAudience},
		Subject:   subject,
		ID:        tokenID,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Hour)),
	}})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign access token for subject %s: %v", strconv.Quote(subject), err)
	}
	return signed
}

func cookiesByName(cookies []*http.Cookie) map[string]*http.Cookie {
	result := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}

func requireAuthHTTPError(t *testing.T, err error, code int, message string) {
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
