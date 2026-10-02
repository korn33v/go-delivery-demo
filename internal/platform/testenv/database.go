//go:build integration

package testenv

import (
	"context"
	"database/sql"
	"errors"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"godelivery/internal/platform/app"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Отдельная schema в СОБСТВЕННОЙ БД сервиса не загрязняет демонстрационные данные.
func Database(t *testing.T, key, migrations string) *sql.DB {
	t.Helper()
	address := os.Getenv(key)
	if address == "" {
		t.Fatalf("%s is required: run make test-integration", key)
	}
	db, err := sql.Open("pgx", address)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := strings.ReplaceAll(app.ID("test"), "-", "_")
	if _, err = db.ExecContext(context.Background(), `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs(migrations)
	if err != nil {
		t.Fatal(err)
	}
	testDB, err := sql.Open("pgx", address)
	if err != nil {
		t.Fatal(err)
	}
	// Один session в тесте: SET search_path явно, без зависимости от startup parameters.
	// Production pool остаётся обычным; эти настройки относятся только к изоляции tests.
	testDB.SetMaxOpenConns(1)
	testDB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := migrateSchema(testDB, schema, path, "down"); err != nil {
			t.Error(err)
		}
		testDB.Close()
		if _, err := db.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	if err = migrateSchema(testDB, schema, path, "up"); err != nil {
		t.Fatal(err)
	}
	var actual string
	if err = testDB.QueryRow(`SELECT current_schema()`).Scan(&actual); err != nil || actual != schema {
		t.Fatalf("test schema isolation failed: want %s got %s err=%v", schema, actual, err)
	}
	return testDB
}

func migrateSchema(db *sql.DB, schema, path, direction string) error {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	// schema генерируется нами, не поступает из HTTP или пользовательских параметров.
	if _, err = conn.ExecContext(context.Background(), `SET search_path TO "`+schema+`"`); err != nil {
		return err
	}
	// WithConnection удерживает тот же session для DDL и migration/advisory lock.
	driver, err := postgres.WithConnection(context.Background(), conn, &postgres.Config{SchemaName: schema, MultiStatementEnabled: true})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+path, "postgres", driver)
	if err != nil {
		return err
	}
	defer m.Close()
	if direction == "down" {
		err = m.Down()
	} else {
		err = m.Up()
	}
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
