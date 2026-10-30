package integration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/automax/registry/internal/config"
	"github.com/automax/registry/internal/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var db *gorm.DB

func TestMain(m *testing.M) {
	var err error
	db, err = openTestDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "registry tests: %v\n", err)
		os.Exit(1)
	}
	if err := resetSchema(db); err != nil {
		fmt.Fprintf(os.Stderr, "registry tests: reset schema: %v\n", err)
		os.Exit(1)
	}
	if err := database.Migrate(db); err != nil {
		fmt.Fprintf(os.Stderr, "registry tests: migrate: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	database.Close(db)
	os.Exit(code)
}

func openTestDB() (*gorm.DB, error) {
	if dsn := strings.TrimSpace(os.Getenv("REGISTRY_TEST_DSN")); dsn != "" {
		return gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		})
	}
	if err := ensurePostgresContainer(); err != nil {
		return nil, err
	}
	cfg := config.DatabaseConfig{
		Host:     "127.0.0.1",
		Port:     "5441",
		User:     "registry",
		Password: "registry",
		DBName:   "registry",
		SSLMode:  "disable",
	}
	deadline := time.Now().Add(40 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		conn, err := database.Connect(cfg)
		if err == nil {
			sqlDB, derr := conn.DB()
			if derr == nil && sqlDB.Ping() == nil {
				return conn, nil
			}
			database.Close(conn)
			last = derr
		} else {
			last = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("postgres did not become ready: %w", last)
}

func ensurePostgresContainer() error {
	const name = "automax-registry-test-pg"
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name).CombinedOutput()
	if err != nil {
		cmd := exec.Command("docker", "run", "-d", "--name", name, "--restart", "unless-stopped",
			"-e", "POSTGRES_USER=registry",
			"-e", "POSTGRES_PASSWORD=registry",
			"-e", "POSTGRES_DB=registry",
			"-p", "127.0.0.1:5441:5432",
			"postgres:16-alpine",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if runErr := cmd.Run(); runErr != nil {
			return fmt.Errorf("start postgres container: %w", runErr)
		}
		return nil
	}
	if strings.TrimSpace(string(out)) != "true" {
		if startErr := exec.Command("docker", "start", name).Run(); startErr != nil {
			return fmt.Errorf("start existing postgres container: %w", startErr)
		}
	}
	return nil
}

func resetSchema(conn *gorm.DB) error {
	if err := conn.Exec(`DROP SCHEMA IF EXISTS public CASCADE`).Error; err != nil {
		return err
	}
	if err := conn.Exec(`CREATE SCHEMA public`).Error; err != nil {
		return err
	}
	return conn.Exec(`GRANT ALL ON SCHEMA public TO public`).Error
}

func truncate(t *testing.T) {
	t.Helper()
	err := db.Exec(`TRUNCATE TABLE channel_acl, device_credentials, thing_external_refs, things, channels, groups CASCADE`).Error
	if err != nil {
		t.Fatal(err)
	}
}
