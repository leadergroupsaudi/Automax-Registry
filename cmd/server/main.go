package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/automax/registry/internal/config"
	"github.com/automax/registry/internal/database"
	"github.com/automax/registry/internal/handlers"
	"github.com/automax/registry/internal/natsclient"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("registry: %v", err)
	}
	defer database.Close(db)

	if err := database.Migrate(db); err != nil {
		log.Fatalf("registry: migrate: %v", err)
	}

	redisClient, err := database.ConnectRedis(cfg.Redis)
	if err != nil {
		log.Fatalf("registry: %v", err)
	}
	defer database.CloseRedis(redisClient)

	nc := natsclient.Connect(cfg.NATS.URL)
	defer nc.Close()

	app := handlers.NewApp(db, nc, handlers.HealthTargets{
		DB:            db,
		Redis:         redisClient,
		NATSConnected: nc.Connected,
	}, cfg.Server.AllowOrigins)
	addr := cfg.Server.Host + ":" + cfg.Server.Port

	go func() {
		log.Printf("registry listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("registry: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("registry shutting down")
	if err := app.Shutdown(); err != nil {
		log.Printf("registry: shutdown: %v", err)
	}
}
