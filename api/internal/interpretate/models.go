package interpretate

import (
	"context"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/helper"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/labstack/echo/v5"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"go.uber.org/zap"
)

// InterpretationService turns validated short text into structured meanings
// and can classify whether text is suitable for interpretation.
type InterpretationService interface {
	Interpretate(ctx context.Context, log *zap.Logger, tr *interpReq) (interpretation, pkg.Usage, error)
	AnalyzeText(ctx context.Context, log *zap.Logger, text string) (bool, pkg.Usage, error)
}

// InterpretationHandler registers and serves interpretation endpoints.
type InterpretationHandler interface {
	RegisterRoutes(g *echo.Group, authenticate echo.MiddlewareFunc)
	Interpretate(c *echo.Context) error
}

// DbHandler supplies installation validation and usage persistence.
type DbHandler interface {
	ValidateUserId(ctx context.Context, userID int64) (bool, error)
	helper.DbHandler
}

type interphandler struct {
	service   InterpretationService
	db        DbHandler
	validator *validation.Catalog
}

type interpservice struct {
	model         shared.ChatModel
	client        openai.Client
	db            DbHandler
	prompt        string
	analyzeprompt string
}

// NewInterpretationHandler panics unless all dependencies and the shared
// language catalog are available.
func NewInterpretationHandler(service InterpretationService, db DbHandler, catalog *languages.Catalog) InterpretationHandler {
	validation.MustNotNil("interpretation service", service)
	validation.MustNotNil("interpretation database", db)
	validation.MustNotNil("language catalog", catalog)
	validation.MustNotNil("language validator", catalog.Validator())
	return &interphandler{service: service, db: db, validator: catalog.Validator()}
}

// NewInterpretationService configures the model prompts used for structured
// interpretation and suitability analysis.
func NewInterpretationService(db DbHandler, model shared.ChatModel, client openai.Client, prompt, analyzeprompt string) InterpretationService {
	validation.MustNotNil("interpretation database", db)
	validation.MustNotBlank("OpenAI model", string(model))
	validation.MustNotZero("OpenAI client", client)
	prompt = validation.MustNotBlank("interpretation prompt", prompt)
	analyzeprompt = validation.MustNotBlank("analysis prompt", analyzeprompt)
	return &interpservice{
		model:         model,
		client:        client,
		db:            db,
		prompt:        prompt,
		analyzeprompt: analyzeprompt,
	}
}

type interpReq struct {
	TargetLang string
	UserId     int
	Text       string
}

type meaningCard struct {
	Context     string `json:"context" example:"Used when greeting someone"`
	Translation string `json:"translation" example:"hola"`
	Example     string `json:"example" example:"Hello, how are you?"`
}

type interpretation struct {
	PartOfSpeech string         `json:"part_of_speech"`
	MeaningCards []*meaningCard `json:"meanings"`
}

type textReq struct {
	Text string `json:"text" example:"break the ice"`
}
