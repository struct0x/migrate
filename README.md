# migrate

[![Go Reference](https://pkg.go.dev/badge/github.com/struct0x/migrate.svg)](https://pkg.go.dev/github.com/struct0x/migrate)
[![Go Report Card](https://goreportcard.com/badge/github.com/struct0x/migrate)](https://goreportcard.com/report/github.com/struct0x/migrate)
![Coverage](https://img.shields.io/badge/Coverage-95.0%25-brightgreen)

Dead simple in-app migration system.

## Install

```
go get github.com/struct0x/migrate
```

## Usage

In your module, declare migrations

```go
package users

import "github.com/struct0x/migrate"

var StoreMigrations = migrate.Migrations{
	ModuleName: "users",
	Migrations: []migrate.Migration{
		{
			Name: "001_create_users",
			SQL: `CREATE TABLE users (
              id   TEXT PRIMARY KEY,
              name TEXT NOT NULL
			)`,
		},
		{
			Name: "002_add_email",
			SQL:  `ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''`,
		},
	},
}

```

At startup, pass all module groups together:

```go
package main

func main() {
	// ...
	if err := migrate.Migrate(ctx, db, migrate.Postgres,
		users.StoreMigrations,
		payments.StoreMigrations,
		licensing.StoreMigrations,
	); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
}

```

## Dialects

| Dialect    | Var                |
|------------|--------------------|
| SQLite     | `migrate.SQLite`   |
| PostgreSQL | `migrate.Postgres` |
| MySQL      | `migrate.MySQL`    |

## Options

| Option                     | Description                                             |
|----------------------------|---------------------------------------------------------|
| `WithLogger(*slog.Logger)` | Sets the logger used to print what migrations were run. |

## Behavior

- Idempotent — safe to call on every startup
- Concurrent-safe — multiple processes can call Migrate simultaneously
- Each migration runs exactly once
- A failed migration is rolled back and returns an error; subsequent migrations in the group are not applied

## License

MIT License
