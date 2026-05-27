package migrate_test

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/struct0x/migrate"
	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

func TestMySQL(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	mc, err := mysql.Run(ctx, "mysql:8.0",
		mysql.WithDatabase("testdb"),
		mysql.WithUsername("root"),
		mysql.WithPassword("rootpass"),
	)
	if err != nil {
		t.Fatalf("start mysql: %v", err)
	}
	t.Cleanup(func() {
		_ = mc.Terminate(ctx)
	})

	host, err := mc.Host(ctx)
	if err != nil {
		t.Fatalf("get mysql host: %v", err)
	}
	mappedPort, err := mc.MappedPort(ctx, "3306/tcp")
	if err != nil {
		t.Fatalf("get mysql mapped port: %v", err)
	}
	port := mappedPort.Port()

	adminDSN := fmt.Sprintf("root:rootpass@tcp(%s:%s)/?parseTime=true", host, port)
	adminDB, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Close()
	})

	var dbID atomic.Int64

	runSuite(t, func(t *testing.T) *sql.DB {
		dbName := fmt.Sprintf("test_%d", dbID.Add(1))

		if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName)); err != nil {
			t.Fatalf("create database %s: %v", dbName, err)
		}
		t.Cleanup(func() {
			_, _ = adminDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		})

		dsn := fmt.Sprintf("root:rootpass@tcp(%s:%s)/%s?parseTime=true", host, port, dbName)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() {
			_ = db.Close()
		})
		return db
	}, migrate.MySQL)
}
