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
	"github.com/automax/registry/internal/repository"
	"github.com/automax/registry/internal/services"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	port := env("ADAPTER_PORT", "8083")
	host := env("ADAPTER_HOST", "0.0.0.0")
	token := strings.TrimSpace(os.Getenv("INGEST_TOKEN"))

	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("adapter: %v", err)
	}
	defer database.Close(db)
	if err := database.Migrate(db); err != nil {
		log.Fatalf("adapter: migrate: %v", err)
	}
	redisClient, err := database.ConnectRedis(cfg.Redis)
	if err != nil {
		log.Fatalf("adapter: %v", err)
	}
	defer database.CloseRedis(redisClient)

	if token == "" {
		log.Println("adapter: INGEST_TOKEN is empty; position ingest will reject requests")
	}
	things := services.NewThingCoordinateWriter(cfg.Ingest.System, repository.NewExternalRefRepository(db), repository.NewThingRepository(db))
	app := handlers.NewAdapterWithThings(token, database.NewGeoStore(redisClient), things)
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
