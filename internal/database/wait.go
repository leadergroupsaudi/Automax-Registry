package database

import (
	"log"
	"strings"
	"time"

	"github.com/automax/registry/internal/config"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ConnectWait retries while Postgres is still starting. Each attempt logs one line.
func ConnectWait(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var last error
	for attempt := 1; attempt <= 30; attempt++ {
		db, err := Connect(cfg)
		if err == nil {
			return db, nil
		}
		last = err
		log.Printf("registry: database not ready attempt=%d/30 reason=%s", attempt, waitReason(err))
		time.Sleep(time.Second)
	}
	return nil, last
}

// ConnectRedisWait retries while Redis is still starting. Each attempt logs one line.
func ConnectRedisWait(cfg config.RedisConfig) (*redis.Client, error) {
	var last error
	for attempt := 1; attempt <= 30; attempt++ {
		client, err := ConnectRedis(cfg)
		if err == nil {
			return client, nil
		}
		last = err
		log.Printf("registry: redis not ready attempt=%d/30 reason=%s", attempt, waitReason(err))
		time.Sleep(time.Second)
	}
	return nil, last
}

func waitReason(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "starting up"), strings.Contains(msg, "not yet accepting"):
		return "postgres is starting"
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "connect: connection refused"):
		return "not accepting connections"
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "context deadline exceeded"):
		return "timed out waiting for a connection"
	default:
		return msg
	}
}
