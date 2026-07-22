package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gocql/gocql"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/valkey-io/valkey-go"

	"github.com/RedditUclaista/chat-service/internal/bus"
	"github.com/RedditUclaista/chat-service/internal/cache"
	"github.com/RedditUclaista/chat-service/internal/database"
	deliveryhttp "github.com/RedditUclaista/chat-service/internal/delivery/http"
	ws "github.com/RedditUclaista/chat-service/internal/delivery/websocket"
	"github.com/RedditUclaista/chat-service/internal/hub"
	"github.com/RedditUclaista/chat-service/internal/lib"
	"github.com/RedditUclaista/chat-service/internal/usecases"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := database.RunMigrations(database.MigrateConfig{
		Host:              getEnv("DB_HOST", "localhost"),
		Keyspace:          getEnv("SCYLLA_KEYSPACE", "chat_service"),
		ReplicationFactor: 1,
	}); err != nil {
		logger.Error("migracion fallo", "error", err)
		os.Exit(1)
	}
	logger.Info("migraciones ejecutadas")

	cluster := gocql.NewCluster(getEnv("DB_HOST", "localhost"))
	cluster.Keyspace = getEnv("SCYLLA_KEYSPACE", "chat_service")
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 5 * time.Second
	cluster.ConnectTimeout = 10 * time.Second
	cluster.NumConns = runtime.GOMAXPROCS(0) * 2

	scyllaSession, err := cluster.CreateSession()
	if err != nil {
		logger.Error("no se pudo conectar a ScyllaDB", "error", err)
		os.Exit(1)
	}
	defer scyllaSession.Close()
	logger.Info("ScyllaDB conectado")

	valkeyClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{fmt.Sprintf("%s:6379", getEnv("CACHE_HOST", "localhost"))},
	})
	if err != nil {
		logger.Error("no se pudo conectar a Valkey", "error", err)
		os.Exit(1)
	}
	defer valkeyClient.Close()
	logger.Info("Valkey conectado")

	amqpConn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:5672/%s",
		getEnv("QUEUE_USER", "admin"),
		getEnv("QUEUE_PASS", "admin"),
		getEnv("QUEUE_HOST", "localhost"),
		getEnv("MQ_VHOST", "internal")))
	if err != nil {
		logger.Error("no se pudo conectar a LavinMQ", "error", err)
		os.Exit(1)
	}
	defer amqpConn.Close()
	logger.Info("LavinMQ conectado")

	publisher, err := bus.NewPublisher(amqpConn)
	if err != nil {
		logger.Error("no se pudo crear publisher", "error", err)
		os.Exit(1)
	}
	defer publisher.Close()

	consumer, err := bus.NewConsumer(amqpConn)
	if err != nil {
		logger.Error("no se pudo crear consumer", "error", err)
		os.Exit(1)
	}
	defer consumer.Close()

	chatHub := hub.New()
	valkeyCache := cache.NewValkeyCache(valkeyClient)
	repo := database.NewScyllaRepository(scyllaSession)
	chatUC := usecases.NewChatUseCase(repo, valkeyCache, publisher, chatHub)
	wsHandler := ws.NewHandler(chatUC, chatHub)
	chatHandler := deliveryhttp.NewChatHandler(chatUC, wsHandler)

	e := echo.New()
	e.Logger = logger
	e.Validator = lib.NewValidator()

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	e.GET("/health", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	api := e.Group("/api")
	api.Use(deliveryhttp.JWTAuth)

	chats := api.Group("/chat")
	chatHandler.RegisterRoutes(chats)
	chatHandler.RegisterWSRoute(chats)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	instanceID := fmt.Sprintf("%s-%d", getEnv("HOSTNAME", "chat-service"), os.Getpid())

	go func() {
		slog.Info("iniciando consumer fanout", "instance", instanceID)
		if err := consumer.ConsumeFanout(ctx, instanceID, chatUC.HandleMessageFanout); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("consumer fanout terminado con error", "error", err)
		}
	}()

	go func() {
		slog.Info("iniciando consumer persistence")
		if err := consumer.ConsumePersistence(ctx, chatUC.HandleMessagePersistence); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("consumer persistence terminado con error", "error", err)
		}
	}()

	port := getEnv("PORT", "10000")
	sc := echo.StartConfig{Address: ":" + port}

	go func() {
		logger.Info("Chat Service iniciando", "port", port)
		if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("error en el servidor", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	logger.Info("apagando servidor...")
	scyllaSession.Close()
	logger.Info("servidor apagado correctamente")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
