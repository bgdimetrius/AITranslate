package translate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/providerfailure"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/bytedance/sonic"
	"github.com/cluttrdev/deepl-go/deepl"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"go.uber.org/zap"
	"moul.io/mdcodec"
)

func (tr *translationReq) check() {
	validation.MustPositiveInt("allowed translation text length", tr.allowedLength)
	validation.MustNotBlank("target language", tr.TargetLang)
	if tr.withSource {
		validation.MustNotBlank("source language", tr.SourceLang)
	}
	if tr.Text == "" || len(tr.Text) > tr.allowedLength {
		panic(validation.ErrInvalidText)
	}
}

func checkDo(ctx context.Context, w io.Writer, tr *translationReq) {
	validation.MustNotNil("context", ctx)
	validation.MustNotNil("response writer", w)
	validation.MustNotNil("translation request", tr)
	tr.check()
}

// Do streams OpenAI text deltas directly to w. Writer errors may be returned
// after partial output; terminal provider or stream-contract failures panic so
// request recovery can alert operators. Usage is complete only after the
// provider emits its completion event.
func (gt *gptTranslator) Do(ctx context.Context, log *zap.Logger, w io.Writer, tr *translationReq) (pkg.Usage, error) {
	checkDo(ctx, w, tr)
	validation.MustNotNil("logger", log)
	log.Debug("gptTranslator.Do", zap.Any("ctx", ctx == nil), zap.Any("w", w), zap.Any("tr", tr))
	var usage pkg.Usage

	md, err := mdcodec.Marshal(tr)
	if err != nil {
		log.Error("md marshal failed", zap.Error(err))
		return usage, err
	}

	stream := gt.client.Responses.NewStreaming(ctx, responses.ResponseNewParams{
		Model:        gt.model,
		Instructions: openai.String(gt.prompt),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(md),
		},
	})
	log.Debug("open ai stream is formed")

	defer func() {
		log.Debug("open ai stream is finished")
		_ = stream.Close()
	}()

	var terminalState string
	var terminalCode string
	completed := false
	wroteOutput := false
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.completed":
			// Token totals are authoritative only on the terminal completion
			// event; intermediate deltas do not carry final usage.
			completed = true
			usage.InputTokens += event.Response.Usage.InputTokens
			usage.OutputTokens += event.Response.Usage.OutputTokens
		case "response.failed":
			terminalState = "response failed"
			terminalCode = string(event.Response.Error.Code)
		case "response.incomplete":
			terminalState = "response incomplete"
			terminalCode = event.Response.IncompleteDetails.Reason
		case "error":
			terminalState = "stream failed"
			terminalCode = event.Code
		case "response.output_text.delta":
			if event.Delta == "" {
				continue
			}
			if _, err = w.Write([]byte(event.Delta)); err != nil {
				log.Error("unable to write response into writer", zap.Error(err))
				return usage, err
			}
			wroteOutput = true
		}
	}
	log.Debug("open ai stream has been processed")

	if stream.Err() != nil {
		err = stream.Err()
		log.Error("open ai stream error", zap.Error(err))
		if ctx.Err() != nil {
			return usage, ctx.Err()
		}
		providerfailure.PanicOpenAI("translation", err)
	}
	if terminalState != "" {
		providerfailure.PanicOpenAIResponse("translation", terminalState, terminalCode)
	}
	if !completed {
		providerfailure.PanicOpenAI("translation", errors.New("stream ended without a completion event"))
	}
	if !wroteOutput {
		providerfailure.PanicOpenAI("translation", errors.New("completed response contained no translated text"))
	}

	return usage, nil
}

func waitForDeepLRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func deepLUsageAllows(usage *deepl.Usage, characters int) bool {
	// A non-positive limit cannot establish exhaustion, so only positive limits
	// participate in the preflight check.
	return usage.CharacterLimit <= 0 || usage.CharacterCount+characters <= usage.CharacterLimit
}

func isDeepLQuotaError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "456 -")
}

