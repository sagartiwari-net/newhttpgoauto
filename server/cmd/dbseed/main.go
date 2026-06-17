// One-shot: apply schema migration + sync built-in tasks (run on panel VPS).
package main

import (
	"log"

	"gohttpauto/internal/config"
	"gohttpauto/internal/db"
	"gohttpauto/internal/dbseed"
)

func main() {
	cfg := config.Load()
	if err := db.Init(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName); err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()
	dbseed.EnsureSchema()
	dbseed.EnsureTasks()
	log.Println("✅ DB schema + tasks synced")
}
