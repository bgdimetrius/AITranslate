package validation

import (
	"errors"
	"reflect"
	"testing"
)

// TestNewCatalog covers capability validation, normalized provider spelling,
// deterministic snapshots, and ownership of caller data.
func TestNewCatalog(t *testing.T) {
	input := map[Provider]Capabilities{
		" ChatGPT ": {
			Source:     []string{" spanish ", "English"},
			Target:     []string{"GERMAN", " French "},
			AutoDetect: true,
		},
	}
	catalog, err := NewCatalog(input)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	input[" ChatGPT "].Source[0] = "changed"
	delete(input, " ChatGPT ")
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
	if got := catalog.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Snapshot() after caller mutation = %#v; want %#v", got, want)
	}

	invalid := []struct {
		name         string
		capabilities map[Provider]Capabilities
	}{
		{name: "no providers"},
		{name: "empty provider", capabilities: map[Provider]Capabilities{" ": {Source: []string{"en"}, Target: []string{"de"}}}},
		{name: "normalized duplicate provider", capabilities: map[Provider]Capabilities{"chatgpt": {Source: []string{"en"}, Target: []string{"de"}}, " CHATGPT ": {Source: []string{"es"}, Target: []string{"fr"}}}},
		{name: "no source languages", capabilities: map[Provider]Capabilities{ProviderChatGPT: {Target: []string{"de"}}}},
		{name: "no target languages", capabilities: map[Provider]Capabilities{ProviderChatGPT: {Source: []string{"en"}}}},
		{name: "empty language", capabilities: map[Provider]Capabilities{ProviderChatGPT: {Source: []string{"en", " "}, Target: []string{"de"}}}},
		{name: "duplicate language", capabilities: map[Provider]Capabilities{ProviderChatGPT: {Source: []string{"en", "EN"}, Target: []string{"de"}}}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCatalog(test.capabilities); err == nil {
				t.Fatal("NewCatalog() error = nil; want validation error")
			}
		})
	}
}

// TestCatalogResolution verifies case-insensitive lookup, configured spelling,
// auto-detection support, and safe nil-catalog behavior.
func TestCatalogResolution(t *testing.T) {
	catalog := testCatalog(t)
	if provider, ok := catalog.ResolveProvider(" CHATGPT "); !ok || provider != ProviderChatGPT {
		t.Fatalf("ResolveProvider() = (%q, %t); want (%q, true)", provider, ok, ProviderChatGPT)
	}
	if source, ok := catalog.ResolveSource(ProviderChatGPT, " english "); !ok || source != "English" {
		t.Fatalf("ResolveSource() = (%q, %t); want (%q, true)", source, ok, "English")
	}
	if target, ok := catalog.ResolveTarget(ProviderDeepL, "es"); !ok || target != "ES" {
		t.Fatalf("ResolveTarget() = (%q, %t); want (%q, true)", target, ok, "ES")
	}
	if !catalog.SupportsAutoDetect(ProviderChatGPT) || catalog.SupportsAutoDetect(ProviderDeepL) {
		t.Fatal("SupportsAutoDetect() did not preserve configured provider capability")
	}
	var nilCatalog *Catalog
	if _, ok := nilCatalog.ResolveProvider("chatgpt"); ok || nilCatalog.SupportsAutoDetect(ProviderChatGPT) {
		t.Fatal("nil catalog reported configured capabilities")
	}
	if got := nilCatalog.Snapshot(); len(got) != 0 {
		t.Fatalf("nil Snapshot() length = %d; want 0", len(got))
	}
}

// TestCatalogValidateSelection covers every documented provider, source, and
// target outcome while preserving configured spelling.
func TestCatalogValidateSelection(t *testing.T) {
	catalog := testCatalog(t)
	tests := []struct {
		name     string
		provider string
		source   string
		target   string
		want     LanguageSelection
		wantErr  error
	}{
		{name: "normalized explicit selection", provider: " CHATGPT ", source: "english", target: "spanish", want: LanguageSelection{Provider: ProviderChatGPT, Source: "English", Target: "Spanish"}},
		{name: "automatic source detection", provider: "chatgpt", target: "Spanish", want: LanguageSelection{Provider: ProviderChatGPT, Target: "Spanish"}},
		{name: "unsupported provider", provider: "unknown", source: "English", target: "Spanish", wantErr: ErrUnsupportedProvider},
		{name: "source required", provider: "deepl", target: "ES", wantErr: ErrSourceRequired},
		{name: "unsupported source", provider: "chatgpt", source: "German", target: "Spanish", wantErr: ErrUnsupportedSource},
		{name: "target required", provider: "chatgpt", source: "English", wantErr: ErrTargetRequired},
		{name: "unsupported target", provider: "chatgpt", source: "English", target: "German", wantErr: ErrUnsupportedTarget},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := catalog.ValidateSelection(test.provider, test.source, test.target)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ValidateSelection() error = %v; want %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("ValidateSelection() = %#v; want %#v", got, test.want)
			}
		})
	}
}

// TestCatalogValidateFixedProvider covers fixed-provider source and target
// validation used by feature handlers.
func TestCatalogValidateFixedProvider(t *testing.T) {
	catalog := testCatalog(t)
	if got, err := catalog.ValidateSource(ProviderChatGPT, ""); err != nil || got != "" {
		t.Fatalf("ValidateSource(auto) = (%q, %v); want empty, nil", got, err)
	}
	if _, err := catalog.ValidateSource(ProviderDeepL, ""); !errors.Is(err, ErrSourceRequired) {
		t.Fatalf("ValidateSource(required) error = %v; want %v", err, ErrSourceRequired)
	}
	if got, err := catalog.ValidateTarget(ProviderChatGPT, " spanish "); err != nil || got != "Spanish" {
		t.Fatalf("ValidateTarget() = (%q, %v); want (%q, nil)", got, err, "Spanish")
	}
	if _, err := catalog.ValidateTarget("unknown", "Spanish"); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("ValidateTarget(unknown) error = %v; want %v", err, ErrUnsupportedProvider)
	}
	var nilCatalog *Catalog
	if _, err := nilCatalog.ValidateSource(ProviderChatGPT, "English"); err == nil {
		t.Fatal("nil ValidateSource() error = nil; want validation error")
	}
}

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(map[Provider]Capabilities{
		ProviderChatGPT: {Source: []string{"English"}, Target: []string{"Spanish"}, AutoDetect: true},
		ProviderDeepL:   {Source: []string{"EN"}, Target: []string{"ES"}},
	})
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	return catalog
}
