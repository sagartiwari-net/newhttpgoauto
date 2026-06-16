package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

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

func Close() {
	if DB != nil {
		_ = DB.Close()
	}
}
