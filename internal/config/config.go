package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	NATS     NATSConfig
	Ingest   IngestConfig
}

type ServerConfig struct {
	Port         string
	Host         string
	AllowOrigins string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (d DatabaseConfig) DSN() string {
	return strings.Join([]string{
		"host=" + d.Host,
		"user=" + d.User,
		"password=" + d.Password,
		"dbname=" + d.DBName,
		"port=" + d.Port,
		"sslmode=" + d.SSLMode,
		"TimeZone=UTC",
	}, " ")
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

func (r RedisConfig) Addr() string {
	return r.Host + ":" + r.Port
}

type NATSConfig struct {
	URL string
}

// IngestConfig is the shared secret and external system name for live positions.
type IngestConfig struct {
	Token  string
	System string
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         env("SERVER_PORT", "8082"),
			Host:         env("SERVER_HOST", "0.0.0.0"),
			AllowOrigins: env("ALLOW_ORIGINS", "http://localhost:3000,http://localhost:5173"),
		},
		Database: DatabaseConfig{
			Host:     env("DB_HOST", "localhost"),
			Port:     env("DB_PORT", "5440"),
			User:     env("DB_USER", "registry"),
			Password: env("DB_PASSWORD", "registry"),
			DBName:   env("DB_NAME", "registry"),
			SSLMode:  env("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Host:     env("REDIS_HOST", "localhost"),
			Port:     env("REDIS_PORT", "6382"),
			Password: env("REDIS_PASSWORD", ""),
			DB:       envInt("REDIS_DB", 0),
		},
		NATS: NATSConfig{
			URL: env("NATS_URL", "nats://localhost:4224"),
		},
		Ingest: IngestConfig{
			Token:  env("INGEST_TOKEN", ""),
			System: env("INGEST_EXTERNAL_SYSTEM", "traccar-go"),
		},
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
