package queue

import (
	"log"
	"time"

	"gohttpauto/internal/db"
)

const workerID = "mac-worker"

// Enqueue adds a task for the remote worker to execute (panel role only).
func Enqueue(taskUID, triggeredBy string) error {
	_, err := db.DB.Exec(
		`INSERT INTO job_queue (task_uid, triggered_by, status) VALUES (?,?,'pending')`,
		taskUID, triggeredBy)
	if err != nil {
		return err
	}
	log.Printf("📥 [QUEUE] Enqueued %s (by %s) for worker", taskUID, triggeredBy)
	return nil
}

// StartJobPoller runs on worker — picks pending jobs from MySQL and executes locally.
func StartJobPoller() {
	go func() {
		log.Println("👂 [WORKER] Job poller started (3s interval)")
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			pollOnce()
		}
	}()
}

func pollOnce() {
	for {
		var id int
		var taskUID, triggeredBy string
		err := db.DB.QueryRow(`
			SELECT id, task_uid, triggered_by FROM job_queue
			WHERE status='pending' ORDER BY id ASC LIMIT 1`).Scan(&id, &taskUID, &triggeredBy)
		if err != nil {
			return
		}
		res, err := db.DB.Exec(`
			UPDATE job_queue SET status='claimed', claimed_by=?, claimed_at=NOW()
			WHERE id=? AND status='pending'`, workerID, id)
		if err != nil {
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return
		}
		log.Printf("▶️ [WORKER] Claimed job #%d: %s (by %s)", id, taskUID, triggeredBy)
		if !RunSync(taskUID, triggeredBy) {
			log.Printf("⚠️ [WORKER] %s already running, re-queueing job #%d", taskUID, id)
			_, _ = db.DB.Exec(`UPDATE job_queue SET status='pending', claimed_by=NULL, claimed_at=NULL WHERE id=?`, id)
			return
		}
		_, _ = db.DB.Exec(`UPDATE job_queue SET status='done', finished_at=NOW() WHERE id=?`, id)
	}
}
