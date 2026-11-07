// Command ai_translate runs the HTTP API and wires its providers, persistence,
// middleware, and route handlers.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/bgdimetrius/AITranslate/docs/swagger"
	"github.com/bgdimetrius/AITranslate/internal/analytics"
	"github.com/bgdimetrius/AITranslate/internal/auth"
	"github.com/bgdimetrius/AITranslate/internal/interpretate"
	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/internal/translate"
	"github.com/bgdimetrius/AITranslate/internal/users"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/config"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	applog "github.com/bgdimetrius/AITranslate/pkg/logger"
	"github.com/bgdimetrius/AITranslate/pkg/paniccatcher"
	"github.com/cluttrdev/deepl-go/deepl"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"
)

func createPool(ctx context.Context, cfg config.DatabaseConfig) (*queries.Queries, *pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse database configuration: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConnections
	poolConfig.MinConns = cfg.MinConnections
	poolConfig.MinIdleConns = cfg.MinIdleConnections

	poolConfig.MaxConnLifetime = cfg.MaxConnectionLifetime
	poolConfig.MaxConnLifetimeJitter = cfg.LifetimeJitter
	poolConfig.MaxConnIdleTime = cfg.MaxConnectionIdleTime

	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod
	poolConfig.PingTimeout = cfg.PingTimeout
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	poolConfig.ConnConfig.RuntimeParams["application_name"] = cfg.ApplicationName
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = cfg.StatementTimeout.String()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("create database pool: %w", err)
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancelPing()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}

	return queries.New(pool), pool, nil
}

func languageCodes(providerLanguages []deepl.Language) []string {
	codes := make([]string, 0, len(providerLanguages))
	for _, language := range providerLanguages {
		codes = append(codes, language.Code)
	}
	return codes
}

