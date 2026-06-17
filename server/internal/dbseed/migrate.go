package dbseed

import (
	"log"

	"gohttpauto/internal/db"
)

// EnsureSchema applies lightweight idempotent schema updates (ENUM values, etc.).
func EnsureSchema() {
	_, err := db.DB.Exec(`
		ALTER TABLE tasks MODIFY automation_type
		ENUM('http','chrome_extension','chrome_hybrid','cred_fetch','chrome_portal')
		NOT NULL DEFAULT 'http'`)
	if err != nil {
		log.Printf("⚠️ [DB] tasks.automation_type migration: %v", err)
	} else {
		log.Println("🌱 [DB] tasks.automation_type includes chrome_portal")
	}

	res, err := db.DB.Exec(`DELETE FROM tasks WHERE task_uid='gfx_captureHomepage'`)
	if err != nil {
		log.Printf("⚠️ [DB] remove gfx_captureHomepage: %v", err)
	} else if n, _ := res.RowsAffected(); n > 0 {
		log.Println("🌱 [DB] Removed gfx_captureHomepage task")
	}
}
