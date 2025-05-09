package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"user-service/internal/delivery/messaging" // Changed from messaging to tcp
	"user-service/internal/infastructure" // Fixed typo in package name
	"user-service/internal/repository"
	"user-service/internal/usecase"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Create a context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// PostgreSQL connection with optimized pool settings
	pgConfig, err := pgxpool.ParseConfig("postgresql://postgres:pfYtJzUVVcksnbRPNwoMUMeAbluqMqgJ@centerbeam.proxy.rlwy.net:44785/railway")
	if err != nil {
		log.Fatalf("Unable to parse PostgreSQL config: %v", err)
	}

	// Connection pool optimization
	pgConfig.MaxConns = 20
	pgConfig.MinConns = 5
	pgConfig.MaxConnLifetime = time.Hour
	pgConfig.MaxConnIdleTime = 30 * time.Minute
	pgConfig.HealthCheckPeriod = 5 * time.Minute

	pgPool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		log.Fatalf("Unable to connect to PostgreSQL: %v", err)
	}
	defer pgPool.Close()

	// Configure Redis client with optimized settings
	redisClient := redis.NewClient(&redis.Options{
		Addr:         "localhost:6379",
		Password:     "", // Add if needed
		DB:           0,  // Default DB
		PoolSize:     10,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	defer redisClient.Close()

	// Verify Redis connection
	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Setup service layers
	userRepo := repository.NewUserRepo(pgPool)
	redisRepo := repository.NewRedisRepo(redisClient)
	jwtService := infastructure.NewJWTService() // Fixed package name
	userUsecase := usecase.NewUserUsecase(userRepo, redisRepo, jwtService)

	// Initialize TCP handler
	tcpHandler := tcp.NewTCPHandler(userUsecase)

	// Start TCP server in a goroutine
	go func() {
		log.Println("Starting TCP server on port 3001")
		if err := tcpHandler.Start(":3001"); err != nil {
			log.Fatalf("TCP server failed: %v", err)
		}
	}()

	// Graceful shutdown handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal
	<-sigCh
	log.Println("Received shutdown signal, initiating graceful shutdown...")

	// Shutdown TCP server
	if err := tcpHandler.Stop(); err != nil {
		log.Printf("Error shutting down TCP server: %v", err)
	}

	log.Println("Service shutdown completed successfully")
}