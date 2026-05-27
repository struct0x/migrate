package migrate_test

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/struct0x/migrate"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPostgres(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	pgc, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("secret"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = pgc.Terminate(ctx)
	})

	adminConnStr, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get admin connection string: %v", err)
	}
	adminDB, err := sql.Open("pgx", adminConnStr)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Close()
	})

	var schemaID atomic.Int64

	runSuite(t, func(t *testing.T) *sql.DB {
		schema := fmt.Sprintf("test_%d", schemaID.Add(1))

		if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema)); err != nil {
			t.Fatalf("create schema %s: %v", schema, err)
		}
		t.Cleanup(func() {
			_, _ = adminDB.ExecContext(ctx, fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
		})

		connStr, err := pgc.ConnectionString(ctx, "sslmode=disable", "search_path="+schema)
		if err != nil {
			t.Fatalf("get connection string: %v", err)
		}
		db, err := sql.Open("pgx", connStr)
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() {
			_ = db.Close()
		})
		return db
	}, migrate.Postgres)
}
