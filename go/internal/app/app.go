package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/api"
	"saveinator/internal/config"
	"saveinator/internal/db"
	"saveinator/internal/handler"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/worker"
)

type App struct {
	cfg *config.Settings
}

func New(cfg *config.Settings) *App {
	return &App{cfg: cfg}
}

func (a *App) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := db.Connect(ctx, a.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	redisClient, err := redisx.Connect(a.cfg.RedisURL)
	if err != nil {
		return err
	}
	defer redisClient.Close()

	bot, err := telego.NewBot(a.cfg.BotToken, telego.WithDefaultLogger(false, false))
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	mode := a.cfg.Mode
	if mode == "" {
		mode = "all"
	}

	errCh := make(chan error, 3)
	if mode == "bot" || mode == "all" {
		go func() { errCh <- a.runBot(ctx, bot, store, redisClient) }()
	}
	if mode == "worker" || mode == "all" {
		go func() { errCh <- a.runWorker(ctx, bot, store, redisClient) }()
	}
	if a.cfg.MetricsEnabled && (mode == "bot" || mode == "all") {
		go func() { errCh <- a.runMetrics(ctx) }()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func (a *App) runBot(ctx context.Context, bot *telego.Bot, store *db.Store, redisClient *redisx.Client) error {
	q, err := queue.NewClient(a.cfg.RedisURL)
	if err != nil {
		return err
	}
	defer q.Close()

	var updates <-chan telego.Update
	if a.cfg.UsePolling {
		slog.Info("starting polling mode")
		updates, err = bot.UpdatesViaLongPolling(nil)
		if err != nil {
			return err
		}
	} else {
		updates, err = a.setupWebhook(ctx, bot)
		if err != nil {
			return err
		}
	}

	bh, err := th.NewBotHandler(bot, updates)
	if err != nil {
		return err
	}
	defer bh.Stop()

	handler.New(a.cfg, store, redisClient, q).Register(bh, bot)

	if err := a.registerBotCommands(bot); err != nil {
		slog.Warn("register bot commands failed", "err", err)
	}

	if a.cfg.UsePolling {
		bh.Start()
		return nil
	}

	go bh.Start()
	addr := fmt.Sprintf("%s:%d", a.cfg.WebhookListen, a.cfg.WebhookPort)
	slog.Info("starting webhook mode", "addr", addr)
	return bot.StartWebhook(addr)
}

func (a *App) setupWebhook(ctx context.Context, bot *telego.Bot) (<-chan telego.Update, error) {
	path := a.cfg.WebhookPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/", health)
	api.RegisterDownloadRoutes(mux, a.cfg)

	addr := fmt.Sprintf("%s:%d", a.cfg.WebhookListen, a.cfg.WebhookPort)
	server := &http.Server{Addr: addr, Handler: mux}

	whServer := telego.HTTPWebhookServer{
		Server:      server,
		ServeMux:    mux,
		SecretToken: a.cfg.WebhookSecretToken,
	}

	updates, err := bot.UpdatesViaWebhook(path,
		telego.WithWebhookServer(whServer),
		telego.WithWebhookSet(&telego.SetWebhookParams{
			URL:                a.cfg.WebhookURL(),
			SecretToken:        a.cfg.WebhookSecretToken,
			DropPendingUpdates: true,
		}),
	)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = bot.StopWebhookWithContext(shutdownCtx)
	}()

	return updates, nil
}

func (a *App) runWorker(ctx context.Context, bot *telego.Bot, store *db.Store, redisClient *redisx.Client) error {
	opt, err := queue.RedisOpt(a.cfg.RedisURL)
	if err != nil {
		return err
	}

	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: 1,
		Queues: map[string]int{
			"default": 1,
		},
	})

	mux := asynq.NewServeMux()
	worker.NewHandler(a.cfg, bot, store, redisClient).Register(mux)
	worker.StartMaintenance(ctx, a.cfg)

	go func() {
		<-ctx.Done()
		srv.Shutdown()
	}()

	slog.Info("starting asynq worker")
	return srv.Run(mux)
}

func (a *App) runMetrics(ctx context.Context) error {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Handle("/metrics", metrics.Handler())

	addr := fmt.Sprintf("%s:%d", a.cfg.MetricsHost, a.cfg.MetricsPort)
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("metrics server listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (a *App) registerBotCommands(bot *telego.Bot) error {
	defaultCommands := []telego.BotCommand{
		{Command: "start", Description: "Start / language"},
		{Command: "settings", Description: "User settings"},
	}
	if err := bot.SetMyCommands(&telego.SetMyCommandsParams{Commands: defaultCommands}); err != nil {
		return err
	}
	if a.cfg.AdminTelegramID > 0 {
		adminCommands := append(defaultCommands,
			telego.BotCommand{Command: "admin", Description: "Admin panel"},
			telego.BotCommand{Command: "stats", Description: "User statistics"},
			telego.BotCommand{Command: "broadcast", Description: "Send broadcast"},
		)
		_ = bot.SetMyCommands(&telego.SetMyCommandsParams{
			Commands: adminCommands,
			Scope: &telego.BotCommandScopeChat{
				Type:   "chat",
				ChatID: tu.ID(a.cfg.AdminTelegramID),
			},
		})
	}
	return bot.SetChatMenuButton(&telego.SetChatMenuButtonParams{
		MenuButton: &telego.MenuButtonCommands{Type: "commands"},
	})
}
