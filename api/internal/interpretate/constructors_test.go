package interpretate

import (
	"testing"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// TestInterpretationConstructors verifies valid shared dependencies and prompts
// produce both service and HTTP adapter components.
func TestInterpretationConstructors(t *testing.T) {
	db := new(interpretationDBStub)
	client := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL("http://provider.test"),
		option.WithUnsafeAllowHTTP(),
	)
	service := NewInterpretationService(db, shared.ChatModel("test-model"), client, "interpret prompt", "analyze prompt")
	if service == nil {
		t.Fatal("NewInterpretationService() = nil")
	}
	catalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {Source: []string{"English"}, Target: []string{"Spanish"}, AutoDetect: true},
	})
	if err != nil {
		t.Fatalf("create language catalog: %v", err)
	}
	if handler := NewInterpretationHandler(service, db, catalog); handler == nil {
		t.Fatal("NewInterpretationHandler() = nil")
	}
}

// TestNewInterpretationHandlerPreconditions covers the documented required
// service, database, and language-catalog dependencies.
func TestNewInterpretationHandlerPreconditions(t *testing.T) {
	service := new(interpretationServiceStub)
	db := new(interpretationDBStub)
	catalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {Source: []string{"English"}, Target: []string{"Spanish"}, AutoDetect: true},
	})
	if err != nil {
		t.Fatalf("create language catalog: %v", err)
	}
	for _, test := range []struct {
		name string
		call func()
	}{
		{name: "nil service", call: func() { NewInterpretationHandler(nil, db, catalog) }},
		{name: "nil database", call: func() { NewInterpretationHandler(service, nil, catalog) }},
		{name: "nil catalog", call: func() { NewInterpretationHandler(service, db, nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !interpretationPanics(test.call) {
				t.Fatal("NewInterpretationHandler() did not panic")
			}
		})
	}
}

func interpretationPanics(fn func()) (didPanic bool) {
	defer func() { didPanic = recover() != nil }()
	fn()
	return false
}
