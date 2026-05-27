package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

type Migration struct {
	Name string
	SQL  string
}

type Migrations struct {
	ModuleName string
	Migrations []Migration
}

type Dialect struct {
	createSQL string
	claimSQL  string
}

var (
	SQLite = Dialect{
		createSQL: `CREATE TABLE if NOT EXISTS schema_migrations (
			module_name TEXT NOT NULL,
			migration   TEXT NOT NULL,
			applied_at  TEXT NOT NULL,
			PRIMARY KEY (module_name, migration)
		)`,
		claimSQL: `INSERT INTO schema_migrations (module_name, migration, applied_at) VALUES (?, ?, ?) ON CONFLICT(module_name, migration) DO NOTHING`,
	}
	Postgres = Dialect{
		createSQL: `DO $$ BEGIN
			CREATE TABLE IF NOT EXISTS schema_migrations (
				module_name TEXT NOT NULL,
				migration   TEXT NOT NULL,
				applied_at  TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (module_name, migration)
			);
		EXCEPTION
			WHEN duplicate_table  THEN NULL;
			WHEN duplicate_object THEN NULL;
			WHEN unique_violation THEN NULL;
		END $$`,
		claimSQL: `INSERT INTO schema_migrations (module_name, migration, applied_at) VALUES ($1, $2, $3) ON CONFLICT(module_name, migration) DO NOTHING`,
	}
	MySQL = Dialect{
		createSQL: `CREATE TABLE if NOT EXISTS schema_migrations (
			module_name VARCHAR(255) NOT NULL,
			migration   VARCHAR(255) NOT NULL,
			applied_at  DATETIME NOT NULL,
			PRIMARY KEY (module_name, migration)
		)`,
		claimSQL: `INSERT IGNORE INTO schema_migrations (module_name, migration, applied_at) VALUES (?, ?, ?)`,
	}
)

type Option interface {
	apply(*config)
}

type config struct {
	migrations []Migrations
	logger     *slog.Logger
}

func (m Migrations) apply(c *config) {
	c.migrations = append(c.migrations, m)
}

type loggerOption struct{ logger *slog.Logger }

func (o loggerOption) apply(c *config) { c.logger = o.logger }

func WithLogger(logger *slog.Logger) Option { return loggerOption{logger} }

// Migrate runs database migrations in a safe and idempotent way.
//
// It creates a schema_migrations table if it doesn't exist, then applies each migration
// exactly once by claiming it first. Migrations are grouped by module name and executed
// in order. Each migration runs in its own transaction - if it fails, the transaction
// is rolled back and an error is returned.
//
// The function is safe for concurrent execution: only one instance will successfully
// claim and apply each migration. Already applied migrations are skipped automatically.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - db: database connection to run migrations on
//   - dialect: database-specific SQL syntax (SQLite, Postgres, or MySQL)
//   - opts: optional configuration (migrations to run, logger for output)
//
// Returns an error if table creation fails, transaction handling fails, or any
// migration SQL fails to execute.
func Migrate(
	ctx context.Context,
	db *sql.DB,
	dialect Dialect,
	opts ...Option,
) error {
	var cfg config
	for _, o := range opts {
		o.apply(&cfg)
	}

	_, err := db.ExecContext(ctx, dialect.createSQL)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, group := range cfg.migrations {
		for _, m := range group.Migrations {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("begin tx for %s/%s: %w", group.ModuleName, m.Name, err)
			}

			res, err := tx.ExecContext(ctx, dialect.claimSQL,
				group.ModuleName, m.Name, time.Now().UTC(),
			)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("claim migration %s/%s: %w", group.ModuleName, m.Name, err)
			}

			n, _ := res.RowsAffected()
			if n == 0 {
				_ = tx.Rollback()
				continue
			}

			if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply migration %s/%s: %w", group.ModuleName, m.Name, err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit migration %s/%s: %w", group.ModuleName, m.Name, err)
			}

			if cfg.logger != nil {
				cfg.logger.InfoContext(ctx, "applied migration",
					"module", group.ModuleName,
					"name", m.Name,
				)
			}
		}
	}

	return nil
}
