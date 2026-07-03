package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/config"
	"saveinator/internal/db"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/sender"
	"saveinator/internal/worker"
	spotifyhandler "saveinator/services/spotify/handler"
	spotifyworker "saveinator/services/spotify/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.LogLevel == "DEBUG" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	if err := run(cfg); err != nil {
		slog.Error("application stopped", "err", err)
		os.Exit(1)
	}
}

func run(cfg *config.Settings) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	redisClient, err := redisx.Connect(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer redisClient.Close()

	metrics.SetActiveUserCounter(func(ctx context.Context) int {
		n, err := redisClient.CountActiveUsers(ctx, 30*time.Minute)
		if err != nil {
			return 0
		}
		return n
	})

	bot, err := telego.NewBot(cfg.BotToken, telego.WithDefaultLogger(false, false))
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	mode := cfg.Mode
	if mode == "" {
		mode = "all"
	}

	errCh := make(chan error, 3)
	if mode == "bot" || mode == "all" {
		go func() { errCh <- runBot(ctx, bot, store, redisClient, cfg) }()
	}
	if mode == "worker" || mode == "all" {
		go func() { errCh <- runWorker(ctx, bot, store, redisClient, cfg) }()
	}
	if cfg.MetricsEnabled {
		go func() { errCh <- runMetrics(ctx, cfg) }()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func runBot(ctx context.Context, bot *telego.Bot, store *db.Store, redisClient *redisx.Client, cfg *config.Settings) error {
	q, err := queue.NewClient(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer q.Close()

	var updates <-chan telego.Update
	if cfg.UsePolling {
		slog.Info("starting polling mode (spotify)")
		updates, err = bot.UpdatesViaLongPolling(nil)
		if err != nil {
			return err
		}
	} else {
		updates, err = setupWebhook(ctx, bot, cfg)
		if err != nil {
			return err
		}
	}

	bh, err := th.NewBotHandler(bot, updates)
	if err != nil {
		return err
	}
	defer bh.Stop()

	spotifyhandler.New(cfg, store, redisClient, q).Register(bh, bot)

	if err := registerBotCommands(bot, cfg); err != nil {
		slog.Warn("register bot commands failed", "err", err)
	}

	if cfg.UsePolling {
		bh.Start()
		return nil
	}

	go bh.Start()
	addr := fmt.Sprintf("%s:%d", cfg.WebhookListen, cfg.WebhookPort)
	slog.Info("starting webhook mode (spotify)", "addr", addr)
	return bot.StartWebhook(addr)
}

func setupWebhook(ctx context.Context, bot *telego.Bot, cfg *config.Settings) (<-chan telego.Update, error) {
	path := cfg.WebhookPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/", health)

	addr := fmt.Sprintf("%s:%d", cfg.WebhookListen, cfg.WebhookPort)
	server := &http.Server{Addr: addr, Handler: mux}

	whServer := telego.HTTPWebhookServer{
		Server:      server,
		ServeMux:    mux,
		SecretToken: cfg.WebhookSecretToken,
	}

	updates, err := bot.UpdatesViaWebhook(path,
		telego.WithWebhookServer(whServer),
		telego.WithWebhookSet(&telego.SetWebhookParams{
			URL:                cfg.WebhookURL(),
			SecretToken:        cfg.WebhookSecretToken,
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

func runWorker(ctx context.Context, bot *telego.Bot, store *db.Store, redisClient *redisx.Client, cfg *config.Settings) error {
	opt, err := queue.RedisOpt(cfg.RedisURL)
	if err != nil {
		return err
	}

	if err := queue.RecoverOrphanedActiveTasks(cfg.RedisURL, queue.SpotifyQueueName); err != nil {
		slog.Warn("asynq queue recovery failed", "err", err)
	}

	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: 2,
		Queues: map[string]int{
			queue.SpotifyQueueName: 1,
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			slog.Warn("asynq task failed", "type", task.Type(), "err", err)
			metrics.RecordCeleryTask(task.Type(), "FAILURE")
			metrics.RecordError("worker")
		}),
	})

	mux := asynq.NewServeMux()
	snd := sender.NewWithUsername(bot, sender.ResolveBotUsername(bot, cfg.BotUsername))
	spotifyworker.NewHandler(cfg, snd, store, redisClient).Register(mux)

	worker.StartTempfileSweep(ctx)

	go func() {
		<-ctx.Done()
		srv.Shutdown()
	}()

	slog.Info("starting asynq worker (spotify)")
	return srv.Run(mux)
}

func runMetrics(ctx context.Context, cfg *config.Settings) error {
	addr := fmt.Sprintf("%s:%d", cfg.MetricsHost, cfg.MetricsPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: metrics.Handler(),
	}

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

func health(w http.ResponseWriter, r *http.Request) {
	metrics.HTTPMiddleware("/health", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})).ServeHTTP(w, r)
}

func registerBotCommands(bot *telego.Bot, cfg *config.Settings) error {
	defaultCommands := []telego.BotCommand{
		{Command: "start", Description: "Start"},
		{Command: "lang", Description: "Change language"},
		{Command: "settings", Description: "User settings"},
		{Command: "clear", Description: "Clear your download queue"},
	}
	if err := bot.SetMyCommands(&telego.SetMyCommandsParams{Commands: defaultCommands}); err != nil {
		return err
	}
	if cfg.AdminTelegramID > 0 {
		adminCommands := append(defaultCommands,
			telego.BotCommand{Command: "admin", Description: "Admin panel"},
			telego.BotCommand{Command: "stats", Description: "User statistics"},
		)
		_ = bot.SetMyCommands(&telego.SetMyCommandsParams{
			Commands: adminCommands,
			Scope: &telego.BotCommandScopeChat{
				Type:   "chat",
				ChatID: tu.ID(cfg.AdminTelegramID),
			},
		})
	}
	return bot.SetChatMenuButton(&telego.SetChatMenuButtonParams{
		MenuButton: &telego.MenuButtonCommands{Type: "commands"},
	})
}
