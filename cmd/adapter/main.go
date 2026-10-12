package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/automax/registry/internal/config"
	"github.com/automax/registry/internal/database"
	"github.com/automax/registry/internal/handlers"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	port := env("ADAPTER_PORT", "8083")
	host := env("ADAPTER_HOST", "0.0.0.0")
	token := strings.TrimSpace(os.Getenv("INGEST_TOKEN"))

	redisClient, err := database.ConnectRedis(cfg.Redis)
	if err != nil {
		log.Fatalf("adapter: %v", err)
	}
	defer database.CloseRedis(redisClient)

	if token == "" {
		log.Println("adapter: INGEST_TOKEN is empty; position ingest will reject requests")
	}
	app := handlers.NewAdapter(token, database.NewGeoStore(redisClient))
	addr := host + ":" + port

	go func() {
		log.Printf("adapter listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("adapter: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("adapter shutting down")
	if err := app.Shutdown(); err != nil {
		log.Printf("adapter: shutdown: %v", err)
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
