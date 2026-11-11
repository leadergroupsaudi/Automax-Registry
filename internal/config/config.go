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
	Geo      GeoConfig
	Caguar   CaguarConfig
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

// GeoConfig is the search radius for FindNearestAvailable, in meters.
type GeoConfig struct {
	RadiusMeters float64
}

// CaguarConfig is the portal that receives device and driver registration.
type CaguarConfig struct {
	BaseURL  string
	Token    string
	Username string
	Password string
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
		Geo: GeoConfig{
			RadiusMeters: envFloat("GEO_RADIUS_METERS", 50000),
		},
		Caguar: CaguarConfig{
			BaseURL:  env("CAGUAR_BASE_URL", ""),
			Token:    env("CAGUAR_TOKEN", ""),
			Username: env("CAGUAR_USERNAME", ""),
			Password: env("CAGUAR_PASSWORD", ""),
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

func envFloat(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