// Do checks each configured DeepL account before use and tries accounts in
// order. One exhausted account is skipped; quota exhaustion becomes a panic
// only after every configured account is unable to fit the complete request.
// Other provider failures retain a bounded retry budget and become a panic
// only when no account can produce a translation.
func (dt *deeplTranslator) Do(ctx context.Context, log *zap.Logger, w io.Writer, tr *translationReq) (pkg.Usage, error) {
	checkDo(ctx, w, tr)
	validation.MustNotNil("logger", log)
	log.Debug("deeplTranslator.Do", zap.Any("ctx", ctx == nil), zap.Any("w", w), zap.Any("tr", tr))
	var usage pkg.Usage
	const maxAttempt = 5
	characters := len([]rune(tr.Text))

	if len(dt.translators) == 0 {
		panic(errors.New("DeepL translation failed: no translators are configured"))
	}

	var lastErr error
	exhaustedTranslators := 0

	for i, translator := range dt.translators {
		for attempt := 1; attempt <= maxAttempt; attempt++ {
			log.Debug("checking DeepL translator", zap.Int("translator", i), zap.Int("attempt", attempt))
			var options []deepl.TranslateOption
			if tr.withSource {
				options = append(options, deepl.WithSourceLang(tr.SourceLang))
			}

			deeplUsage, usageErr := translator.GetUsage()
			if usageErr != nil {
				lastErr = fmt.Errorf("DeepL translator %d usage check failed: %w", i, usageErr)
				log.Warn("DeepL usage check failed", zap.Int("translator", i), zap.Error(usageErr))
				if attempt < maxAttempt {
					if waitErr := waitForDeepLRetry(ctx, attempt); waitErr != nil {
						return usage, waitErr
					}
				}
				continue
			}

			if !deepLUsageAllows(deeplUsage, characters) {
				exhaustedTranslators++
				log.Warn("DeepL translator lacks quota for request",
					zap.Int("translator", i),
					zap.Int("character_count", deeplUsage.CharacterCount),
					zap.Int("character_limit", deeplUsage.CharacterLimit),
					zap.Int("required_characters", characters),
				)
				break
			}

			translation, translateErr := translator.TranslateText([]string{tr.Text}, tr.TargetLang, options...)
			if translateErr != nil {
				lastErr = fmt.Errorf("DeepL translator %d request failed: %w", i, translateErr)
				if isDeepLQuotaError(translateErr) {
					// Usage can change between the preflight and translation when
					// requests race, so a provider-side quota response is authoritative.
					exhaustedTranslators++
					log.Warn("DeepL translator exhausted during request", zap.Int("translator", i))
					break
				}
				log.Warn("DeepL translation failed", zap.Int("translator", i), zap.Error(translateErr))
				if attempt < maxAttempt {
					if waitErr := waitForDeepLRetry(ctx, attempt); waitErr != nil {
						return usage, waitErr
					}
				}
				continue
			}

			if len(translation) < 1 {
				lastErr = errors.New("DeepL returned no translations")
				log.Warn("DeepL returned an empty translation array")
				if attempt < maxAttempt {
					if waitErr := waitForDeepLRetry(ctx, attempt); waitErr != nil {
						return usage, waitErr
					}
				}
				continue
			}

			bytes, err := sonic.Marshal(&deeplResp{
				SourceLang: translation[0].DetectedSourceLanguage,
				Text:       translation[0].Text,
			})
			if err != nil {
				log.Error("json marshal failed", zap.Error(err))
				return usage, err
			}

			_, err = w.Write(bytes)
			if err != nil {
				log.Error("unable to write response into writer", zap.Error(err))
				return usage, err
			}

			return usage, nil
		}
	}

	if exhaustedTranslators == len(dt.translators) {
		panic(fmt.Errorf("DeepL quota exhausted: all %d configured translators lack quota for a %d-character request", len(dt.translators), characters))
	}
	if ctx.Err() != nil {
		return usage, ctx.Err()
	}
	if lastErr == nil {
		lastErr = errors.New("no DeepL translator produced a result")
	}
	panic(fmt.Errorf("DeepL translation failed after trying %d configured translators: %w", len(dt.translators), lastErr))
}
