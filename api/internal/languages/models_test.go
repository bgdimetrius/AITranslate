package languages

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// TestNewCatalog covers adapter validation, normalized capabilities, detached
// snapshots, and ownership of caller input.
func TestNewCatalog(t *testing.T) {
	t.Run("copies and normalizes configured capabilities", func(t *testing.T) {
		input := map[Provider]Capabilities{
			ProviderChatGPT: {
				Source:     []string{" spanish ", "English"},
				Target:     []string{"GERMAN", " French "},
				AutoDetect: true,
			},
		}
		catalog, err := NewCatalog(input)
		if err != nil {
			t.Fatalf("NewCatalog() error = %v", err)
		}

		input[ProviderChatGPT].Source[0] = "changed"
		delete(input, ProviderChatGPT)
		want := map[Provider]Capabilities{
			ProviderChatGPT: {
				Source:     []string{"English", "spanish"},
				Target:     []string{"French", "GERMAN"},
				AutoDetect: true,
			},
		}
		if got := catalog.Snapshot(); !reflect.DeepEqual(got, want) {
			t.Fatalf("Snapshot() = %#v; want %#v", got, want)
		}

		snapshot := catalog.Snapshot()
		snapshot[ProviderChatGPT].Source[0] = "changed"
		delete(snapshot, ProviderChatGPT)
		if got := catalog.Snapshot(); !reflect.DeepEqual(got, want) {
			t.Fatalf("Snapshot() after caller mutation = %#v; want %#v", got, want)
		}
	})

	tests := []struct {
		name         string
		capabilities map[Provider]Capabilities
	}{
		{name: "no providers", capabilities: nil},
		{name: "empty provider", capabilities: map[Provider]Capabilities{" ": {Source: []string{"en"}, Target: []string{"de"}}}},
		{name: "no source languages", capabilities: map[Provider]Capabilities{ProviderChatGPT: {Target: []string{"de"}}}},
		{name: "no target languages", capabilities: map[Provider]Capabilities{ProviderChatGPT: {Source: []string{"en"}}}},
		{name: "empty language", capabilities: map[Provider]Capabilities{ProviderChatGPT: {Source: []string{"en", " "}, Target: []string{"de"}}}},
		{name: "case-insensitive duplicate", capabilities: map[Provider]Capabilities{ProviderChatGPT: {Source: []string{"en", "EN"}, Target: []string{"de"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCatalog(test.capabilities); err == nil {
				t.Fatal("NewCatalog() error = nil; want validation error")
			}
		})
	}
}

// TestCatalogResolution covers case-insensitive resolution, configured
// spelling, unknown values, and automatic source detection.
func TestCatalogResolution(t *testing.T) {
	catalog := mustCatalog(t, map[Provider]Capabilities{
		ProviderChatGPT: {Source: []string{"English"}, Target: []string{"French"}, AutoDetect: true},
		ProviderDeepL:   {Source: []string{"EN"}, Target: []string{"DE"}},
	})

	if got, ok := catalog.ResolveSource(ProviderChatGPT, " english "); !ok || got != "English" {
		t.Fatalf("ResolveSource() = (%q, %t); want (%q, true)", got, ok, "English")
	}
	if got, ok := catalog.ResolveTarget(ProviderDeepL, "de"); !ok || got != "DE" {
		t.Fatalf("ResolveTarget() = (%q, %t); want (%q, true)", got, ok, "DE")
	}
	if _, ok := catalog.ResolveSource(ProviderDeepL, "French"); ok {
		t.Fatal("ResolveSource() accepted an unsupported language")
	}
	if _, ok := catalog.ResolveTarget(Provider("unknown"), "French"); ok {
		t.Fatal("ResolveTarget() accepted an unknown provider")
	}
	if !catalog.SupportsAutoDetect(ProviderChatGPT) {
		t.Fatal("SupportsAutoDetect(chatgpt) = false; want true")
	}
	if catalog.SupportsAutoDetect(ProviderDeepL) || catalog.SupportsAutoDetect(Provider("unknown")) {
		t.Fatal("SupportsAutoDetect() reported support for an unconfigured capability")
	}
}

// TestValidateSourceLang covers provider selection, normalization, automatic
// detection, and invalid middleware input.
func TestValidateSourceLang(t *testing.T) {
	catalog := mustCatalog(t, map[Provider]Capabilities{
		ProviderChatGPT: {Source: []string{"English"}, Target: []string{"French"}, AutoDetect: true},
		ProviderDeepL:   {Source: []string{"EN"}, Target: []string{"DE"}},
	})

	tests := []struct {
		name         string
		target       string
		provider     Provider
		setProvider  bool
		wantCode     int
		wantLanguage any
		wantNext     bool
	}{
		{name: "normalizes configured source", target: "/?source_lang=english", provider: ProviderChatGPT, setProvider: true, wantLanguage: "English", wantNext: true},
		{name: "allows omitted source with auto-detection", target: "/", provider: ProviderChatGPT, setProvider: true, wantNext: true},
		{name: "rejects omitted source without auto-detection", target: "/", provider: ProviderDeepL, setProvider: true, wantCode: http.StatusBadRequest},
		{name: "rejects unsupported source", target: "/?source_lang=French", provider: ProviderChatGPT, setProvider: true, wantCode: http.StatusBadRequest},
		{name: "requires selected provider", target: "/?source_lang=English", wantCode: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := newLanguageContext(test.target)
			if test.setProvider {
				SetProvider(c, test.provider)
			}
			nextCalled := false
			err := catalog.ValidateSourceLang(func(c *echo.Context) error {
				nextCalled = true
				if got := c.Get("source_lang"); got != test.wantLanguage {
					t.Fatalf("source_lang = %#v; want %#v", got, test.wantLanguage)
				}
				return nil
			})(c)

			if test.wantNext {
				if err != nil {
					t.Fatalf("ValidateSourceLang() error = %v", err)
				}
			} else {
				requireLanguageHTTPError(t, err, test.wantCode)
			}
			if nextCalled != test.wantNext {
				t.Fatalf("next called = %t; want %t", nextCalled, test.wantNext)
			}
		})
	}
}

// TestValidateTargetLang covers provider selection, normalization, and invalid
// or omitted target input.
func TestValidateTargetLang(t *testing.T) {
	catalog := mustCatalog(t, map[Provider]Capabilities{
		ProviderChatGPT: {Source: []string{"English"}, Target: []string{"French"}, AutoDetect: true},
	})

	tests := []struct {
		name        string
		target      string
		setProvider bool
		wantCode    int
		wantNext    bool
	}{
		{name: "normalizes configured target", target: "/?target_lang=french", setProvider: true, wantNext: true},
		{name: "rejects omitted target", target: "/", setProvider: true, wantCode: http.StatusBadRequest},
		{name: "rejects unsupported target", target: "/?target_lang=German", setProvider: true, wantCode: http.StatusBadRequest},
		{name: "requires selected provider", target: "/?target_lang=French", wantCode: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := newLanguageContext(test.target)
			if test.setProvider {
				SetProvider(c, ProviderChatGPT)
			}
			nextCalled := false
			err := catalog.ValidateTargetLang(func(c *echo.Context) error {
				nextCalled = true
				if got := c.Get("target_lang"); got != "French" {
					t.Fatalf("target_lang = %#v; want %q", got, "French")
				}
				return nil
			})(c)

			if test.wantNext {
				if err != nil {
					t.Fatalf("ValidateTargetLang() error = %v", err)
				}
			} else {
				requireLanguageHTTPError(t, err, test.wantCode)
			}
			if nextCalled != test.wantNext {
				t.Fatalf("next called = %t; want %t", nextCalled, test.wantNext)
			}
		})
	}
}

func mustCatalog(t *testing.T, capabilities map[Provider]Capabilities) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(capabilities)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	return catalog
}

func newLanguageContext(target string) *echo.Context {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, target, nil), httptest.NewRecorder())
	c.Set("logger", zap.NewNop())
	return c
}

func requireLanguageHTTPError(t *testing.T, err error, code int) {
	t.Helper()
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %v; want *echo.HTTPError", err)
	}
	if httpErr.Code != code {
		t.Fatalf("HTTP error code = %d; want %d", httpErr.Code, code)
	}
	wantMessage := pkg.InvalidInput
	if code == http.StatusInternalServerError {
		wantMessage = pkg.SmthWW
	}
	if httpErr.Message != wantMessage {
		t.Fatalf("HTTP error message = %v; want %q", httpErr.Message, wantMessage)
	}
}
