package botkit

import (
	"context"
	"fmt"
	"io"
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
	"gopkg.in/yaml.v3"

	"saveinator/internal/api"
	"saveinator/internal/botkit/botworker"
	"saveinator/internal/config"
	"saveinator/internal/db"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/sender"
	"saveinator/internal/worker"
)

// BotSpec is the YAML form of one bot in bots.yaml.
type BotSpec struct {
	Slug            string   `yaml:"slug"`
	TokenEnv        string   `yaml:"token_env"`
	Username        string   `yaml:"username"`
	Languages       []string `yaml:"languages"`
	DefaultLang     string   `yaml:"default_lang"`
	WelcomeKey      string   `yaml:"welcome_key"`
	NotSupportedKey string   `yaml:"not_supported_key"`
	Queue           string   `yaml:"queue"`
	RegisterAPI     bool     `yaml:"register_api"`
	Platforms       []string `yaml:"platforms"`
	Concurrency     int      `yaml:"concurrency"`
}

type Fleet struct {
	Bots []BotSpec `yaml:"bots"`
}

// LoadFleet reads bots.yaml.
func LoadFleet(path string) (*Fleet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fleet config: %w", err)
	}
	var f Fleet
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse fleet config: %w", err)
	}
	if len(f.Bots) == 0 {
		return nil, fmt.Errorf("fleet config %s has no bots", path)
	}
	return &f, nil
}

// ---- Platform registry ----

var platformRegistry = map[string]func() Platform{}

// RegisterPlatform makes a platform constructor available to bots.yaml by
// name. Platform packages call this from init().
func RegisterPlatform(name string, ctor func() Platform) {
	platformRegistry[name] = ctor
}

func (s *BotSpec) toBotConfig() (*BotConfig, string, error) {
	if s.TokenEnv == "" {
		return nil, "", fmt.Errorf("bot %q: token_env is required", s.Slug)
	}
	token := os.Getenv(s.TokenEnv)
	if token == "" {
		return nil, "", fmt.Errorf("bot %q: env %s is empty", s.Slug, s.TokenEnv)
	}
	var platforms []Platform
	for _, name := range s.Platforms {
		ctor, ok := platformRegistry[name]
		if !ok {
			return nil, "", fmt.Errorf("bot %q: unknown platform %q", s.Slug, name)
		}
		platforms = append(platforms, ctor())
	}
	bc := &BotConfig{
		Slug:            s.Slug,
		Languages:       s.Languages,
		DefaultLang:     s.DefaultLang,
		WelcomeKey:      s.WelcomeKey,
		NotSupportedKey: s.NotSupportedKey,
		Queue:           s.Queue,
		RegisterAPI:     s.RegisterAPI,
		Platforms:       platforms,
	}
	if err := bc.normalize(); err != nil {
		return nil, "", err
	}
	return bc, token, nil
}

// ---- Shared webhook server ----

// sharedWebhookServer implements telego.WebhookServer on a mux owned by the
// fleet: handler registration only, the fleet runs the HTTP server itself.
type sharedWebhookServer struct {
	mux         *http.ServeMux
	secretToken string
}

func (s *sharedWebhookServer) Start(string) error           { return nil }
func (s *sharedWebhookServer) Stop(context.Context) error   { return nil }

func (s *sharedWebhookServer) RegisterHandler(path string, handler telego.WebhookHandler) error {
	s.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.secretToken != "" && r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != s.secretToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := handler(context.WithoutCancel(r.Context()), data); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return nil
}

// ---- Fleet runtime ----

// FleetMain is the botd entrypoint: config + logging + fleet run.
func FleetMain(fleetPath string) {
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

	fleet, err := LoadFleet(fleetPath)
	if err != nil {
		slog.Error("fleet config error", "err", err)
		os.Exit(1)
	}

	if err := RunFleet(context.Background(), cfg, fleet); err != nil {
		slog.Error("fleet stopped", "err", err)
		os.Exit(1)
	}
}

