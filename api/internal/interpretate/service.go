package interpretate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/providerfailure"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"go.uber.org/zap"
	"moul.io/mdcodec"
)

const maxAttempt = 5

func waitForRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func assert(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}

// Interpretate gives provider and decoding failures one bounded retry budget.
// A terminal failure panics so request recovery can alert operators; context
// cancellation and deadline errors are returned to the caller.
func (ints *interpservice) Interpretate(ctx context.Context, log *zap.Logger, intr *interpReq) (interpretation, pkg.Usage, error) {
	validation.MustNotNil("context", ctx)
	validation.MustNotNil("logger", log)
	validation.MustNotNil("interpretation request", intr)
	validation.MustPositiveInt("user ID", intr.UserId)
	validation.MustNotBlank("target language", intr.TargetLang)
	assert(func() bool {
		ok, err := ints.db.ValidateUserId(ctx, int64(intr.UserId))
		if err != nil {
			log.Error("db query returned error", zap.Error(err))
			return false
		}
		return ok
	}(), "")
	if _, err := validation.ValidateInterpretationText(intr.Text); err != nil {
		panic(err)
	}

	var res interpretation
	var usage pkg.Usage

	md, err := mdcodec.Marshal(intr)
	if err != nil {
		log.Error("md marshal failed", zap.Error(err))
		return res, usage, err
	}

	// Provider failures and malformed model output share one retry budget: a
	// successful provider call is not useful until it satisfies the JSON contract.
	for attempt := 1; attempt <= maxAttempt; attempt++ {
		resp, requestErr := ints.client.Responses.New(ctx, responses.ResponseNewParams{
			Model:        ints.model,
			Instructions: openai.String(ints.prompt),
			Input:        responses.ResponseNewParamsInputUnion{OfString: openai.String(md)},
		})

		if requestErr != nil {
			err = requestErr
			log.Warn("open ai returned error", zap.Error(err))
			providerfailure.PanicIfOpenAIQuotaExceeded("interpretation", err)
			if attempt < maxAttempt {
				if err = waitForRetry(ctx, attempt); err != nil {
					return res, usage, err
				}
			}
			continue
		}

		usage.InputTokens += resp.Usage.InputTokens
		usage.OutputTokens += resp.Usage.OutputTokens

		if unmarshalErr := sonic.Unmarshal([]byte(resp.OutputText()), &res); unmarshalErr != nil {
			err = unmarshalErr
			log.Warn("coudln't unmarshal output text", zap.Error(err))
			if attempt < maxAttempt {
				if err = waitForRetry(ctx, attempt); err != nil {
					return res, usage, err
				}
			}
			continue
		}

		return res, usage, nil
	}

	log.Error("retry loop exited with an error", zap.Error(err))
	if ctx.Err() != nil {
		return res, usage, ctx.Err()
	}
	providerfailure.PanicOpenAI("interpretation", err)
	return res, usage, err
}

// AnalyzeText accepts only an exact, case-insensitive true or false response.
// Provider and contract failures share a bounded retry budget and panic when
// exhausted; context cancellation and deadline errors are returned.
func (ints *interpservice) AnalyzeText(ctx context.Context, log *zap.Logger, text string) (bool, pkg.Usage, error) {
	validation.MustNotNil("context", ctx)
	validation.MustNotNil("logger", log)
	if _, err := validation.ValidateInterpretationText(text); err != nil {
		panic(err)
	}

	var err error
	var usage pkg.Usage
	// A response outside the strict true/false contract is retriable just like a
	// provider error; callers must never receive an ambiguous classification.
	for attempt := 1; attempt <= maxAttempt; attempt++ {
		resp, requestErr := ints.client.Responses.New(ctx, responses.ResponseNewParams{
			Model:        ints.model,
			Instructions: openai.String(ints.analyzeprompt),
			Input:        responses.ResponseNewParamsInputUnion{OfString: openai.String(text)},
		})

		if requestErr != nil {
			err = requestErr
			log.Warn("open ai returned error", zap.Error(err))
			providerfailure.PanicIfOpenAIQuotaExceeded("text analysis", err)
			if attempt < maxAttempt {
				if err = waitForRetry(ctx, attempt); err != nil {
					return false, usage, err
				}
			}
			continue
		}

		usage.InputTokens += resp.Usage.InputTokens
		usage.OutputTokens += resp.Usage.OutputTokens

		s := strings.ToLower(resp.OutputText())
		switch s {
		case "true":
			return true, usage, nil
		case "false":
			return false, usage, nil
		default:
			err = fmt.Errorf("unexpected analysis response: %q", s)
			log.Warn("unexpected response", zap.String("text", s))
			if attempt < maxAttempt {
				if err = waitForRetry(ctx, attempt); err != nil {
					return false, usage, err
				}
			}
			continue
		}
	}

	log.Error("retry loop exited with an error", zap.Error(err))
	if ctx.Err() != nil {
		return false, usage, ctx.Err()
	}
	providerfailure.PanicOpenAI("text analysis", err)
	return false, usage, err
}
