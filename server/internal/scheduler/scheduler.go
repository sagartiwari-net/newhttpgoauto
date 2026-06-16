package scheduler

import (
	"log"
	"time"

	"gohttpauto/internal/db"
	"gohttpauto/internal/queue"
)

// Start runs a 60-second cron loop for enabled tasks.
func Start() {
	go func() {
		log.Println("⏰ [SCHEDULER] Started (60s interval)")
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			checkDueTasks()
		}
	}()
}

func checkDueTasks() {
	rows, err := db.DB.Query(`
		SELECT task_uid FROM tasks
		WHERE is_enabled=1 AND (next_run_at IS NULL OR next_run_at <= NOW())`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) == nil {
			queue.Submit(uid, "cron")
		}
	}
}