// RunFleet runs all bots of the fleet in one process: one HTTP server with a
// webhook path per bot, one asynq server per bot, one metrics endpoint.
func RunFleet(ctx context.Context, cfg *config.Settings, fleet *Fleet) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	baseRedis, err := redisx.Connect(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer baseRedis.Close()

	metrics.SetActiveUserCounter(func(ctx context.Context) int {
		n, err := baseRedis.CountActiveUsers(ctx, 30*time.Minute)
		if err != nil {
			return 0
		}
		return n
	})

	q, err := queue.NewClient(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer q.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	apiRegistered := false

	mode := cfg.Mode
	if mode == "" {
		mode = "all"
	}

	errCh := make(chan error, len(fleet.Bots)*2+2)

	for i := range fleet.Bots {
		spec := &fleet.Bots[i]
		bc, token, err := spec.toBotConfig()
		if err != nil {
			return err
		}

		tgBot, err := telego.NewBot(token, telego.WithDefaultLogger(false, false))
		if err != nil {
			return fmt.Errorf("bot %q: create: %w", bc.Slug, err)
		}

		botRedis := baseRedis.WithPrefix(bc.Slug)

		if bc.RegisterAPI && !apiRegistered {
			api.RegisterDownloadRoutes(mux, cfg)
			apiRegistered = true
		}

		if mode == "bot" || mode == "all" {
			if err := startFleetBot(ctx, cfg, bc, tgBot, store, botRedis, q, mux, errCh); err != nil {
				return err
			}
		}
		if mode == "worker" || mode == "all" {
			username := spec.Username
			concurrency := spec.Concurrency
			go func() {
				errCh <- runFleetWorker(ctx, cfg, bc, tgBot, store, botRedis, username, concurrency)
			}()
		}
	}

	worker.StartTempfileSweep(ctx)

	if mode == "bot" || mode == "all" {
		addr := fmt.Sprintf("%s:%d", cfg.WebhookListen, cfg.WebhookPort)
		srv := &http.Server{Addr: addr, Handler: mux}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancelSrv := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelSrv()
			_ = srv.Shutdown(shutdownCtx)
		}()
		go func() {
			slog.Info("fleet webhook server listening", "addr", addr, "bots", len(fleet.Bots))
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
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

func startFleetBot(ctx context.Context, cfg *config.Settings, bc *BotConfig, tgBot *telego.Bot, store *db.Store, botRedis *redisx.Client, q *queue.Client, mux *http.ServeMux, errCh chan<- error) error {
	var updates <-chan telego.Update
	var err error

	if cfg.UsePolling {
		slog.Info("starting polling mode", "bot", bc.Slug)
		updates, err = tgBot.UpdatesViaLongPolling(nil)
		if err != nil {
			return fmt.Errorf("bot %q: polling: %w", bc.Slug, err)
		}
	} else {
		path := "/webhook/" + bc.Slug
		whURL := strings.TrimRight(cfg.WebhookHost, "/") + path
		updates, err = tgBot.UpdatesViaWebhook(path,
			telego.WithWebhookServer(&sharedWebhookServer{mux: mux, secretToken: cfg.WebhookSecretToken}),
			telego.WithWebhookSet(&telego.SetWebhookParams{
				URL:                whURL,
				SecretToken:        cfg.WebhookSecretToken,
				DropPendingUpdates: true,
			}),
		)
		if err != nil {
			return fmt.Errorf("bot %q: webhook: %w", bc.Slug, err)
		}
		slog.Info("webhook registered", "bot", bc.Slug, "url", whURL)
	}

	bh, err := th.NewBotHandler(tgBot, updates)
	if err != nil {
		return fmt.Errorf("bot %q: handler: %w", bc.Slug, err)
	}

	New(bc, cfg, store, botRedis, q).Register(bh, tgBot)

	if err := registerBotCommands(tgBot, cfg); err != nil {
		slog.Warn("register bot commands failed", "err", err, "bot", bc.Slug)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("bot %q: handler panic: %v", bc.Slug, r)
			}
		}()
		bh.Start()
	}()
	go func() {
		<-ctx.Done()
		bh.Stop()
	}()
	return nil
}

func runFleetWorker(ctx context.Context, cfg *config.Settings, bc *BotConfig, tgBot *telego.Bot, store *db.Store, botRedis *redisx.Client, username string, concurrency int) error {
	opt, err := queue.RedisOpt(cfg.RedisURL)
	if err != nil {
		return err
	}

	if err := queue.RecoverOrphanedActiveTasks(cfg.RedisURL, bc.Queue); err != nil {
		slog.Warn("asynq queue recovery failed", "err", err, "bot", bc.Slug)
	}

	if concurrency <= 0 {
		concurrency = 2
	}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			bc.Queue: 1,
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			slog.Warn("asynq task failed", "type", task.Type(), "err", err, "bot", bc.Slug)
			metrics.RecordCeleryTask(task.Type(), "FAILURE")
			metrics.BotTasksTotal.WithLabelValues(bc.Slug, "FAILURE").Inc()
			metrics.RecordError("worker")
		}),
	})

	amux := asynq.NewServeMux()
	snd := sender.NewWithUsername(tgBot, sender.ResolveBotUsername(tgBot, username))
	deps := botworker.NewDeps(cfg, snd, store, botRedis)
	for _, p := range bc.Platforms {
		p.RegisterWorker(amux, deps, bc)
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown()
	}()

	slog.Info("starting asynq worker", "bot", bc.Slug, "queue", bc.Queue)
	return srv.Run(amux)
}