// @title AI Translate API
// @version 1.0
// @description Translation, interpretation, authentication, and feedback API.
// @description Authenticated operations use the HttpOnly access_token cookie. Swagger 2 models it as the Cookie request header.
// @description Use access_token=<JWT> as the CookieAuth value; /api/v1/auth/handshake additionally requires refresh_token=<token> in the same header.
// @description A successful handshake immediately supersedes the prior access token and rejects replay of the prior refresh token.
// @description Every operation passes through request-ID assignment, request logging, panic recovery, deadline propagation, security headers, CORS, per-IP rate limiting, and the global body-size limit before route middleware.
// @BasePath /api/v1
// @schemes http https
// @tag.name users
// @tag.description Anonymous installation provisioning.
// @tag.name auth
// @tag.description Session refresh and token rotation.
// @tag.name translation
// @tag.description Text translation through configured providers.
// @tag.name interpretation
// @tag.description Short-text analysis and contextual meanings.
// @tag.name analytics
// @tag.description Authenticated product feedback and lead capture.
// @securityDefinitions.apikey CookieAuth
// @in header
// @name Cookie
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log, err := applog.New(cfg.App.LogLevel)
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	var telegramBot *tgbotapi.BotAPI
	if cfg.Telegram.BotAPIKey != "" {
		// Bot construction calls getMe to validate the credential. The same
		// timeout bounds later alert delivery so Telegram cannot stall recovery.
		telegramBot, err = tgbotapi.NewBotAPIWithClient(
			cfg.Telegram.BotAPIKey,
			tgbotapi.APIEndpoint,
			&http.Client{Timeout: cfg.Telegram.RequestTimeout},
		)
		if err != nil {
			log.Fatal("Telegram bot initialization failed", zap.Error(err))
		}
		log.Info("Telegram panic alerts enabled", zap.String("bot_username", telegramBot.Self.UserName))
	}

	queries, pool, err := createPool(context.Background(), cfg.Database)
	if err != nil {
		log.Fatal("database initialization failed", zap.Error(err))
	}
	defer pool.Close()

	auth.Init(cfg.Auth.JWTSecret, cfg.Auth.SecureCookies)

	gpt := openai.NewClient(
		option.WithAPIKey(cfg.OpenAI.APIKey),
		option.WithRequestTimeout(cfg.OpenAI.RequestTimeout),
		option.WithMaxRetries(cfg.OpenAI.MaxRetries),
	)

	translators := make([]*deepl.Translator, 0, len(cfg.DeepL.APIKeys))
	for _, key := range cfg.DeepL.APIKeys {
		t, err := deepl.NewTranslator(key)
		if err != nil {
			log.Fatal("DeepL client initialization failed", zap.Error(err))
		}
		translators = append(translators, t)
	}
	deepLSourceLanguages, err := translators[0].GetLanguages("source")
	if err != nil {
		log.Fatal("DeepL source-language discovery failed", zap.Error(err))
	}
	deepLTargetLanguages, err := translators[0].GetLanguages("target")
	if err != nil {
		log.Fatal("DeepL target-language discovery failed", zap.Error(err))
	}
	languageCatalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {
			Source:     cfg.OpenAI.TranslationLanguages,
			Target:     cfg.OpenAI.TranslationLanguages,
			AutoDetect: true,
		},
		languages.ProviderDeepL: {
			Source:     languageCodes(deepLSourceLanguages),
			Target:     languageCodes(deepLTargetLanguages),
			AutoDetect: true,
		},
	})
	if err != nil {
		log.Fatal("language catalog initialization failed", zap.Error(err))
	}

	model := shared.ChatModel(cfg.OpenAI.Model)
	analyticsService := analytics.NewAnalyticsService(queries)
	interetationService := interpretate.NewInterpretationService(queries, model, gpt, cfg.Prompts.Interpret, cfg.Prompts.Analyze)
	gptTranslator := translate.NewGptTranslator(gpt, model, cfg.Prompts.Translate)
	deeplTranslator := translate.NewDeepLTranslator(translators)

	analyticsHandler := analytics.NewAnalyticsHandlerWithValidation(
		analyticsService,
		languageCatalog,
		cfg.Limits.TranslationCharacters,
	)
	interetationHandler := interpretate.NewInterpretationHandler(interetationService, queries, languageCatalog)
	authHandler := auth.NewAuthHandler(queries)
	translationHandler := translate.NewTranslationHandler(gptTranslator, deeplTranslator, cfg.Limits.TranslationCharacters, queries, languageCatalog)
	usersHandler := users.NewUserHandler(queries)

	e := echo.New()

	e.IPExtractor = echo.ExtractIPFromXFFHeader(
		echo.TrustLoopback(true),
		echo.TrustPrivateNet(true),
	)

	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		RequestIDHandler: func(c *echo.Context, requestID string) {
			c.Set("logger", log.With(zap.String("request_id", requestID)))
			if telegramBot != nil {
				paniccatcher.SetTelegramAlertTarget(c, telegramBot, cfg.Telegram.ChatID)
			}
		},
	}))
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:   true,
		LogURIPath:  true, // path only: the text to check rides in the query string and must never be logged
		LogStatus:   true,
		LogLatency:  true,
		LogRemoteIP: true,
		HandleError: true, // let the global error handler set the status before we log it
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			requestLog, ok := c.Get("logger").(*zap.Logger)
			if !ok {
				requestLog = log
			}
			fields := []zap.Field{
				zap.String("remote_ip", v.RemoteIP),
				zap.String("method", v.Method),
				zap.String("path", v.URIPath),
				zap.Int("status", v.Status),
				zap.Duration("latency", v.Latency),
			}
			if v.Error != nil {
				fields = append(fields, zap.Error(v.Error))
			}

			switch {
			case v.Status >= http.StatusInternalServerError:
				requestLog.Error("request completed", fields...)
			case v.Status >= http.StatusBadRequest:
				requestLog.Warn("request completed", fields...)
			default:
				requestLog.Info("request completed", fields...)
			}
			return nil
		},
	}))
	// Keep recovery inside the request logger so recovered panics are emitted as
	// error-level request records instead of bypassing access logging.
	e.Use(paniccatcher.Middleware(log))
	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: cfg.Server.RequestTimeout,
		ErrorHandler: func(_ *echo.Context, err error) error {
			if err == nil {
				return nil
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return echo.NewHTTPError(http.StatusGatewayTimeout, pkg.RequestTimedOut).Wrap(err)
			}
			return err
		},
	}))
	e.Use(middleware.Secure())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.Server.CORSOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderContentType},
	}))
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStoreWithConfig(
		middleware.RateLimiterMemoryStoreConfig{
			Rate:      cfg.Server.RateLimit,
			Burst:     cfg.Server.RateBurst,
			ExpiresIn: cfg.Server.RateLimiterExpiry,
		},
	)))
	globalBodyLimit, err := middleware.BodyLimitConfig{
		LimitBytes: cfg.Server.MaxBodyBytes,
	}.ToMiddleware()
	if err != nil {
		log.Fatal("body limit configuration is invalid", zap.Error(err))
	}
	e.Use(globalBodyLimit)

	// Keep this endpoint outside the versioned API so container and deployment
	// health checks remain stable across public API revisions.
	e.GET("/health", func(c *echo.Context) error {
		pingCtx, cancelPing := context.WithTimeout(c.Request().Context(), cfg.Database.PingTimeout)
		defer cancelPing()
		if err := pool.Ping(pingCtx); err != nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "service unavailable").Wrap(err)
		}
		return c.NoContent(http.StatusNoContent)
	})

	// Keep the static Swagger @BasePath synchronized with the deployed APP_VERSION.
	api := e.Group("/api/" + cfg.App.Version)
	authenticate := auth.Authenticate(queries)

	analyticsHandler.RegisterRoutes(api, authenticate)
	languageCatalog.RegisterRoutes(api)
	interetationHandler.RegisterRoutes(api, authenticate)
	authHandler.RegisterRoutes(api)
	translationHandler.RegisterRoutes(api, authenticate)
	usersHandler.RegisterRoutes(api)

	// The Swagger handler serves the UI here; a generated specification must be
	// registered separately before /swagger/doc.json becomes available.
	api.GET("/swagger/*", echo.WrapHandler(httpSwagger.WrapHandler))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sc := echo.StartConfig{
		Address:         ":" + cfg.Server.Port,
		GracefulTimeout: cfg.Server.GracefulTimeout,
		BeforeServeFunc: func(s *http.Server) error {
			s.ReadHeaderTimeout = cfg.Server.ReadHeaderTimeout
			s.ReadTimeout = cfg.Server.ReadTimeout
			s.WriteTimeout = cfg.Server.WriteTimeout
			s.IdleTimeout = cfg.Server.IdleTimeout
			return nil
		},
	}
	if err := sc.Start(ctx, e); err != nil {
		log.Fatal("server stopped", zap.Error(err))
	}
}
