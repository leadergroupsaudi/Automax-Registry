package database

import (
	_ "embed"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

//go:embed sql/0001_registry_schema.sql
var migration0001 string

//go:embed sql/0002_thing_coordinates.sql
var migration0002 string

func Migrate(db *gorm.DB) error {
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`).Error; err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}

	migrations := []struct {
		version string
		sql     string
	}{
		{"0001_registry_schema", migration0001},
		{"0002_thing_coordinates", migration0002},
	}
	for _, m := range migrations {
		var applied int64
		if err := db.Raw(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&applied).Error; err != nil {
			return fmt.Errorf("check migration %s: %w", m.version, err)
		}
		if applied > 0 {
			continue
		}
		if err := applyMigration(db, m.version, m.sql); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(db *gorm.DB, version, script string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range splitSQL(script) {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("migration %s: %w", version, err)
			}
		}
		if err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		return nil
	})
}

// splitSQL splits a script on semicolons that are outside single-quoted strings.
func splitSQL(script string) []string {
	var stmts []string
	var b strings.Builder
	inSingle := false
	for i := 0; i < len(script); i++ {
		c := script[i]
		if c == '\'' {
			if inSingle && i+1 < len(script) && script[i+1] == '\'' {
				b.WriteByte(c)
				b.WriteByte(script[i+1])
				i++
				continue
			}
			inSingle = !inSingle
			b.WriteByte(c)
			continue
		}
		if c == ';' && !inSingle {
			if s := strings.TrimSpace(b.String()); s != "" && !onlyComments(s) {
				stmts = append(stmts, s)
			}
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	if s := strings.TrimSpace(b.String()); s != "" && !onlyComments(s) {
		stmts = append(stmts, s)
	}
	return stmts
}

func onlyComments(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		return false
	}
	return true
}
