package translate

import (
	"testing"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/cluttrdev/deepl-go/deepl"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// TestTranslationConstructors verifies valid provider and handler dependencies
// produce usable translation components.
func TestTranslationConstructors(t *testing.T) {
	openAIClient := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL("http://provider.test"),
		option.WithUnsafeAllowHTTP(),
	)
	gpt := NewGptTranslator(openAIClient, shared.ChatModel("test-model"), "translate prompt")
	if gpt == nil {
		t.Fatal("NewGptTranslator() = nil")
	}
	deepLClient, err := deepl.NewTranslator("test-key:fx")
	if err != nil {
		t.Fatalf("create DeepL client: %v", err)
	}
	deepLTranslator := NewDeepLTranslator([]*deepl.Translator{deepLClient})
	if deepLTranslator == nil {
		t.Fatal("NewDeepLTranslator() = nil")
	}
	catalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {Source: []string{"English"}, Target: []string{"Spanish"}, AutoDetect: true},
		languages.ProviderDeepL:   {Source: []string{"EN"}, Target: []string{"ES"}, AutoDetect: true},
	})
	if err != nil {
		t.Fatalf("create language catalog: %v", err)
	}
	handler := NewTranslationHandler(gpt, deepLTranslator, 100, &translationDBStub{}, catalog)
	if handler == nil {
		t.Fatal("NewTranslationHandler() = nil")
	}
}
