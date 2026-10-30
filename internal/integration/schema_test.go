package integration

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSchemaHasSixTablesAndRealForeignKeys(t *testing.T) {
	var tables []string
	if err := db.Raw(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE' AND table_name <> 'schema_migrations'
		ORDER BY table_name`).Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	wantTables := []string{"channel_acl", "channels", "device_credentials", "groups", "thing_external_refs", "things"}
	assertThingCoordinateColumns(t)
	if strings.Join(tables, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("tables = %v", tables)
	}

	type fkRow struct {
		TableName    string `gorm:"column:table_name"`
		ColumnName   string `gorm:"column:column_name"`
		ForeignTable string `gorm:"column:foreign_table"`
		DeleteRule   string `gorm:"column:delete_rule"`
	}
	var fks []fkRow
	if err := db.Raw(`
		SELECT tc.table_name, kcu.column_name, ccu.table_name AS foreign_table, rc.delete_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		JOIN information_schema.referential_constraints rc
		  ON tc.constraint_name = rc.constraint_name AND tc.table_schema = rc.constraint_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON rc.unique_constraint_name = ccu.constraint_name AND rc.unique_constraint_schema = ccu.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = 'public'
		ORDER BY tc.table_name, kcu.column_name`).Scan(&fks).Error; err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, fk := range fks {
		got[fk.TableName+"."+fk.ColumnName] = fk.ForeignTable + "|" + fk.DeleteRule
	}
	want := map[string]string{
		"things.group_id":              "groups|RESTRICT",
		"groups.parent_group_id":       "groups|RESTRICT",
		"channel_acl.channel_id":       "channels|CASCADE",
		"channel_acl.thing_id":         "things|CASCADE",
		"device_credentials.thing_id":  "things|RESTRICT",
		"thing_external_refs.thing_id": "things|RESTRICT",
	}
	if len(got) != len(want) {
		t.Fatalf("foreign keys = %#v", got)
	}
	for key, rule := range want {
		if got[key] != rule {
			t.Fatalf("fk %s = %q, want %q (all: %#v)", key, got[key], rule, got)
		}
	}
	for key := range got {
		if strings.Contains(key, "department") {
			t.Fatalf("department_id must not be a foreign key: %s", key)
		}
	}

	type uniq struct {
		Name string
		Cols string
	}
	var uniques []uniq
	if err := db.Raw(`
		SELECT tc.constraint_name AS name,
		       string_agg(kcu.column_name, ',' ORDER BY kcu.ordinal_position) AS cols
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'UNIQUE' AND tc.table_schema = 'public'
		GROUP BY tc.constraint_name
		ORDER BY tc.constraint_name`).Scan(&uniques).Error; err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, u := range uniques {
		found[u.Name] = u.Cols
	}
	for name, cols := range map[string]string{
		"uq_channel_acl_channel_thing":        "channel_id,thing_id",
		"uq_device_credentials_api_key":       "api_key",
		"uq_thing_external_refs_thing_system": "thing_id,external_system",
	} {
		if found[name] != cols {
			t.Fatalf("unique %s = %q, want %q (all %#v)", name, found[name], cols, found)
		}
	}
}

func TestDatabaseRejectsBrokenForeignKeys(t *testing.T) {
	truncate(t)
	missingGroup := uuid.MustParse("00000000-0000-0000-0000-000000000099")
	err := db.Exec(`INSERT INTO things (name, type, department_id, group_id) VALUES ('x', 'sensor', ?, ?)`, uuid.New(), missingGroup).Error
	if pgCode(err) != "23503" {
		t.Fatalf("thing with missing group: code %s err %v", pgCode(err), err)
	}

	thingID := uuid.New()
	if err := db.Exec(`INSERT INTO things (id, name, type, department_id) VALUES (?, 'truck', 'vehicle', ?)`, thingID, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}

	parent := uuid.New()
	child := uuid.New()
	dept := uuid.New()
	if err := db.Exec(`INSERT INTO groups (id, name, department_id) VALUES (?, 'parent', ?)`, parent, dept).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO groups (id, name, parent_group_id, department_id) VALUES (?, 'child', ?, ?)`, child, parent, dept).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Exec(`DELETE FROM groups WHERE id = ?`, parent).Error
	if pgCode(err) != "23503" {
		t.Fatalf("delete parent group: code %s err %v", pgCode(err), err)
	}

	if err := db.Exec(`INSERT INTO device_credentials (thing_id, api_key, api_secret_hash, status) VALUES (?, 'k1', 'hash-1', 'nope')`, thingID).Error; pgCode(err) != "23514" {
		t.Fatalf("bad credential status: code %s err %v", pgCode(err), err)
	}
	if err := db.Exec(`INSERT INTO device_credentials (thing_id, api_key, api_secret_hash) VALUES (?, 'k1', 'hash-1')`, thingID).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Exec(`INSERT INTO device_credentials (thing_id, api_key, api_secret_hash) VALUES (?, 'k1', 'hash-2')`, thingID).Error
	if pgCode(err) != "23505" {
		t.Fatalf("duplicate api_key: code %s err %v", pgCode(err), err)
	}

	if err := db.Exec(`INSERT INTO thing_external_refs (thing_id, external_system, external_id) VALUES (?, 'traccar-go', 'dev-1')`, thingID).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Exec(`INSERT INTO thing_external_refs (thing_id, external_system, external_id) VALUES (?, 'traccar-go', 'dev-2')`, thingID).Error
	if pgCode(err) != "23505" {
		t.Fatalf("duplicate external ref: code %s err %v", pgCode(err), err)
	}

	err = db.Exec(`DELETE FROM things WHERE id = ?`, thingID).Error
	if pgCode(err) != "23503" {
		t.Fatalf("delete thing with credential: code %s err %v", pgCode(err), err)
	}
}

func assertThingCoordinateColumns(t *testing.T) {
	t.Helper()
	type col struct {
		Name     string `gorm:"column:column_name"`
		Nullable string `gorm:"column:is_nullable"`
		Type     string `gorm:"column:data_type"`
	}
	var cols []col
	err := db.Raw(`
		SELECT column_name, is_nullable, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'things' AND column_name IN ('latitude', 'longitude')
		ORDER BY column_name`).Scan(&cols).Error
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 2 || cols[0].Name != "latitude" || cols[1].Name != "longitude" {
		t.Fatalf("coordinate columns %#v", cols)
	}
	for _, c := range cols {
		if c.Nullable != "YES" || c.Type != "double precision" {
			t.Fatalf("column %#v", c)
		}
	}
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
