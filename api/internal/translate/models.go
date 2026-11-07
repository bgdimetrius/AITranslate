package translate

import (
	"context"
	"io"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/cluttrdev/deepl-go/deepl"
	"github.com/labstack/echo/v5"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"go.uber.org/zap"
)

// Translator writes a provider response to w and reports any billable usage.
// Implementations may write partial output before returning an error.
type Translator interface {
	Do(ctx context.Context, log *zap.Logger, w io.Writer, tr *translationReq) (pkg.Usage, error)
}

type gptTranslator struct {
	model  shared.ChatModel
	client openai.Client
	prompt string
}

type deepLClient interface {
	GetUsage() (*deepl.Usage, error)
	TranslateText(text []string, targetLang string, opts ...deepl.TranslateOption) ([]deepl.Translation, error)
}

type deeplTranslator struct {
	translators []deepLClient
}

// NewGptTranslator returns a translator that streams OpenAI response deltas to
// the caller.
func NewGptTranslator(client openai.Client, model shared.ChatModel, prompt string) Translator {
	validation.MustNotZero("OpenAI client", client)
	validation.MustNotBlank("OpenAI model", string(model))
	prompt = validation.MustNotBlank("translation prompt", prompt)
	return &gptTranslator{
		model:  model,
		client: client,
		prompt: prompt,
	}
}

// NewDeepLTranslator returns a translator that tries the supplied DeepL
// clients in order until one produces a translation.
func NewDeepLTranslator(translators []*deepl.Translator) Translator {
	validation.MustPositiveInt("DeepL translator count", len(translators))
	clients := make([]deepLClient, len(translators))
	for i, translator := range translators {
		validation.MustNotNil("DeepL translator", translator)
		clients[i] = translator
	}
	return &deeplTranslator{translators: clients}
}

type translationReq struct {
	SourceLang string `json:"source_lang" md:"source language"`
	TargetLang string `json:"target_lang" md:"target language"`
	UserId     int
	Text       string `json:"text" md:"text"`

	withSource    bool
	allowedLength int
}

// TranslationHandler registers and serves the translation endpoint.
type TranslationHandler interface {
	RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc)
	Translate(c *echo.Context) error
}

// DbHandler is the persistence contract required to record translation usage.
type DbHandler interface {
	helper.DbHandler
}

type trslhandler struct {
	gpt               Translator
	deepl             Translator
	allowedTextLength int
	db                DbHandler
	validator         *validation.Catalog
}

type deeplResp struct {
	SourceLang string `json:"source_lang"`
	Text       string `json:"text"`
}

// NewTranslationHandler selects between gpt and deepl per request and rejects
// input that exceeds allowedTextLength before invoking either provider.
func NewTranslationHandler(gpt, deepl Translator, allowedTextLength int, db DbHandler, catalog *languages.Catalog) TranslationHandler {
	validation.MustNotNil("ChatGPT translator", gpt)
	validation.MustNotNil("DeepL translator", deepl)
	validation.MustPositiveInt("allowed translation text length", allowedTextLength)
	validation.MustNotNil("translation database", db)
	validation.MustNotNil("language catalog", catalog)
	validation.MustNotNil("language validator", catalog.Validator())
	return &trslhandler{
		gpt:               gpt,
		deepl:             deepl,
		allowedTextLength: allowedTextLength,
		db:                db,
		validator:         catalog.Validator(),
	}
}
