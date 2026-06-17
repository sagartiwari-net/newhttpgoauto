package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

// PingContext checks whether the pool can reach MySQL.
func PingContext(ctx context.Context) error {
	if DB == nil {
		return fmt.Errorf("db not initialized")
	}
	return DB.PingContext(ctx)
}

// Reconnect closes the pool and opens a new one (e.g. after SSH tunnel drop).
func Reconnect(host, port, user, pass, name string) error {
	if DB != nil {
		_ = DB.Close()
		DB = nil
	}
	return Init(host, port, user, pass, name)
}

func Init(host, port, user, pass, name string) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local&charset=utf8mb4",
		user, pass, host, port, name)
	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	DB.SetMaxOpenConns(20)
	DB.SetMaxIdleConns(8)
	DB.SetConnMaxLifetime(5 * time.Minute)
	if err = DB.Ping(); err != nil {
		return fmt.Errorf("mysql ping failed: %w", err)
	}
	log.Printf("✅ [DB] Connected to %s:%s/%s", host, port, name)
	return nil
}

// InitWithRetry waits for MySQL (e.g. SSH tunnel still starting on worker boot).
func InitWithRetry(host, port, user, pass, name string, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := Init(host, port, user, pass, name); err == nil {
			return nil
		} else {
			lastErr = err
			if DB != nil {
				_ = DB.Close()
				DB = nil
			}
			log.Printf("⏳ [DB] waiting for MySQL (%s:%s)... %v", host, port, err)
			time.Sleep(3 * time.Second)
		}
	}
	return lastErr
}

func Close() {
	if DB != nil {
		_ = DB.Close()
	}
}
