//go:build integration

package languages_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/labstack/echo/v5"
)

// TestLanguageRoutes verifies that the real catalog and Echo route expose detached, normalized provider capabilities.
func TestLanguageRoutes(t *testing.T) {
	catalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {
			Source:     []string{" Spanish ", "English"},
			Target:     []string{"German", " French "},
			AutoDetect: true,
		},
		languages.ProviderDeepL: {
			Source: []string{"EN", "DE"},
			Target: []string{"ES", "FR"},
		},
	})
	if err != nil {
		t.Fatalf("create language catalog: %v", err)
	}
	e := echo.New()
	catalog.RegisterRoutes(e.Group(""))

	response := httptest.NewRecorder()
	e.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/languages", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusOK)
	}
	var got map[languages.Provider]languages.Capabilities
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode language response %q: %v", response.Body.Bytes(), err)
	}
	want := map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {
			Source:     []string{"English", "Spanish"},
			Target:     []string{"French", "German"},
			AutoDetect: true,
		},
		languages.ProviderDeepL: {
			Source: []string{"DE", "EN"},
			Target: []string{"ES", "FR"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("language response = %#v; want %#v", got, want)
	}
}
