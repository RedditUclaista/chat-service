package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gocql/gocql"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/RedditUclaista/chat-service/internal/database"
	deliveryhttp "github.com/RedditUclaista/chat-service/internal/delivery/http"
	"github.com/RedditUclaista/chat-service/internal/lib"
	"github.com/RedditUclaista/chat-service/internal/usecases"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// ────────────────────────────────────────────────────────────────────────
	// 1. CONEXIÓN A SCYLLADB
	// DB_HOST viene del docker-compose environment
	// ────────────────────────────────────────────────────────────────────────
	cluster := gocql.NewCluster(getEnv("DB_HOST", "localhost"))
	cluster.Keyspace = getEnv("SCYLLA_KEYSPACE", "chat_service")
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 5 * time.Second
	cluster.ConnectTimeout = 10 * time.Second

	session, err := cluster.CreateSession()
	if err != nil {
		logger.Error("no se pudo conectar a ScyllaDB", "error", err)
		os.Exit(1)
	}
	defer session.Close()
	logger.Info("ScyllaDB conectado")

	// ────────────────────────────────────────────────────────────────────────
	// 2. DEPENDENCY INJECTION
	// ────────────────────────────────────────────────────────────────────────
	repo := database.NewScyllaRepository(session)
	chatUC := usecases.NewChatUseCase(repo)
	chatHandler := deliveryhttp.NewChatHandler(chatUC)

	// ────────────────────────────────────────────────────────────────────────
	// 3. SERVIDOR ECHO v5
	// En v5 HideBanner se configura via StartConfig, no en Echo struct
	// ────────────────────────────────────────────────────────────────────────
	e := echo.New()

	// Echo v5 usa *slog.Logger directamente
	e.Logger = logger

	// Validador personalizado
	e.Validator = lib.NewValidator()

	// Middlewares globales
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger()) // usa slog internamente en v5

	// Health check (sin autenticación)
	e.GET("/health", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Grupo de rutas API
	api := e.Group("/api/v1")
	// TODO: api.Use(tu middleware JWT aquí)

	chats := api.Group("/chats")
	chatHandler.RegisterRoutes(chats)

	// ────────────────────────────────────────────────────────────────────────
	// 4. ARRANQUE CON GRACEFUL SHUTDOWN
	// El puerto interno siempre es 10000 (el docker-compose mapea APP_PORT:10000)
	// ────────────────────────────────────────────────────────────────────────
	port := getEnv("PORT", "10000")

	// Arrancar en goroutine para no bloquear el graceful shutdown
	go func() {
		cfg := echo.StartConfig{Address: ":" + port}
		logger.Info("Chat Service iniciando", "port", port)
		if err := cfg.Start(e); err != nil && err != http.ErrServerClosed {
			logger.Error("error en el servidor", "error", err)
			os.Exit(1)
		}
	}()

	// Esperar señal de cierre (Ctrl+C o SIGTERM del orquestador)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("apagando servidor...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		logger.Error("error en shutdown", "error", err)
	}
	logger.Info("servidor apagado correctamente")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
