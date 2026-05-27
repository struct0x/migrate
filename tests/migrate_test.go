package migrate_test

import (
	"database/sql"
	"slices"
	"sync"
	"testing"

	"github.com/struct0x/migrate"
	_ "modernc.org/sqlite"
)

func TestSQLite(t *testing.T) {
	t.Parallel()

	runSuite(t, func(t *testing.T) *sql.DB {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() {
			_ = db.Close()
		})
		return db
	}, migrate.SQLite)
}

// assertMigrations verifies the schema_migrations table contains exactly the
// given "module_name/migration" entries and nothing else.
func assertMigrations(t *testing.T, db *sql.DB, want ...string) {
	t.Helper()

	rows, err := db.QueryContext(t.Context(), `SELECT module_name, migration FROM schema_migrations ORDER BY module_name, migration`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var module, migration string
		if err := rows.Scan(&module, &migration); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, module+"/"+migration)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("schema_migrations mismatch\nwant: %v\ngot:  %v", want, got)
	}
}

func runSuite(t *testing.T, newDB func(*testing.T) *sql.DB, dialect migrate.Dialect) {
	t.Helper()
	ctx := t.Context()

	t.Run("applies_migrations", func(t *testing.T) {
		db := newDB(t)
		group := migrate.Migrations{
			ModuleName: "core",
			Migrations: []migrate.Migration{
				{Name: "001_create_users", SQL: `CREATE TABLE users (id INTEGER PRIMARY KEY)`},
				{Name: "002_add_email", SQL: `ALTER TABLE users ADD COLUMN email TEXT`},
			},
		}
		if err := migrate.Migrate(ctx, db, dialect, group); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		assertMigrations(
			t,
			db,
			"core/001_create_users",
			"core/002_add_email",
		)
	})

	t.Run("idempotent", func(t *testing.T) {
		t.Parallel()
		db := newDB(t)
		group := migrate.Migrations{
			ModuleName: "core",
			Migrations: []migrate.Migration{
				{Name: "001_create_users", SQL: `CREATE TABLE users (id INTEGER PRIMARY KEY)`},
			},
		}
		for range 3 {
			if err := migrate.Migrate(ctx, db, dialect, group); err != nil {
				t.Fatalf("migrate: %v", err)
			}
		}
		assertMigrations(t, db, "core/001_create_users")
	})

	t.Run("multiple_modules", func(t *testing.T) {
		t.Parallel()
		db := newDB(t)
		a := migrate.Migrations{
			ModuleName: "module_a",
			Migrations: []migrate.Migration{
				{Name: "001_init", SQL: `CREATE TABLE ta (id INTEGER PRIMARY KEY)`},
			},
		}
		b := migrate.Migrations{
			ModuleName: "module_b",
			Migrations: []migrate.Migration{
				{Name: "001_init", SQL: `CREATE TABLE tb (id INTEGER PRIMARY KEY)`},
			},
		}
		if err := migrate.Migrate(ctx, db, dialect, a, b); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		assertMigrations(
			t,
			db,
			"module_a/001_init",
			"module_b/001_init",
		)
	})

	t.Run("failed_migration_rolls_back", func(t *testing.T) {
		t.Parallel()
		db := newDB(t)
		group := migrate.Migrations{
			ModuleName: "core",
			Migrations: []migrate.Migration{
				{Name: "001_bad", SQL: `THIS IS NOT VALID SQL`},
			},
		}
		if err := migrate.Migrate(ctx, db, dialect, group); err == nil {
			t.Fatal("want error for invalid SQL, got nil")
		}
		assertMigrations(t, db)
	})

	t.Run("concurrent", func(t *testing.T) {
		t.Parallel()
		db := newDB(t)
		group := migrate.Migrations{
			ModuleName: "core",
			Migrations: []migrate.Migration{
				{Name: "001_create_events", SQL: `CREATE TABLE events (id INTEGER PRIMARY KEY)`},
			},
		}
		const n = 20
		errs := make([]error, n)
		var wg sync.WaitGroup
		wg.Add(n)
		for i := range n {
			go func(i int) {
				defer wg.Done()
				errs[i] = migrate.Migrate(ctx, db, dialect, group)
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
			}
		}
		assertMigrations(
			t,
			db,
			"core/001_create_events",
		)
	})
}
